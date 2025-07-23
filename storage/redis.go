package storage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type PubSubManager struct {
	client     *redis.Client
	publishers map[string]*redis.Client
}

func NewRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
}

func NewPubSubManager(client *redis.Client) *PubSubManager {
	return &PubSubManager{
		client:     client,
		publishers: make(map[string]*redis.Client),
	}
}

func (psm *PubSubManager) InitializeLanguagePublishers(languages []string) error {
	for _, language := range languages {
		channelName := fmt.Sprintf("matchmaking:%s", language)
		psm.publishers[channelName] = psm.client
	}
	return nil
}

func (psm *PubSubManager) PublishToLanguageChannel(ctx context.Context, language string, message interface{}) error {
	channelName := fmt.Sprintf("matchmaking:%s", language)
	return psm.client.Publish(ctx, channelName, message).Err()
}

func (psm *PubSubManager) SubscribeToLanguageChannel(ctx context.Context, language string) *redis.PubSub {
	channelName := fmt.Sprintf("matchmaking:%s", language)
	return psm.client.Subscribe(ctx, channelName)
}
