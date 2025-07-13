package api

import (
	"net/http"
	"time"

	"langapp-backend/internal/api/handlers"
	"langapp-backend/internal/queue"
	"langapp-backend/internal/sessions"
	"langapp-backend/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Dependencies struct {
	Storage      *storage.Storage
	QueueManager *queue.Manager
	WSManager    *sessions.WSManager
	MatchHandler *handlers.MatchHandler
}

func NewRouter(deps *Dependencies) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(middleware.Compress(5))

	// CORS middleware for WebSocket connections
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"langapp-backend"}`))
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Match endpoints
		r.Post("/match/request", deps.MatchHandler.RequestMatch)
		r.Delete("/match/cancel/{userID}", deps.MatchHandler.CancelMatch)
		r.Get("/queue/status/{userID}", deps.MatchHandler.GetQueueStatus)
	})

	// WebSocket endpoints
	r.Get("/ws/match/{userID}", deps.WSManager.HandleMatchWebSocket)

	return r
}