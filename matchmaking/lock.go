package matchmaking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	holdSetKeyPrefix  = "hold:"
	holdDataKeyPrefix = "hold:data:"
	holdTTL           = 30 * time.Second // TTL for hold states to prevent stuck users
)

// putUserOnHold atomically moves a user from the queue to hold state
func (ms *MatchmakingService) putUserOnHold(ctx context.Context, userID, language string) (*QueueEntry, error) {
	queueKey := "queue:" + language
	holdSetKey := holdSetKeyPrefix + language
	holdDataKey := holdDataKeyPrefix + userID

	// First, try to pop the user from the queue
	userIDFromQueue, err := ms.redisClient.LPop(ctx, queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			log.Printf("No user in queue on pop, %s", userID)
			return nil, nil // No user in queue
		}
		return nil, fmt.Errorf("failed to pop from queue '%s': %w", queueKey, err)
	}

	// Verify this is the expected user (prevent race conditions)
	if userIDFromQueue != userID {
		// Put the user back at the front of the queue if it's not the expected user
		if pushErr := ms.redisClient.LPush(ctx, queueKey, userIDFromQueue).Err(); pushErr != nil {
			// Log error but don't return it as the main operation failed
			fmt.Printf("Warning: failed to restore user '%s' to queue after mismatch: %v", userIDFromQueue, pushErr)
		}
		return nil, fmt.Errorf("queue race condition detected: expected user '%s', got '%s'", userID, userIDFromQueue)
	}

	// Get user data from the main hash
	entryJSON, err := ms.redisClient.HGet(ctx, usersDataHashKey, userID).Result()
	if err != nil {
		// Restore user to queue since we couldn't get their data
		if pushErr := ms.redisClient.LPush(ctx, queueKey, userID).Err(); pushErr != nil {
			fmt.Printf("Warning: failed to restore user '%s' to queue after data fetch error: %v", userID, pushErr)
		}
		return nil, fmt.Errorf("could not find data for user '%s': %w", userID, err)
	}

	// Parse the user data
	var entry QueueEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		// Restore user to queue since we couldn't parse their data
		if pushErr := ms.redisClient.LPush(ctx, queueKey, userID).Err(); pushErr != nil {
			fmt.Printf("Warning: failed to restore user '%s' to queue after parse error: %v", userID, pushErr)
		}
		return nil, fmt.Errorf("failed to unmarshal data for user '%s': %w", userID, err)
	}

	// Atomically put user in hold state with TTL
	pipe := ms.redisClient.Pipeline()
	pipe.SAdd(ctx, holdSetKey, userID)
	pipe.Expire(ctx, holdSetKey, holdTTL)
	pipe.HSet(ctx, holdDataKey, "data", entryJSON)
	pipe.Expire(ctx, holdDataKey, holdTTL)
	_, err = pipe.Exec(ctx)
	if err != nil {
		// Restore user to queue since hold operation failed
		if pushErr := ms.redisClient.LPush(ctx, queueKey, userID).Err(); pushErr != nil {
			fmt.Printf("Warning: failed to restore user '%s' to queue after hold operation failure: %v", userID, pushErr)
		}
		return nil, fmt.Errorf("failed to put user '%s' on hold: %w", userID, err)
	}

	return &entry, nil
}

// releaseUserFromHold removes a user from hold state after successful matching
func (ms *MatchmakingService) releaseUserFromHold(ctx context.Context, userID, language string) error {
	holdSetKey := holdSetKeyPrefix + language
	holdDataKey := holdDataKeyPrefix + userID

	// Atomically remove user from hold state and main user data
	pipe := ms.redisClient.Pipeline()
	pipe.SRem(ctx, holdSetKey, userID)
	pipe.Del(ctx, holdDataKey)
	pipe.HDel(ctx, usersDataHashKey, userID)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to release user '%s' from hold: %w", userID, err)
	}

	return nil
}

// restoreUserFromHold moves a user back from hold state to the queue
func (ms *MatchmakingService) restoreUserFromHold(ctx context.Context, userID, language string) error {
	holdSetKey := holdSetKeyPrefix + language
	holdDataKey := holdDataKeyPrefix + userID
	queueKey := "queue:" + language

	// Get user data from hold
	entryJSON, err := ms.redisClient.HGet(ctx, holdDataKey, "data").Result()
	if err != nil {
		if err == redis.Nil {
			// User not in hold, try to clean up from hold set anyway
			ms.redisClient.SRem(ctx, holdSetKey, userID)
			return nil
		}
		return fmt.Errorf("could not find hold data for user '%s': %w", userID, err)
	}

	// Parse the user data to validate
	var entry QueueEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		return fmt.Errorf("failed to unmarshal hold data for user '%s': %w", userID, err)
	}

	// Atomically restore user to queue and remove from hold
	pipe := ms.redisClient.Pipeline()
	pipe.RPush(ctx, queueKey, userID) // Put back at end of queue
	pipe.SRem(ctx, holdSetKey, userID)
	pipe.Del(ctx, holdDataKey)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to restore user '%s' from hold to queue: %w", userID, err)
	}

	return nil
}
