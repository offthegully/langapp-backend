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
	CreateSession(ctx context.Context, practiceUserID, nativeUserID, language string) (*session.Session, error)
	GetSessionByUserID(ctx context.Context, userID string) (*session.Session, error)
}

type MatchmakingService struct {
	redisClient       RedisClient
	pubSubManager     PubSubManager
	wsManager         *websocket.Manager
	sessionRepository SessionRepository
	languages         []string
}

type Match struct {
	PracticeUser QueueEntry `json:"practice_user"`
	NativeUser   QueueEntry `json:"native_user"`
	Language     string     `json:"language"`
	CreatedAt    time.Time  `json:"created_at"`
}

type MatchNotification struct {
	PartnerID string `json:"partner_id"`
	Language  string `json:"language"`
	Message   string `json:"message"`
}

func NewMatchmakingService(redisClient RedisClient, pubSubManager PubSubManager, wsManager *websocket.Manager, sessionRepository SessionRepository, languages []string) *MatchmakingService {
	return &MatchmakingService{
		redisClient:       redisClient,
		pubSubManager:     pubSubManager,
		wsManager:         wsManager,
		sessionRepository: sessionRepository,
		languages:         languages,
	}
}

func (ms *MatchmakingService) Start(ctx context.Context) {
	for _, language := range ms.languages {
		go ms.listenToLanguageChannel(ctx, language)
	}
	log.Printf("Matching service started for %d languages", len(ms.languages))
}

func (ms *MatchmakingService) listenToLanguageChannel(ctx context.Context, language string) {
	pubsub := ms.pubSubManager.SubscribeToLanguageChannel(ctx, language)
	defer pubsub.Close()

	log.Printf("Listening to channel for language: %s", language)

	ch := pubsub.Channel()
	for msg := range ch {
		var nativeEntry QueueEntry
		err := json.Unmarshal([]byte(msg.Payload), &nativeEntry)
		if err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		log.Printf("New user in %s channel: %s (native: %s, practice: %s)", language, nativeEntry.UserID, nativeEntry.NativeLanguage, nativeEntry.PracticeLanguage)

		err = ms.processMessage(ctx, nativeEntry)
		if err != nil {
			log.Printf("Error processing message: %v", err)
			continue
		}
	}
}

func (ms *MatchmakingService) processMessage(ctx context.Context, nativeEntry QueueEntry) error {
	match, err := ms.findMatch(ctx, nativeEntry)
	if err != nil {
		log.Printf("Error finding match: %v", err)
		return nil
	}

	if match != nil {
		log.Printf("Match found! %s <-> %s", match.PracticeUser.UserID, match.NativeUser.UserID)

		err := ms.notifyMatch(ctx, match)
		if err != nil {
			log.Printf("Error creating session/notifying match: %v", err)

			if restoreErr := ms.restorePracticeUserToQueue(ctx, match.PracticeUser); restoreErr != nil {
				log.Printf("Failed to restore practice user to queue: %v", restoreErr)
			}
			return nil
		}

		err = ms.removeMatchedUsers(ctx, match)
		if err != nil {
			log.Printf("Warning: Error removing matched users (session already created): %v", err)
		}
	}

	return nil
}

func (ms *MatchmakingService) findMatch(ctx context.Context, nativeEntry QueueEntry) (*Match, error) {
	language := nativeEntry.NativeLanguage
	queueKey := "queue:" + language

	userID, err := ms.redisClient.LPop(ctx, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to pop from queue '%s': %w", queueKey, err)
	}

	entryJSON, err := ms.redisClient.HGet(ctx, usersDataHashKey, userID).Result()
	if err != nil {
		return nil, fmt.Errorf("could not find data for user '%s': %w", userID, err)
	}

	if err := ms.redisClient.HDel(ctx, usersDataHashKey, userID).Err(); err != nil {
		log.Printf("Warning: failed to clean up user data for '%s': %v", userID, err)
	}

	var practiceEntry QueueEntry
	if err := json.Unmarshal([]byte(entryJSON), &practiceEntry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data for user '%s': %w", userID, err)
	}

	match := &Match{
		PracticeUser: practiceEntry,
		NativeUser:   nativeEntry,
		Language:     practiceEntry.PracticeLanguage,
		CreatedAt:    time.Now(),
	}

	return match, nil
}

func (ms *MatchmakingService) removeMatchedUsers(ctx context.Context, match *Match) error {
	var errors []error

	err := ms.removeUserFromAllQueues(ctx, match.NativeUser)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to remove native user %s: %w", match.NativeUser.UserID, err))
	}

	if err := ms.removeUserFromAllQueues(ctx, match.PracticeUser); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove practice user %s: %w", match.PracticeUser.UserID, err))
	}

	if len(errors) > 0 {
		for _, err := range errors {
			log.Printf("User removal error: %v", err)
		}
		return errors[0]
	}

	log.Printf("Successfully removed both matched users from all queues")
	return nil
}

func (ms *MatchmakingService) removeUserFromAllQueues(ctx context.Context, user QueueEntry) error {
	pipe := ms.redisClient.Pipeline()
	for _, language := range ms.languages {
		queueKey := "queue:" + language
		pipe.LRem(ctx, queueKey, 0, user.UserID) // 0 means remove all occurrences
	}

	pipe.HDel(ctx, usersDataHashKey, user.UserID)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove user %s from all queues: %w", user.UserID, err)
	}

	log.Printf("Removed user %s from all possible queues and cleaned up data", user.UserID)
	return nil
}

func (ms *MatchmakingService) restorePracticeUserToQueue(ctx context.Context, entry QueueEntry) error {
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal user data for restore: %w", err)
	}

	ms.enqueueUser(ctx, entry, entryJSON)

	log.Printf("Restored user %s to queue %s", entry.UserID, entry.PracticeLanguage)
	return nil
}

func (ms *MatchmakingService) notifyMatch(ctx context.Context, match *Match) error {
	session, err := ms.sessionRepository.CreateSession(
		ctx,
		match.PracticeUser.UserID,
		match.NativeUser.UserID,
		match.Language,
	)
	if err != nil {
		log.Printf("Failed to create session for match: %v", err)
		return err
	}

	log.Printf("Created session %s for match - Language: %s", session.ID.String(), match.Language)

	practiceUserMessage := websocket.Message{
		Type: websocket.MatchFound,
		Data: MatchNotification{
			PartnerID: match.NativeUser.UserID,
			Language:  match.Language,
			Message:   fmt.Sprintf("Match found! You'll practice %s with %s", match.Language, match.NativeUser.UserID),
		},
	}

	nativeUserMessage := websocket.Message{
		Type: websocket.MatchFound,
		Data: MatchNotification{
			PartnerID: match.PracticeUser.UserID,
			Language:  match.Language,
			Message:   fmt.Sprintf("Match found! You'll help %s practice %s", match.PracticeUser.UserID, match.Language),
		},
	}

	if err := ms.wsManager.SendMessage(match.PracticeUser.UserID, practiceUserMessage); err != nil {
		log.Printf("Failed to notify practice user %s: %v", match.PracticeUser.UserID, err)
	}

	if err := ms.wsManager.SendMessage(match.NativeUser.UserID, nativeUserMessage); err != nil {
		log.Printf("Failed to notify native user %s: %v", match.NativeUser.UserID, err)
	}

	return nil
}
