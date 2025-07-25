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
			// TODO - maybe we just remove the user from matchmaking here, will have to decide
			// Restore the practice user back to the queue since session creation failed
			if restoreErr := ms.restoreUserFromHold(ctx, practiceEntry.UserID, nativeEntry.NativeLanguage); restoreErr != nil {
				log.Printf("Failed to restore user %s from hold after session creation failure: %v", practiceEntry.UserID, restoreErr)
			}
			return fmt.Errorf("error initializing session after finding match: %v", err)
		}

		// Session created successfully, release the practice user from hold
		if releaseErr := ms.releaseUserFromHold(ctx, practiceEntry.UserID, nativeEntry.NativeLanguage); releaseErr != nil {
			log.Printf("Warning: failed to release user %s from hold after successful match: %v", practiceEntry.UserID, releaseErr)
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

	// Get the next user from the queue without removing them yet
	userID, err := ms.redisClient.LIndex(ctx, queueKey, 0).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			log.Printf("No user in '%s' queue on pop", language)
			return nil, nil // No users in queue
		}
		return nil, fmt.Errorf("failed to peek queue '%s': %w", queueKey, err)
	}

	// Put the user on hold (this atomically removes from queue and places in hold)
	practiceEntry, err := ms.putUserOnHold(ctx, userID, language)
	if err != nil {
		return nil, fmt.Errorf("failed to put user on hold: %w", err)
	}

	if practiceEntry == nil {
		log.Printf("No user in queue on pop, %s", userID)
		return nil, nil // No user was available (race condition)
	}

	return practiceEntry, nil
}
