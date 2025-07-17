package main

import (
	"context"
	"log"
	"net/http"

	"langapp-backend/api"
	"langapp-backend/languages"
	"langapp-backend/matchmaking"
	"langapp-backend/storage"
	"langapp-backend/websocket"
)

func main() {
	ctx := context.Background()

	redisClient := storage.NewRedisClient()
	pubSubManager := storage.NewPubSubManager(redisClient)

	languagesService := languages.NewService()
	supportedLanguages := languagesService.GetSupportedLanguages()

	languageNames := make([]string, len(supportedLanguages))
	for i, lang := range supportedLanguages {
		languageNames[i] = lang.Name
	}

	if err := pubSubManager.InitializeLanguagePublishers(languageNames); err != nil {
		log.Fatalf("Failed to initialize language publishers: %v", err)
	}

	matchmakingService := matchmaking.NewMatchmakingService(redisClient, pubSubManager)

	if err := matchmakingService.InitializeLanguageChannels(ctx, languageNames); err != nil {
		log.Fatalf("Failed to initialize language channels: %v", err)
	}

	wsManager := websocket.NewManager()
	go wsManager.Start()

	matchingService := matchmaking.NewMatchingService(redisClient, pubSubManager, wsManager, languageNames)
	go matchingService.Start(ctx)

	apiService := api.NewAPIService(matchmakingService, languagesService, wsManager)
	r := api.NewRouter(apiService)

	log.Printf("Server starting on :8080 with %d language channels initialized", len(languageNames))
	log.Fatal(http.ListenAndServe(":8080", r))
}
