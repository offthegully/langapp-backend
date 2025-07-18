package matchmaking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"langapp-backend/session"
	"langapp-backend/websocket"
)

type SessionRepository interface {
	CreateSession(ctx context.Context, user1ID, user2ID string, user1Native, user1Practice, user2Native, user2Practice string) (*session.Session, error)
	GetActiveSessionByUserID(ctx context.Context, userID string) (*session.Session, error)
}

type MatchingService struct {
	redisClient       RedisClient
	pubSubManager     PubSubManager
	wsManager         *websocket.Manager
	sessionRepository SessionRepository
	languages         []string
}

type Match struct {
	ID        string     `json:"match_id"`
	SessionID string     `json:"session_id"`
	User1     QueueEntry `json:"user1"`
	User2     QueueEntry `json:"user2"`
	CreatedAt time.Time  `json:"created_at"`
}

func NewMatchingService(redisClient RedisClient, pubSubManager PubSubManager, wsManager *websocket.Manager, sessionRepository SessionRepository, languages []string) *MatchingService {
	return &MatchingService{
		redisClient:       redisClient,
		pubSubManager:     pubSubManager,
		wsManager:         wsManager,
		sessionRepository: sessionRepository,
		languages:         languages,
	}
}

func (s *MatchingService) Start(ctx context.Context) {
	for _, language := range s.languages {
		go s.listenToLanguageChannel(ctx, language)
	}
	log.Printf("Matching service started for %d languages", len(s.languages))
}

func (s *MatchingService) listenToLanguageChannel(ctx context.Context, language string) {
	pubsub := s.pubSubManager.SubscribeToLanguageChannel(ctx, language)
	defer pubsub.Close()

	log.Printf("Listening to channel for language: %s", language)

	ch := pubsub.Channel()
	for msg := range ch {
		var newUser QueueEntry
		if err := json.Unmarshal([]byte(msg.Payload), &newUser); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		log.Printf("New user in %s channel: %s (native: %s, practice: %s)",
			language, newUser.UserID, newUser.NativeLanguage, newUser.PracticeLanguage)

		match, err := s.findMatch(ctx, newUser)
		if err != nil {
			log.Printf("Error finding match: %v", err)
			continue
		}

		if match != nil {
			log.Printf("Match found! %s <-> %s", match.User1.UserID, match.User2.UserID)
			if err := s.notifyMatch(match); err != nil {
				log.Printf("Error notifying match: %v", err)
			}
		}
	}
}

func (s *MatchingService) findMatch(ctx context.Context, newUser QueueEntry) (*Match, error) {
	// Check if user already has an active session
	if hasActiveSession, err := s.userHasActiveSession(ctx, newUser.UserID); err != nil {
		return nil, err
	} else if hasActiveSession {
		log.Printf("User %s already has an active session, skipping match", newUser.UserID)
		return nil, nil
	}

	// Look for matches in both directions (newUser's practice language and native language)
	var bestMatch *QueueEntry
	var primaryUser QueueEntry
	var secondaryUser QueueEntry

	// First, check if someone wants to practice newUser's native language
	practiceQueue := "queue:" + newUser.NativeLanguage
	if match := s.findMatchInQueue(ctx, practiceQueue, newUser, true); match != nil {
		// newUser gets to practice their language (they are secondary)
		primaryUser = *match
		secondaryUser = newUser
		bestMatch = match
	}

	// Then, check if someone is native in newUser's practice language
	nativeQueue := "queue:" + newUser.PracticeLanguage
	if match := s.findMatchInQueue(ctx, nativeQueue, newUser, false); match != nil {
		// Check if this match is better (older) than the previous one
		if bestMatch == nil || match.Timestamp.Before(bestMatch.Timestamp) {
			// newUser gets to practice their language (they are primary)
			primaryUser = newUser
			secondaryUser = *match
			bestMatch = match
		}
	}

	if bestMatch == nil {
		return nil, nil
	}

	// Remove both users from all queues
	if err := s.removeUserFromAllQueues(ctx, primaryUser.UserID); err != nil {
		log.Printf("Error removing primary user from queues: %v", err)
		return nil, err
	}
	if err := s.removeUserFromAllQueues(ctx, secondaryUser.UserID); err != nil {
		log.Printf("Error removing secondary user from queues: %v", err)
		return nil, err
	}

	matchID := fmt.Sprintf("match_%d", time.Now().Unix())
	return &Match{
		ID:        matchID,
		User1:     primaryUser,  // User who gets to practice their language
		User2:     secondaryUser, // User who speaks their native language
		CreatedAt: time.Now(),
	}, nil
}

func (s *MatchingService) findMatchInQueue(ctx context.Context, queueKey string, newUser QueueEntry, newUserIsPrimary bool) *QueueEntry {
	queueLength, err := s.redisClient.LLen(ctx, queueKey).Result()
	if err != nil {
		log.Printf("Error getting queue length for %s: %v", queueKey, err)
		return nil
	}

	var oldestMatch *QueueEntry
	for i := int64(0); i < queueLength; i++ {
		entryJSON, err := s.redisClient.LIndex(ctx, queueKey, i).Result()
		if err != nil {
			continue
		}

		var candidateUser QueueEntry
		if err := json.Unmarshal([]byte(entryJSON), &candidateUser); err != nil {
			continue
		}

		// Skip if same user
		if candidateUser.UserID == newUser.UserID {
			continue
		}

		// Check if user already has active session
		if hasActiveSession, err := s.userHasActiveSession(ctx, candidateUser.UserID); err != nil {
			log.Printf("Error checking active session for user %s: %v", candidateUser.UserID, err)
			continue
		} else if hasActiveSession {
			continue
		}

		// Check compatibility based on direction
		var isCompatible bool
		if newUserIsPrimary {
			// newUser wants to practice, candidateUser should be native in that language
			isCompatible = newUser.PracticeLanguage == candidateUser.NativeLanguage
		} else {
			// candidateUser wants to practice, newUser should be native in that language
			isCompatible = candidateUser.PracticeLanguage == newUser.NativeLanguage
		}

		if isCompatible {
			// Keep the oldest (earliest timestamp) match
			if oldestMatch == nil || candidateUser.Timestamp.Before(oldestMatch.Timestamp) {
				oldestMatch = &candidateUser
			}
		}
	}

	return oldestMatch
}

func (s *MatchingService) userHasActiveSession(ctx context.Context, userID string) (bool, error) {
	// Check if user has an active session in the database
	session, err := s.sessionRepository.GetActiveSessionByUserID(ctx, userID)
	if err != nil {
		return false, err
	}
	return session != nil, nil
}

func (s *MatchingService) removeUserFromAllQueues(ctx context.Context, userID string) error {
	// We need to remove the user from all possible queues
	// Since we don't know which queue they're in, we'll try all language queues
	for _, language := range s.languages {
		queueKey := "queue:" + language
		
		// Get all entries in this queue
		queueLength, err := s.redisClient.LLen(ctx, queueKey).Result()
		if err != nil {
			continue
		}

		// Search for user in this queue
		for i := int64(0); i < queueLength; i++ {
			entryJSON, err := s.redisClient.LIndex(ctx, queueKey, i).Result()
			if err != nil {
				continue
			}

			var entry QueueEntry
			if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
				continue
			}

			if entry.UserID == userID {
				// Remove this entry
				if err := s.redisClient.LRem(ctx, queueKey, 1, entryJSON).Err(); err != nil {
					log.Printf("Error removing user %s from queue %s: %v", userID, queueKey, err)
				} else {
					log.Printf("Removed user %s from queue %s", userID, queueKey)
				}
				break // User should only be in one queue, but continue checking other queues for safety
			}
		}
	}
	return nil
}

func (s *MatchingService) removeFromQueue(ctx context.Context, user QueueEntry) error {
	queueKey := "queue:" + user.PracticeLanguage
	userJSON, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return s.redisClient.LRem(ctx, queueKey, 1, userJSON).Err()
}

func (s *MatchingService) notifyMatch(match *Match) error {
	// Create session in database
	ctx := context.Background()
	session, err := s.sessionRepository.CreateSession(ctx,
		match.User1.UserID,
		match.User2.UserID,
		match.User1.NativeLanguage,
		match.User1.PracticeLanguage,
		match.User2.NativeLanguage,
		match.User2.PracticeLanguage,
	)
	if err != nil {
		log.Printf("Failed to create session for match %s: %v", match.ID, err)
		return err
	}

	match.SessionID = session.ID.String()

	log.Printf("Created session %s for match %s", session.ID.String(), match.ID)

	notification1 := websocket.MatchNotification{
		MatchID:   match.ID,
		PartnerID: match.User2.UserID,
		Language1: match.User1.NativeLanguage,
		Language2: match.User1.PracticeLanguage,
		Message:   fmt.Sprintf("Match found! You'll practice %s with %s", match.User1.PracticeLanguage, match.User2.UserID),
	}

	notification2 := websocket.MatchNotification{
		MatchID:   match.ID,
		PartnerID: match.User1.UserID,
		Language1: match.User2.NativeLanguage,
		Language2: match.User2.PracticeLanguage,
		Message:   fmt.Sprintf("Match found! You'll practice %s with %s", match.User2.PracticeLanguage, match.User1.UserID),
	}

	if err := s.wsManager.NotifyMatch(match.User1.UserID, notification1); err != nil {
		log.Printf("Failed to notify user %s: %v", match.User1.UserID, err)
	}

	if err := s.wsManager.NotifyMatch(match.User2.UserID, notification2); err != nil {
		log.Printf("Failed to notify user %s: %v", match.User2.UserID, err)
	}

	return nil
}
