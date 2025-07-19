package matchmaking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"langapp-backend/session"
	"langapp-backend/websocket"

	"github.com/redis/go-redis/v9"
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
	ID           string     `json:"match_id"`
	SessionID    string     `json:"session_id"`
	PracticeUser QueueEntry `json:"practice_user"`
	NativeUser   QueueEntry `json:"native_user"`
	Language     string     `json:"language"`
	CreatedAt    time.Time  `json:"created_at"`
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
		var nativeEntry QueueEntry
		if err := json.Unmarshal([]byte(msg.Payload), &nativeEntry); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		} // TODO - maybe remove from the channel?

		log.Printf("New user in %s channel: %s (native: %s, practice: %s)", language, nativeEntry.UserID, nativeEntry.NativeLanguage, nativeEntry.PracticeLanguage)

		match, err := s.findMatch(ctx, nativeEntry)
		if err != nil {
			log.Printf("Error finding match: %v", err)
			continue
		}

		if match != nil {
			// remove from something

			log.Printf("Match found! %s <-> %s", match.PracticeUser.UserID, match.NativeUser.UserID)
			if err := s.notifyMatch(match); err != nil {
				log.Printf("Error notifying match: %v", err)
			}
		}
	}
}

func (s *MatchingService) findMatch(ctx context.Context, nativeEntry QueueEntry) (*Match, error) {
	language := nativeEntry.NativeLanguage
	key := fmt.Sprintf("queue:%s", language)
	element, err := s.redisClient.LPop(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			log.Printf("Queue '%s' is empty.", key)
		}
		return nil, err
	}

	var practiceEntry QueueEntry

	err = json.Unmarshal([]byte(element), &practiceEntry)
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return nil, err
	}

	matchID := fmt.Sprintf("match_%d", time.Now().Unix())

	match := &Match{
		ID:           matchID,
		PracticeUser: practiceEntry,
		NativeUser:   nativeEntry,
		Language:     practiceEntry.PracticeLanguage,
		CreatedAt:    time.Now(),
	}

	return match, nil
}

func (s *MatchingService) findMatchInQueue(ctx context.Context, queueKey string, newUser QueueEntry) *QueueEntry {
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

		// Check for reciprocal language compatibility
		// For a perfect match:
		// - candidateUser's native language should be newUser's practice language
		// - candidateUser's practice language should be newUser's native language
		isCompatible := candidateUser.NativeLanguage == newUser.PracticeLanguage &&
			candidateUser.PracticeLanguage == newUser.NativeLanguage

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
		match.PracticeUser.UserID,
		match.NativeUser.UserID,
		match.PracticeUser.NativeLanguage,
		match.PracticeUser.PracticeLanguage,
		match.NativeUser.NativeLanguage,
		match.NativeUser.PracticeLanguage,
	)
	if err != nil {
		log.Printf("Failed to create session for match %s: %v", match.ID, err)
		return err
	}

	match.SessionID = session.ID.String()

	log.Printf("Created session %s for match %s - Language: %s", session.ID.String(), match.ID, match.Language)

	// User1 is the learner, User2 is the native speaker
	learnerNotification := websocket.MatchNotification{
		MatchID:   match.ID,
		PartnerID: match.NativeUser.UserID,
		Language:  match.Language,
		Message:   fmt.Sprintf("Match found! You'll practice %s with %s", match.Language, match.NativeUser.UserID),
	}

	nativeNotification := websocket.MatchNotification{
		MatchID:   match.ID,
		PartnerID: match.PracticeUser.UserID,
		Language:  match.Language,
		Message:   fmt.Sprintf("Match found! You'll help %s practice %s", match.PracticeUser.UserID, match.Language),
	}

	if err := s.wsManager.NotifyMatch(match.PracticeUser.UserID, learnerNotification); err != nil {
		log.Printf("Failed to notify learner %s: %v", match.PracticeUser.UserID, err)
	}

	if err := s.wsManager.NotifyMatch(match.NativeUser.UserID, nativeNotification); err != nil {
		log.Printf("Failed to notify native speaker %s: %v", match.NativeUser.UserID, err)
	}

	return nil
}
