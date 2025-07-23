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
		if err := json.Unmarshal([]byte(msg.Payload), &nativeEntry); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		} // TODO - maybe remove from the channel?

		log.Printf("New user in %s channel: %s (native: %s, practice: %s)", language, nativeEntry.UserID, nativeEntry.NativeLanguage, nativeEntry.PracticeLanguage)

		match, err := ms.findMatch(ctx, nativeEntry)
		if err != nil {
			log.Printf("Error finding match: %v", err)
			continue
		}

		if match != nil {
			log.Printf("Match found! %s <-> %s", match.PracticeUser.UserID, match.NativeUser.UserID)

			// First create the session to ensure it's valid before removing users
			if err := ms.notifyMatch(match); err != nil {
				log.Printf("Error creating session/notifying match: %v", err)
				// If session creation fails, try to restore the practice user to queue
				if restoreErr := ms.restorePracticeUserToQueue(ctx, match.PracticeUser); restoreErr != nil {
					log.Printf("Failed to restore practice user to queue: %v", restoreErr)
				}
				continue
			}

			// Only after successful session creation, remove both users from all queues
			if err := ms.removeMatchedUsers(ctx, match); err != nil {
				log.Printf("Warning: Error removing matched users (session already created): %v", err)
				// Don't fail here since the session is already created and users notified
			}
		}
	}
}

func (ms *MatchmakingService) findMatch(ctx context.Context, nativeEntry QueueEntry) (*Match, error) {
	language := nativeEntry.NativeLanguage
	queueKey := "queue:" + language

	// 1. Pop the next user ID from the left of the list (FIFO).
	userID, err := ms.redisClient.LPop(ctx, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// This is an expected error when the queue is empty.
			return nil, err
		}
		return nil, fmt.Errorf("failed to pop from queue '%s': %w", queueKey, err)
	}

	// 2. Get the user's data from the hash.
	entryJSON, err := ms.redisClient.HGet(ctx, usersDataHashKey, userID).Result()
	if err != nil {
		// If data is missing for some reason, return an error.
		return nil, fmt.Errorf("could not find data for user '%s': %w", userID, err)
	}

	// 3. Clean up the user's data from the hash.
	if err := ms.redisClient.HDel(ctx, usersDataHashKey, userID).Err(); err != nil {
		log.Printf("Warning: failed to clean up user data for '%s': %v", userID, err)
		// Continue anyway since we have the data we need
	}

	// Unmarshal the data and return it.
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

func (ms *MatchmakingService) Remove(ctx context.Context, language, userID string) error {
	queueKey := "queue:" + language

	// Use a pipeline for efficiency.
	pipe := ms.redisClient.Pipeline()
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

// removeMatchedUsers removes both users from all possible queues and cleans up their data
func (ms *MatchmakingService) removeMatchedUsers(ctx context.Context, match *Match) error {
	var errors []error

	// Remove the native user from their queue (they were listening on pubsub)
	// The native user would be in the queue for their practice language
	if err := ms.removeUserFromAllQueues(ctx, match.NativeUser); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove native user %s: %w", match.NativeUser.UserID, err))
	}

	// The practice user was already popped from their queue in findMatch,
	// but we should ensure they're cleaned up from the hash and any other queues
	if err := ms.removeUserFromAllQueues(ctx, match.PracticeUser); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove practice user %s: %w", match.PracticeUser.UserID, err))
	}

	if len(errors) > 0 {
		// Log all errors but return the first one
		for _, err := range errors {
			log.Printf("User removal error: %v", err)
		}
		return errors[0]
	}

	log.Printf("Successfully removed both matched users from all queues")
	return nil
}

// removeUserFromAllQueues removes a user from all possible language queues and hash data
func (ms *MatchmakingService) removeUserFromAllQueues(ctx context.Context, user QueueEntry) error {
	pipe := ms.redisClient.Pipeline()

	// Remove from all possible language queues
	// A user could potentially be in multiple queues if they were added multiple times
	for _, language := range ms.languages {
		queueKey := "queue:" + language
		pipe.LRem(ctx, queueKey, 0, user.UserID) // 0 means remove all occurrences
	}

	// Remove user data from hash
	pipe.HDel(ctx, usersDataHashKey, user.UserID)

	// Execute all commands
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove user %s from all queues: %w", user.UserID, err)
	}

	log.Printf("Removed user %s from all possible queues and cleaned up data", user.UserID)
	return nil
}

// restorePracticeUserToQueue restores a practice user back to their queue if session creation fails
func (ms *MatchmakingService) restorePracticeUserToQueue(ctx context.Context, user QueueEntry) error {
	// Re-marshal the user data
	entryJSON, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user data for restore: %w", err)
	}

	// Use pipeline to restore both hash data and queue position
	pipe := ms.redisClient.Pipeline()

	// Restore user data to hash
	pipe.HSet(ctx, usersDataHashKey, user.UserID, entryJSON)

	// Add user back to the front of their practice language queue (since they were already waiting)
	queueKey := "queue:" + user.PracticeLanguage
	pipe.LPush(ctx, queueKey, user.UserID)

	// Execute the pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to restore user %s to queue: %w", user.UserID, err)
	}

	log.Printf("Restored user %s to queue %s", user.UserID, queueKey)
	return nil
}

func (ms *MatchmakingService) notifyMatch(match *Match) error {
	// Create session in database
	ctx := context.Background()
	session, err := ms.sessionRepository.CreateSession(ctx,
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
