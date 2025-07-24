package matchmaking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

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
	practiceEntry, err := ms.findMatch(ctx, nativeEntry)
	if err != nil {
		log.Printf("Error finding match: %v", err)
		return fmt.Errorf("error finding match: %v", err)
	}

	if practiceEntry != nil {
		log.Printf("Match found! %s <-> %s practicing %s", nativeEntry.UserID, practiceEntry.UserID, nativeEntry.NativeLanguage)
		err = ms.initializeSession(ctx, nativeEntry, *practiceEntry)
		if err != nil {
			return fmt.Errorf("error initializing session after finding match: %v", err)
		}
	}

	log.Printf("Match not found for user %s", nativeEntry.UserID)
	return nil
}

func (ms *MatchmakingService) initializeSession(ctx context.Context, nativeEntry, practiceEntry QueueEntry) error {
	language := nativeEntry.NativeLanguage
	session, err := ms.sessionRepository.CreateSession(
		ctx,
		practiceEntry.UserID,
		nativeEntry.UserID,
		language,
	)
	if err != nil {
		log.Printf("Failed to create session for match: %v", err)
		return err
	}

	log.Printf("Created session %s for match - Language: %s", session.ID.String(), language)

	practiceUserMessage := websocket.Message{
		Type: websocket.MatchFound,
		Data: MatchNotification{
			PartnerID: nativeEntry.UserID,
			Language:  language,
			Message:   fmt.Sprintf("Match found! You'll practice %s with %s", language, nativeEntry.UserID),
		},
	}

	nativeUserMessage := websocket.Message{
		Type: websocket.MatchFound,
		Data: MatchNotification{
			PartnerID: practiceEntry.UserID,
			Language:  language,
			Message:   fmt.Sprintf("Match found! You'll help %s practice %s", practiceEntry.UserID, language),
		},
	}

	if err := ms.wsManager.SendMessage(practiceEntry.UserID, practiceUserMessage); err != nil {
		log.Printf("Failed to notify practice user %s: %v", practiceEntry.UserID, err)
	}

	if err := ms.wsManager.SendMessage(practiceEntry.UserID, nativeUserMessage); err != nil {
		log.Printf("Failed to notify native user %s: %v", nativeEntry.UserID, err)
	}

	return nil
}

func (ms *MatchmakingService) findMatch(ctx context.Context, nativeEntry QueueEntry) (*QueueEntry, error) {
	language := nativeEntry.NativeLanguage
	queueKey := "queue:" + language

	userID, err := ms.redisClient.LPop(ctx, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
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

	return &practiceEntry, nil
}
