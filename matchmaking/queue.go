package matchmaking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient interface {
	Ping(ctx context.Context) *redis.StatusCmd
	LPop(ctx context.Context, key string) *redis.StringCmd
	RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	LLen(ctx context.Context, key string) *redis.IntCmd
	LIndex(ctx context.Context, key string, index int64) *redis.StringCmd
	LRem(ctx context.Context, key string, count int64, value interface{}) *redis.IntCmd
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
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

func NewMatchmakingService(redisClient RedisClient, pubSubManager PubSubManager) *MatchmakingService {
	return &MatchmakingService{
		redisClient:   redisClient,
		pubSubManager: pubSubManager,
	}
}

func (ms *MatchmakingService) AddToQueue(ctx context.Context, entry QueueEntry) error {
	entry.Timestamp = time.Now()

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Store user in Redis queue for their practice language (what they want to learn)
	// This allows others looking for their native language to find them
	queueKey := "queue:" + entry.PracticeLanguage
	if err := ms.redisClient.RPush(ctx, queueKey, entryJSON).Err(); err != nil {
		return err
	}

	// Publish to native language channel so others practicing that language can see them
	err = ms.pubSubManager.PublishToLanguageChannel(ctx, entry.NativeLanguage, entryJSON)
	if err != nil {
		return err
	}

	return nil
}

func (ms *MatchmakingService) RemoveFromQueue(ctx context.Context, userID string, practiceLanguage string) error {
	queueKey := "queue:" + practiceLanguage
	
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
		
		var entry QueueEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
			continue
		}
		
		// If we found the user, remove them from the queue
		if entry.UserID == userID {
			return ms.redisClient.LRem(ctx, queueKey, 1, entryJSON).Err()
		}
	}
	
	// User not found in queue - this is not an error
	return nil
}

func (ms *MatchmakingService) InitializeLanguageChannels(ctx context.Context, languages []string) error {
	return ms.pubSubManager.InitializeLanguagePublishers(languages)
}
