package queue

import (
	"context"
	"log"
	"time"

	"langapp-backend/internal/storage"

	"github.com/google/uuid"
)

type Matcher struct {
	storage *storage.Storage
}

func NewMatcher(storage *storage.Storage) *Matcher {
	return &Matcher{storage: storage}
}

type Match struct {
	UserA       storage.MatchRequest
	UserB       storage.MatchRequest
	LanguageA   string // A's native language (B's practice)
	LanguageB   string // B's native language (A's practice)
	MatchType   string // "perfect", "asymmetric", "fallback"
}

func (m *Matcher) FindMatches(ctx context.Context) ([]Match, error) {
	languages, err := m.storage.Redis.GetAllQueueLanguages(ctx)
	if err != nil {
		return nil, err
	}

	var matches []Match

	// Process each language queue
	for _, lang := range languages {
		queueMatches, err := m.processLanguageQueue(ctx, lang)
		if err != nil {
			log.Printf("Error processing queue for language %s: %v", lang, err)
			continue
		}
		matches = append(matches, queueMatches...)
	}

	return matches, nil
}

func (m *Matcher) processLanguageQueue(ctx context.Context, practiceLanguage string) ([]Match, error) {
	// Get all requests for this practice language
	requests, err := m.storage.Redis.GetQueueMembers(ctx, practiceLanguage, 100)
	if err != nil {
		return nil, err
	}

	if len(requests) < 2 {
		return nil, nil // Need at least 2 people to match
	}

	var matches []Match

	// Try to find perfect matches first (A's native = B's practice, B's native = A's practice)
	matches = append(matches, m.findPerfectMatches(requests, practiceLanguage)...)

	// Remove matched users from the requests slice
	remainingRequests := m.removeMatchedUsers(requests, matches)

	// Try asymmetric matches (one person's native matches other's practice)
	matches = append(matches, m.findAsymmetricMatches(remainingRequests, practiceLanguage)...)

	return matches, nil
}

func (m *Matcher) findPerfectMatches(requests []storage.MatchRequest, practiceLanguage string) []Match {
	var matches []Match
	used := make(map[uuid.UUID]bool)

	for i, reqA := range requests {
		if used[reqA.UserID] {
			continue
		}

		for j, reqB := range requests {
			if i >= j || used[reqB.UserID] || reqA.UserID == reqB.UserID {
				continue
			}

			// Check if A's native language contains B's practice language
			// and B's native language contains A's practice language
			aHasBPractice := m.hasLanguage(reqA.NativeLanguages, reqB.PracticeLanguage)
			bHasAPractice := m.hasLanguage(reqB.NativeLanguages, reqA.PracticeLanguage)

			if aHasBPractice && bHasAPractice {
				matches = append(matches, Match{
					UserA:     reqA,
					UserB:     reqB,
					LanguageA: m.findMatchingLanguage(reqA.NativeLanguages, reqB.PracticeLanguage),
					LanguageB: m.findMatchingLanguage(reqB.NativeLanguages, reqA.PracticeLanguage),
					MatchType: "perfect",
				})
				used[reqA.UserID] = true
				used[reqB.UserID] = true
				break
			}
		}
	}

	return matches
}

func (m *Matcher) findAsymmetricMatches(requests []storage.MatchRequest, practiceLanguage string) []Match {
	var matches []Match
	used := make(map[uuid.UUID]bool)

	for i, reqA := range requests {
		if used[reqA.UserID] {
			continue
		}

		for j, reqB := range requests {
			if i >= j || used[reqB.UserID] || reqA.UserID == reqB.UserID {
				continue
			}

			// Check if A's native contains B's practice (A teaches, B learns)
			if m.hasLanguage(reqA.NativeLanguages, reqB.PracticeLanguage) {
				matches = append(matches, Match{
					UserA:     reqA,
					UserB:     reqB,
					LanguageA: m.findMatchingLanguage(reqA.NativeLanguages, reqB.PracticeLanguage),
					LanguageB: reqA.PracticeLanguage, // A gets to practice their target
					MatchType: "asymmetric",
				})
				used[reqA.UserID] = true
				used[reqB.UserID] = true
				break
			}

			// Check if B's native contains A's practice (B teaches, A learns)
			if m.hasLanguage(reqB.NativeLanguages, reqA.PracticeLanguage) {
				matches = append(matches, Match{
					UserA:     reqA,
					UserB:     reqB,
					LanguageA: reqA.PracticeLanguage, // A gets to practice their target
					LanguageB: m.findMatchingLanguage(reqB.NativeLanguages, reqA.PracticeLanguage),
					MatchType: "asymmetric",
				})
				used[reqA.UserID] = true
				used[reqB.UserID] = true
				break
			}
		}
	}

	return matches
}

func (m *Matcher) removeMatchedUsers(requests []storage.MatchRequest, matches []Match) []storage.MatchRequest {
	matchedUsers := make(map[uuid.UUID]bool)
	for _, match := range matches {
		matchedUsers[match.UserA.UserID] = true
		matchedUsers[match.UserB.UserID] = true
	}

	var remaining []storage.MatchRequest
	for _, req := range requests {
		if !matchedUsers[req.UserID] {
			remaining = append(remaining, req)
		}
	}

	return remaining
}

func (m *Matcher) hasLanguage(languages []string, target string) bool {
	for _, lang := range languages {
		if lang == target {
			return true
		}
	}
	return false
}

func (m *Matcher) findMatchingLanguage(languages []string, target string) string {
	for _, lang := range languages {
		if lang == target {
			return lang
		}
	}
	return ""
}

func (m *Matcher) CreateChatSession(ctx context.Context, match Match) (*storage.ChatSession, error) {
	session := &storage.ChatSession{
		UserAID:   match.UserA.UserID,
		UserBID:   match.UserB.UserID,
		LanguageA: match.LanguageA,
		LanguageB: match.LanguageB,
		Status:    storage.SessionWaiting,
	}

	if err := m.storage.DB.CreateChatSession(ctx, session); err != nil {
		return nil, err
	}

	// Remove both users from their respective queues
	go func() {
		ctx := context.Background()
		m.storage.Redis.RemoveFromQueue(ctx, match.UserA.UserID.String(), match.UserA.PracticeLanguage)
		m.storage.Redis.RemoveFromQueue(ctx, match.UserB.UserID.String(), match.UserB.PracticeLanguage)
	}()

	// Set session info in Redis
	if err := m.storage.Redis.SetSessionUsers(ctx, session.ID.String(), 
		match.UserA.UserID.String(), match.UserB.UserID.String()); err != nil {
		log.Printf("Error setting session users in Redis: %v", err)
	}

	// Set session expiration (2 hours)
	if err := m.storage.Redis.ExpireSession(ctx, session.ID.String(), 2*time.Hour); err != nil {
		log.Printf("Error setting session expiration: %v", err)
	}

	return session, nil
}