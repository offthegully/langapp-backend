package queue

import (
	"context"
	"log"
	"time"

	"langapp-backend/internal/sessions"
	"langapp-backend/internal/storage"

	"github.com/hibiken/asynq"
)

type Processor struct {
	matcher   *Matcher
	wsManager *sessions.WSManager
	storage   *storage.Storage
	server    *asynq.Server
}

func NewProcessor(storage *storage.Storage, wsManager *sessions.WSManager, redisURL string) *Processor {
	matcher := NewMatcher(storage)

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: parseRedisAddr(redisURL)},
		asynq.Config{
			Concurrency: 5,
			Queues: map[string]int{
				"matching": 6,
				"default":  3,
				"cleanup":  1,
			},
			StrictPriority: true,
		},
	)

	return &Processor{
		matcher:   matcher,
		wsManager: wsManager,
		storage:   storage,
		server:    server,
	}
}

func (p *Processor) Start(ctx context.Context) error {
	mux := asynq.NewServeMux()

	// Register task handlers
	mux.HandleFunc("matching:process", p.handleMatchingTask)
	mux.HandleFunc("cleanup:expired", p.handleCleanupTask)

	// Start the background server
	go func() {
		if err := p.server.Run(mux); err != nil {
			log.Printf("Asynq server error: %v", err)
		}
	}()

	// Start periodic matching
	go p.startPeriodicMatching(ctx)

	// Start periodic cleanup
	go p.startPeriodicCleanup(ctx)

	log.Println("Queue processor started")
	return nil
}

func (p *Processor) Stop() {
	p.server.Shutdown()
}

func (p *Processor) handleMatchingTask(ctx context.Context, task *asynq.Task) error {
	log.Println("Processing matching task...")

	matches, err := p.matcher.FindMatches(ctx)
	if err != nil {
		log.Printf("Error finding matches: %v", err)
		return err
	}

	log.Printf("Found %d matches", len(matches))

	for _, match := range matches {
		if err := p.processMatch(ctx, match); err != nil {
			log.Printf("Error processing match: %v", err)
			continue
		}
	}

	return nil
}

func (p *Processor) processMatch(ctx context.Context, match Match) error {
	// Create chat session
	session, err := p.matcher.CreateChatSession(ctx, match)
	if err != nil {
		return err
	}

	log.Printf("Created chat session %s for users %s and %s", 
		session.ID, match.UserA.UserID, match.UserB.UserID)

	// Send notifications via Redis pub/sub
	if err := p.storage.Redis.PublishMatchFound(ctx, match.UserA.UserID.String(), session.ID.String()); err != nil {
		log.Printf("Error publishing match notification for user A: %v", err)
	}

	if err := p.storage.Redis.PublishMatchFound(ctx, match.UserB.UserID.String(), session.ID.String()); err != nil {
		log.Printf("Error publishing match notification for user B: %v", err)
	}

	// Send direct WebSocket notifications if users are connected
	p.wsManager.SendMatchNotification(match.UserA.UserID.String(), session.ID.String())
	p.wsManager.SendMatchNotification(match.UserB.UserID.String(), session.ID.String())

	return nil
}

func (p *Processor) handleCleanupTask(ctx context.Context, task *asynq.Task) error {
	log.Println("Processing cleanup task...")

	// Clean up expired match requests
	// This would involve checking Redis queues for expired entries
	// and removing them

	languages, err := p.storage.Redis.GetAllQueueLanguages(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	cleanedCount := 0

	for _, lang := range languages {
		requests, err := p.storage.Redis.GetQueueMembers(ctx, lang, 1000)
		if err != nil {
			log.Printf("Error getting queue members for %s: %v", lang, err)
			continue
		}

		for _, req := range requests {
			if now.After(req.ExpiresAt) {
				if err := p.storage.Redis.RemoveFromQueue(ctx, req.UserID.String(), req.PracticeLanguage); err != nil {
					log.Printf("Error removing expired request: %v", err)
				} else {
					cleanedCount++
				}
			}
		}
	}

	if cleanedCount > 0 {
		log.Printf("Cleaned up %d expired match requests", cleanedCount)
	}

	return nil
}

func (p *Processor) startPeriodicMatching(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: "localhost:6379"})
	defer client.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Queue a matching task
			task := asynq.NewTask("matching:process", nil)
			if _, err := client.Enqueue(task, asynq.Queue("matching")); err != nil {
				log.Printf("Error enqueueing matching task: %v", err)
			}
		}
	}
}

func (p *Processor) startPeriodicCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: "localhost:6379"})
	defer client.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Queue a cleanup task
			task := asynq.NewTask("cleanup:expired", nil)
			if _, err := client.Enqueue(task, asynq.Queue("cleanup")); err != nil {
				log.Printf("Error enqueueing cleanup task: %v", err)
			}
		}
	}
}

func parseRedisAddr(redisURL string) string {
	// Extract address from Redis URL
	// For simplicity, assuming localhost:6379
	// In production, parse the full URL properly
	if redisURL == "" {
		return "localhost:6379"
	}
	
	// Simple parsing for redis://localhost:6379
	if redisURL == "redis://localhost:6379" {
		return "localhost:6379"
	}
	
	return "localhost:6379"
}