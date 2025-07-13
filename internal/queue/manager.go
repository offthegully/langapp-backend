package queue

import (
	"context"
	"fmt"
	"log"
	"time"

	"langapp-backend/internal/storage"

	"github.com/google/uuid"
)

type Manager struct {
	storage *storage.Storage
	timeout time.Duration
}

func NewManager(storage *storage.Storage, timeout time.Duration) *Manager {
	return &Manager{
		storage: storage,
		timeout: timeout,
	}
}

type QueueRequest struct {
	UserID           uuid.UUID `json:"user_id"`
	NativeLanguages  []string  `json:"native_languages"`
	PracticeLanguage string    `json:"practice_language"`
}

type QueueResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (m *Manager) AddToQueue(ctx context.Context, req QueueRequest) (*QueueResponse, error) {
	start := time.Now()
	operationID := fmt.Sprintf("add_%d_%s", time.Now().UnixNano(), req.UserID.String()[:8])
	
	log.Printf("[QUEUE_ADD] %s - Adding user %s to queue for practice language: %s, native languages: %v", 
		operationID, req.UserID, req.PracticeLanguage, req.NativeLanguages)
	
	// Remove any existing requests for this user first
	removalStart := time.Now()
	if err := m.RemoveUserFromAllQueues(ctx, req.UserID.String()); err != nil {
		log.Printf("[QUEUE_ADD] %s - Warning: Error removing user from existing queues (continuing anyway): %v", operationID, err)
	} else {
		log.Printf("[QUEUE_ADD] %s - Successfully cleaned existing user requests in %v", operationID, time.Since(removalStart))
	}

	// Create match request
	requestID := uuid.New()
	now := time.Now().UTC()
	matchReq := &storage.MatchRequest{
		ID:               requestID,
		UserID:           req.UserID,
		NativeLanguages:  req.NativeLanguages,
		PracticeLanguage: req.PracticeLanguage,
		RequestedAt:      now,
		ExpiresAt:        now.Add(m.timeout),
		Status:           storage.MatchPending,
	}
	
	log.Printf("[QUEUE_ADD] %s - Created match request: ID=%s, ExpiresAt=%s, Timeout=%v", 
		operationID, requestID, matchReq.ExpiresAt.Format(time.RFC3339), m.timeout)

	// Add to Redis queue
	redisStart := time.Now()
	log.Printf("[QUEUE_ADD] %s - Adding to Redis queue for language: %s", operationID, req.PracticeLanguage)
	if err := m.storage.Redis.AddToQueue(ctx, matchReq); err != nil {
		log.Printf("[QUEUE_ADD] %s - Failed to add to Redis queue after %v: %v", 
			operationID, time.Since(redisStart), err)
		return nil, fmt.Errorf("failed to add to queue: %w", err)
	}
	redisAddDuration := time.Since(redisStart)
	log.Printf("[QUEUE_ADD] %s - Successfully added to Redis queue in %v", operationID, redisAddDuration)

	totalDuration := time.Since(start)
	log.Printf("[QUEUE_ADD] %s - Operation completed successfully in %v (Redis: %v, Cleanup: %v)", 
		operationID, totalDuration, redisAddDuration, time.Since(removalStart))
	
	// Log metrics for monitoring
	log.Printf("[QUEUE_ADD_METRICS] OperationID=%s UserID=%s PracticeLanguage=%s NativeLanguages=%v Duration=%v RedisAddDuration=%v RequestID=%s", 
		operationID, req.UserID, req.PracticeLanguage, req.NativeLanguages, totalDuration, redisAddDuration, requestID)
	
	return &QueueResponse{
		RequestID: matchReq.ID.String(),
		Status:    storage.MatchPending,
		ExpiresAt: matchReq.ExpiresAt,
	}, nil
}

func (m *Manager) RemoveFromQueue(ctx context.Context, userID, practiceLanguage string) error {
	start := time.Now()
	operationID := fmt.Sprintf("remove_%d_%s", time.Now().UnixNano(), userID[:8])
	
	log.Printf("[QUEUE_REMOVE] %s - Removing user %s from queue for language: %s", 
		operationID, userID, practiceLanguage)
	
	err := m.storage.Redis.RemoveFromQueue(ctx, userID, practiceLanguage)
	duration := time.Since(start)
	
	if err != nil {
		log.Printf("[QUEUE_REMOVE] %s - Failed to remove user after %v: %v", 
			operationID, duration, err)
		return err
	}
	
	log.Printf("[QUEUE_REMOVE] %s - Successfully removed user in %v", operationID, duration)
	log.Printf("[QUEUE_REMOVE_METRICS] OperationID=%s UserID=%s PracticeLanguage=%s Duration=%v", 
		operationID, userID, practiceLanguage, duration)
	
	return nil
}

func (m *Manager) RemoveUserFromAllQueues(ctx context.Context, userID string) error {
	start := time.Now()
	operationID := fmt.Sprintf("removeall_%d_%s", time.Now().UnixNano(), userID[:8])
	
	log.Printf("[QUEUE_REMOVEALL] %s - Removing user %s from all queues", operationID, userID)
	
	languagesStart := time.Now()
	languages, err := m.storage.Redis.GetAllQueueLanguages(ctx)
	languagesDuration := time.Since(languagesStart)
	if err != nil {
		log.Printf("[QUEUE_REMOVEALL] %s - Failed to get queue languages after %v: %v", 
			operationID, languagesDuration, err)
		return err
	}
	
	log.Printf("[QUEUE_REMOVEALL] %s - Found %d languages to check in %v: %v", 
		operationID, len(languages), languagesDuration, languages)
	
	removedCount := 0
	for _, lang := range languages {
		removeStart := time.Now()
		if err := m.storage.Redis.RemoveFromQueue(ctx, userID, lang); err != nil {
			log.Printf("[QUEUE_REMOVEALL] %s - Error removing user from queue %s after %v: %v", 
				operationID, lang, time.Since(removeStart), err)
		} else {
			removedCount++
			log.Printf("[QUEUE_REMOVEALL] %s - Removed user from queue %s in %v", 
				operationID, lang, time.Since(removeStart))
		}
	}
	
	totalDuration := time.Since(start)
	log.Printf("[QUEUE_REMOVEALL] %s - Completed removal from %d/%d queues in %v", 
		operationID, removedCount, len(languages), totalDuration)
	log.Printf("[QUEUE_REMOVEALL_METRICS] OperationID=%s UserID=%s LanguagesChecked=%d RemovedCount=%d Duration=%v", 
		operationID, userID, len(languages), removedCount, totalDuration)
	
	return nil
}

func (m *Manager) GetQueueStatus(ctx context.Context, userID string) (map[string]int, error) {
	start := time.Now()
	operationID := fmt.Sprintf("status_%d_%s", time.Now().UnixNano(), userID[:8])
	
	log.Printf("[QUEUE_STATUS] %s - Getting queue status for user %s", operationID, userID)
	
	languagesStart := time.Now()
	languages, err := m.storage.Redis.GetAllQueueLanguages(ctx)
	languagesDuration := time.Since(languagesStart)
	if err != nil {
		log.Printf("[QUEUE_STATUS] %s - Failed to get queue languages after %v: %v", 
			operationID, languagesDuration, err)
		return nil, err
	}
	
	log.Printf("[QUEUE_STATUS] %s - Found %d queue languages in %v: %v", 
		operationID, len(languages), languagesDuration, languages)
	
	status := make(map[string]int)
	totalMembers := 0
	processedLanguages := 0
	
	for _, lang := range languages {
		memberStart := time.Now()
		members, err := m.storage.Redis.GetQueueMembers(ctx, lang, 1000)
		memberDuration := time.Since(memberStart)
		
		if err != nil {
			log.Printf("[QUEUE_STATUS] %s - Error getting members for queue %s after %v: %v", 
				operationID, lang, memberDuration, err)
			continue
		}
		
		processedLanguages++
		status[lang] = len(members)
		totalMembers += len(members)
		
		log.Printf("[QUEUE_STATUS] %s - Queue %s has %d members (retrieved in %v)", 
			operationID, lang, len(members), memberDuration)
	}
	
	totalDuration := time.Since(start)
	log.Printf("[QUEUE_STATUS] %s - Status collection completed in %v: %d languages processed, %d total members", 
		operationID, totalDuration, processedLanguages, totalMembers)
	log.Printf("[QUEUE_STATUS_METRICS] OperationID=%s UserID=%s Duration=%v LanguagesProcessed=%d TotalMembers=%d", 
		operationID, userID, totalDuration, processedLanguages, totalMembers)
	
	return status, nil
}