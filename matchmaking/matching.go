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
			s.Remove(ctx, language, match.NativeUser.UserID)

			log.Printf("Match found! %s <-> %s", match.PracticeUser.UserID, match.NativeUser.UserID)
			if err := s.notifyMatch(match); err != nil {
				log.Printf("Error notifying match: %v", err)
			}
		}
	}
}

func (s *MatchingService) findMatch(ctx context.Context, nativeEntry QueueEntry) (*Match, error) {
	language := nativeEntry.NativeLanguage
	queueKey := "queue:" + language

	// 1. Pop the next user ID from the left of the list (FIFO).
	userID, err := s.redisClient.LPop(ctx, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// This is an expected error when the queue is empty.
			return nil, err
		}
		return nil, fmt.Errorf("failed to pop from queue '%s': %w", queueKey, err)
	}

	// 2. Get the user's data from the hash.
	entryJSON, err := s.redisClient.HGet(ctx, usersDataHashKey, userID).Result()
	if err != nil {
		// If data is missing for some reason, return an error.
		return nil, fmt.Errorf("could not find data for user '%s': %w", userID, err)
	}

	// 3. Clean up the user's data from the hash.
	if err := s.redisClient.HDel(ctx, usersDataHashKey, userID).Err(); err != nil {
		log.Printf("Warning: failed to clean up user data for '%s': %v", userID, err)
		// Continue anyway since we have the data we need
	}

	// Unmarshal the data and return it.
	var practiceEntry QueueEntry
	if err := json.Unmarshal([]byte(entryJSON), &practiceEntry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data for user '%s': %w", userID, err)
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

func (s *MatchingService) Remove(ctx context.Context, language, userID string) error {
	queueKey := "queue:" + language

	// Use a pipeline for efficiency.
	pipe := s.redisClient.Pipeline()
	// Command to remove the user ID from the queue list.
	lremResult := pipe.LRem(ctx, queueKey, 1, userID)
	// Command to delete the user data from the hash.
	pipe.HDel(ctx, usersDataHashKey, userID)
	_, err := pipe.Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to execute removal for user '%s': %w", userID, err)
	}

	// Check if the user was actually found and removed from the list.
	if lremResult.Val() == 0 {
		return fmt.Errorf("user '%s' not found in queue '%s'", userID, language)
	}

	log.Printf("Removed user '%s' from language '%s'", userID, language)
	return nil
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
