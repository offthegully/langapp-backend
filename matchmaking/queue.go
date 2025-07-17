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

	queueKey := "queue:" + entry.PracticeLanguage
	if err := ms.redisClient.RPush(ctx, queueKey, entryJSON).Err(); err != nil {
		return err
	}

	return ms.pubSubManager.PublishToLanguageChannel(ctx, entry.PracticeLanguage, entryJSON)
}

func (ms *MatchmakingService) InitializeLanguageChannels(ctx context.Context, languages []string) error {
	return ms.pubSubManager.InitializeLanguagePublishers(languages)
}
