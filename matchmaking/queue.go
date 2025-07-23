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

type QueueEntry struct {
	UserID           string    `json:"user_id"`
	NativeLanguage   string    `json:"native_language"`
	PracticeLanguage string    `json:"practice_language"`
	Timestamp        time.Time `json:"timestamp"`
}

const (
	usersDataHashKey = "users:data"
)

func (ms *MatchmakingService) InitiateMatchmaking(ctx context.Context, userID, nativeLanguage, practiceLanguage string) (*QueueEntry, error) {
	entry := QueueEntry{
		UserID:           userID,
		NativeLanguage:   nativeLanguage,
		PracticeLanguage: practiceLanguage,
		Timestamp:        time.Now(),
	}
	ms.dequeueUserByEntry(ctx, entry)

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}

	err = ms.enqueueUser(ctx, entry, entryJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue user '%s': %w", entry.UserID, err)
	}

	err = ms.pubSubManager.PublishToLanguageChannel(ctx, entry.NativeLanguage, entryJSON)
	if err != nil {
		return nil, err
	}

	return &entry, nil
}

func (ms *MatchmakingService) CancelMatchmaking(ctx context.Context, userID string) error {
	err := ms.dequeueUserByID(ctx, userID)
	if err != nil {
		return err
	}
	return nil
}

func (ms *MatchmakingService) enqueueUser(ctx context.Context, entry QueueEntry, value []byte) error {
	queueKey := "queue:" + entry.PracticeLanguage
	pipe := ms.redisClient.Pipeline()
	pipe.HSet(ctx, usersDataHashKey, entry.UserID, value)
	pipe.RPush(ctx, queueKey, entry.UserID)
	_, err := pipe.Exec(ctx)
	return err
}

func (ms *MatchmakingService) dequeueUserByID(ctx context.Context, userID string) error {
	val, err := ms.redisClient.HGet(ctx, usersDataHashKey, userID).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}

	var entry QueueEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return err
	}

	err = ms.dequeueUserByEntry(ctx, entry)
	if err != nil {
		return err
	}

	return nil
}

func (ms *MatchmakingService) dequeueUserByEntry(ctx context.Context, entry QueueEntry) error {
	queueKey := "queue:" + entry.PracticeLanguage
	pipe := ms.redisClient.Pipeline()
	pipe.LRem(ctx, queueKey, 0, entry.UserID)
	pipe.HDel(ctx, usersDataHashKey, entry.UserID)
	_, err := pipe.Exec(ctx)
	return err
}

func (ms *MatchmakingService) InitializeLanguageChannels(ctx context.Context, languages []string) error {
	return ms.pubSubManager.InitializeLanguagePublishers(languages)
}
