package main

import (
	"context"
	"log"
	"net/http"

	"langapp-backend/api"
	"langapp-backend/languages"
	"langapp-backend/matchmaking"
	"langapp-backend/session"
	"langapp-backend/storage/postgres"
	"langapp-backend/storage/redis"
	"langapp-backend/websocket"
)

func main() {
	ctx := context.Background()

	redisClient := redis.NewRedisClient()
	pubSubManager := redis.NewPubSubManager(redisClient)

	postgresClient := postgres.NewPostgresClient(ctx)
	defer postgresClient.Close()

	// Run database migrations
	if err := postgresClient.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	sessionRepository := session.NewRepository(postgresClient)

	languagesRepository := languages.NewRepository(postgresClient)
	languages, err := languagesRepository.GetAllLanguages(ctx)
	if err != nil {
		log.Fatalf("Failed to get supported languages: %v", err)
	}

	languageNames := make([]string, len(languages))
	for i, lang := range languages {
		languageNames[i] = lang.Name
	}

	if err := pubSubManager.InitializeLanguagePublishers(languageNames); err != nil {
		log.Fatalf("Failed to initialize language publishers: %v", err)
	}

	wsManager := websocket.NewManager()
	go wsManager.Start()

	matchmakingService := matchmaking.NewMatchmakingService(redisClient, pubSubManager, wsManager, sessionRepository, languageNames)
	if err := matchmakingService.InitializeLanguageChannels(ctx, languageNames); err != nil {
		log.Fatalf("Failed to initialize language channels: %v", err)
	}
	go matchmakingService.Start(ctx)

	apiService := api.NewAPIService(matchmakingService, languagesRepository, wsManager)
	r := api.NewRouter(apiService)

	log.Printf("Server starting on :8080 with %d language channels initialized", len(languageNames))
	log.Fatal(http.ListenAndServe(":8080", r))
}
