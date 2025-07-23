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
	InitiateMatchmaking(ctx context.Context, userID, nativeLanguage, practiceLanguage string) (*matchmaking.QueueEntry, error)
	CancelMatchmaking(ctx context.Context, userID string) error
}

type LanguagesRepository interface {
	GetAllLanguages(ctx context.Context) ([]languages.Language, error)
	GetLanguageByName(ctx context.Context, name string) (*languages.Language, error)
}

type APIService struct {
	matchmakingService  MatchmakingService
	languagesRepository LanguagesRepository
	wsManager           *websocket.Manager
}

func NewAPIService(matchmakingService MatchmakingService, languagesRepository LanguagesRepository, wsManager *websocket.Manager) *APIService {
	return &APIService{
		matchmakingService:  matchmakingService,
		languagesRepository: languagesRepository,
		wsManager:           wsManager,
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
