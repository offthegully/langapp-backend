package api

import (
	"context"
	"langapp-backend/languages"
	"langapp-backend/matchmaking"
	"langapp-backend/websocket"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type MatchmakingService interface {
	AddToQueue(ctx context.Context, entry matchmaking.QueueEntry) error
}

type LanguagesService interface {
	GetSupportedLanguages() []languages.Language
	IsValidLanguage(language string) bool
}

type APIService struct {
	matchmakingService MatchmakingService
	languagesService   LanguagesService
	wsManager          *websocket.Manager
}

func NewAPIService(matchmakingService MatchmakingService, languagesService LanguagesService, wsManager *websocket.Manager) *APIService {
	return &APIService{
		matchmakingService: matchmakingService,
		languagesService:   languagesService,
		wsManager:          wsManager,
	}
}

func NewRouter(apiService *APIService) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/languages", apiService.GetLanguagesHandler)
	r.Post("/queue", apiService.StartMatchmaking)
	r.Delete("/queue", apiService.CancelMatchmaking)
	r.HandleFunc("/ws", apiService.wsManager.HandleWebSocket)

	return r
}
