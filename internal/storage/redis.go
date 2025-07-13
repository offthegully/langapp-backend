package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(ctx context.Context, redisURL string) (*RedisClient, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisClient{client: client}, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Queue operations
func (r *RedisClient) AddToQueue(ctx context.Context, req *MatchRequest) error {
	start := time.Now()
	operationID := fmt.Sprintf("redis_add_%d_%s", time.Now().UnixNano(), req.UserID.String()[:8])
	
	log.Printf("[REDIS_ADD] %s - Adding match request to queue: UserID=%s, Language=%s", 
		operationID, req.UserID, req.PracticeLanguage)
	
	marshalStart := time.Now()
	data, err := json.Marshal(req)
	marshalDuration := time.Since(marshalStart)
	if err != nil {
		log.Printf("[REDIS_ADD] %s - Failed to marshal request after %v: %v", 
			operationID, marshalDuration, err)
		return err
	}
	log.Printf("[REDIS_ADD] %s - Marshaled request in %v, size: %d bytes", 
		operationID, marshalDuration, len(data))

	// Add to sorted set with timestamp as score for FIFO processing
	score := float64(req.RequestedAt.Unix())
	key := fmt.Sprintf("queue:%s", req.PracticeLanguage)
	
	log.Printf("[REDIS_ADD] %s - Adding to Redis sorted set: key=%s, score=%f", 
		operationID, key, score)
	
	redisStart := time.Now()
	err = r.client.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: string(data),
	}).Err()
	redisDuration := time.Since(redisStart)
	totalDuration := time.Since(start)
	
	if err != nil {
		log.Printf("[REDIS_ADD] %s - Failed to add to Redis after %v: %v", 
			operationID, redisDuration, err)
		return err
	}
	
	log.Printf("[REDIS_ADD] %s - Successfully added to queue in %v (Redis: %v, Marshal: %v)", 
		operationID, totalDuration, redisDuration, marshalDuration)
	log.Printf("[REDIS_ADD_METRICS] OperationID=%s UserID=%s Language=%s Duration=%v RedisDuration=%v DataSize=%d", 
		operationID, req.UserID, req.PracticeLanguage, totalDuration, redisDuration, len(data))
	
	return nil
}

func (r *RedisClient) RemoveFromQueue(ctx context.Context, userID, practiceLanguage string) error {
	start := time.Now()
	operationID := fmt.Sprintf("redis_remove_%d_%s", time.Now().UnixNano(), userID[:8])
	
	log.Printf("[REDIS_REMOVE] %s - Removing user %s from queue: %s", 
		operationID, userID, practiceLanguage)
	
	key := fmt.Sprintf("queue:%s", practiceLanguage)
	
	// Get all members and remove those matching userID
	getStart := time.Now()
	members, err := r.client.ZRange(ctx, key, 0, -1).Result()
	getDuration := time.Since(getStart)
	if err != nil {
		log.Printf("[REDIS_REMOVE] %s - Failed to get queue members after %v: %v", 
			operationID, getDuration, err)
		return err
	}
	
	log.Printf("[REDIS_REMOVE] %s - Retrieved %d members from queue in %v", 
		operationID, len(members), getDuration)

	for i, member := range members {
		unmarshalStart := time.Now()
		var req MatchRequest
		if err := json.Unmarshal([]byte(member), &req); err != nil {
			log.Printf("[REDIS_REMOVE] %s - Failed to unmarshal member %d after %v: %v", 
				operationID, i, time.Since(unmarshalStart), err)
			continue
		}
		
		if req.UserID.String() == userID {
			log.Printf("[REDIS_REMOVE] %s - Found matching user at position %d", operationID, i)
			removeStart := time.Now()
			err := r.client.ZRem(ctx, key, member).Err()
			removeDuration := time.Since(removeStart)
			totalDuration := time.Since(start)
			
			if err != nil {
				log.Printf("[REDIS_REMOVE] %s - Failed to remove member after %v: %v", 
					operationID, removeDuration, err)
				return err
			}
			
			log.Printf("[REDIS_REMOVE] %s - Successfully removed user in %v (total: %v, get: %v, remove: %v)", 
				operationID, removeDuration, totalDuration, getDuration, removeDuration)
			log.Printf("[REDIS_REMOVE_METRICS] OperationID=%s UserID=%s Language=%s Duration=%v Found=true Position=%d", 
				operationID, userID, practiceLanguage, totalDuration, i)
			return nil
		}
	}

	totalDuration := time.Since(start)
	log.Printf("[REDIS_REMOVE] %s - User not found in queue after %v (checked %d members)", 
		operationID, totalDuration, len(members))
	log.Printf("[REDIS_REMOVE_METRICS] OperationID=%s UserID=%s Language=%s Duration=%v Found=false MembersChecked=%d", 
		operationID, userID, practiceLanguage, totalDuration, len(members))
	
	return nil
}

func (r *RedisClient) GetQueueMembers(ctx context.Context, practiceLanguage string, limit int64) ([]MatchRequest, error) {
	key := fmt.Sprintf("queue:%s", practiceLanguage)
	
	members, err := r.client.ZRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	requests := make([]MatchRequest, 0, len(members))
	for _, member := range members {
		var req MatchRequest
		if err := json.Unmarshal([]byte(member), &req); err != nil {
			continue
		}
		requests = append(requests, req)
	}

	return requests, nil
}

func (r *RedisClient) GetAllQueueLanguages(ctx context.Context) ([]string, error) {
	keys, err := r.client.Keys(ctx, "queue:*").Result()
	if err != nil {
		return nil, err
	}

	languages := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key) > 6 { // "queue:" prefix
			languages = append(languages, key[6:])
		}
	}

	return languages, nil
}

// Session management
func (r *RedisClient) SetSessionStatus(ctx context.Context, sessionID, status string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.client.HSet(ctx, key, "status", status).Err()
}

func (r *RedisClient) GetSessionStatus(ctx context.Context, sessionID string) (string, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.client.HGet(ctx, key, "status").Result()
}

func (r *RedisClient) SetSessionUsers(ctx context.Context, sessionID string, userAID, userBID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.client.HMSet(ctx, key, map[string]interface{}{
		"user_a": userAID,
		"user_b": userBID,
	}).Err()
}

func (r *RedisClient) ExpireSession(ctx context.Context, sessionID string, expiration time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.client.Expire(ctx, key, expiration).Err()
}

// Pub/Sub for real-time notifications
func (r *RedisClient) PublishMatchFound(ctx context.Context, userID, sessionID string) error {
	channel := fmt.Sprintf("user:%s:matches", userID)
	message := map[string]string{
		"type":       "match_found",
		"session_id": sessionID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return r.client.Publish(ctx, channel, data).Err()
}

type RedisSubscriber struct {
	*redis.PubSub
}

func (rs *RedisSubscriber) ReceiveMessage(ctx context.Context) (*redis.Message, error) {
	return rs.PubSub.ReceiveMessage(ctx)
}

func (r *RedisClient) SubscribeToUserEvents(ctx context.Context, userID string) *RedisSubscriber {
	channel := fmt.Sprintf("user:%s:matches", userID)
	pubsub := r.client.Subscribe(ctx, channel)
	return &RedisSubscriber{PubSub: pubsub}
}