package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"langapp-backend/internal/api"
	"langapp-backend/internal/api/handlers"
	"langapp-backend/internal/config"
	"langapp-backend/internal/queue"
	"langapp-backend/internal/sessions"
	"langapp-backend/internal/storage"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// Initialize storage
	storage, err := storage.NewStorage(ctx, cfg.Database.URL, cfg.Redis.URL)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()

	// Initialize queue manager
	queueManager := queue.NewManager(storage, cfg.Queue.DefaultTimeout)

	// Initialize WebSocket manager
	wsManager := sessions.NewWSManager(storage)

	// Initialize background processor
	processor := queue.NewProcessor(storage, wsManager, cfg.Redis.URL)
	if err := processor.Start(ctx); err != nil {
		log.Fatalf("Failed to start queue processor: %v", err)
	}
	defer processor.Stop()

	// Initialize handlers
	matchHandler := handlers.NewMatchHandler(queueManager)

	// Initialize dependencies
	deps := &api.Dependencies{
		Storage:      storage,
		QueueManager: queueManager,
		WSManager:    wsManager,
		MatchHandler: matchHandler,
	}

	// Initialize router
	r := api.NewRouter(deps)

	// Server setup
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on port %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}