package matchmaking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient interface {
	Ping(ctx context.Context) *redis.StatusCmd
	LPop(ctx context.Context, key string) *redis.StringCmd
	LPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	LLen(ctx context.Context, key string) *redis.IntCmd
	LIndex(ctx context.Context, key string, index int64) *redis.StringCmd
	LRem(ctx context.Context, key string, count int64, value interface{}) *redis.IntCmd
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
	Pipeline() redis.Pipeliner
	HGet(ctx context.Context, key, field string) *redis.StringCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd
}

type PubSubManager interface {
	PublishToLanguageChannel(ctx context.Context, language string, message interface{}) error
	SubscribeToLanguageChannel(ctx context.Context, language string) *redis.PubSub
	InitializeLanguagePublishers(languages []string) error
}

type MatchmakingService struct {
	redisClient   RedisClient
	pubSubManager PubSubManager
}

type QueueEntry struct {
	UserID           string    `json:"user_id"`
	NativeLanguage   string    `json:"native_language"`
	PracticeLanguage string    `json:"practice_language"`
	Timestamp        time.Time `json:"timestamp"`
}

const (
	usersDataHashKey = "users:data"
)

func NewMatchmakingService(redisClient RedisClient, pubSubManager PubSubManager) *MatchmakingService {
	return &MatchmakingService{
		redisClient:   redisClient,
		pubSubManager: pubSubManager,
	}
}

func (ms *MatchmakingService) InitiateMatchmaking(ctx context.Context, userID, nativeLanguage, practiceLanguage string) (QueueEntry, error) {
	entry := QueueEntry{
		UserID:           userID,
		NativeLanguage:   nativeLanguage,
		PracticeLanguage: practiceLanguage,
		Timestamp:        time.Now(),
	}

	// Check if user is already in queue
	ms.dequeueUser(ctx, entry)

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return entry, err
	}

	// Store user in Redis queue for their practice language (what they want to learn)
	err = ms.enqueueUser(ctx, entry, entryJSON)
	if err != nil {
		return entry, fmt.Errorf("failed to enqueue user '%s': %w", entry.UserID, err)
	}

	// Publish to native language channel so others practicing that language can see them
	err = ms.pubSubManager.PublishToLanguageChannel(ctx, entry.NativeLanguage, entryJSON)
	if err != nil {
		return entry, err
	}

	return entry, nil
}

func (ms *MatchmakingService) CancelMatchmaking(ctx context.Context, userID string) error {
	return nil
}

func (ms *MatchmakingService) enqueueUser(ctx context.Context, entry QueueEntry, value []byte) error {
	queueKey := "queue:" + entry.PracticeLanguage
	pipe := ms.redisClient.Pipeline()
	pipe.HSet(ctx, usersDataHashKey, entry.UserID, value) // Store data in the hash.
	pipe.RPush(ctx, queueKey, entry.UserID)               // Store ID in the list (queue).
	_, err := pipe.Exec(ctx)
	return err
}

func (ms *MatchmakingService) dequeueUser(ctx context.Context, entry QueueEntry) error {
	queueKey := "queue:" + entry.PracticeLanguage

	// Get all entries in the queue to find the user
	queueLength, err := ms.redisClient.LLen(ctx, queueKey).Result()
	if err != nil {
		return err
	}

	// Search through the queue to find the user
	for i := int64(0); i < queueLength; i++ {
		entryJSON, err := ms.redisClient.LIndex(ctx, queueKey, i).Result()
		if err != nil {
			continue
		}

		var storedEntry QueueEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
			continue
		}

		// If we found the user, remove them from the queue
		if storedEntry.UserID == entry.UserID {
			return ms.redisClient.LRem(ctx, queueKey, 1, entryJSON).Err()
		}
	}

	// User not found in queue - this is not an error
	return nil
}

func (ms *MatchmakingService) InitializeLanguageChannels(ctx context.Context, languages []string) error {
	return ms.pubSubManager.InitializeLanguagePublishers(languages)
}
