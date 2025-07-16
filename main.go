package main

import (
	"log"
	"net/http"

	"langapp-backend/api"
	"langapp-backend/languages"
	"langapp-backend/matchmaking"
	"langapp-backend/storage"
)

func main() {
	redisClient := storage.NewRedisClient()
	matchmakingService := matchmaking.NewMatchmakingService(redisClient)
	languagesService := languages.NewService()
	apiService := api.NewAPIService(matchmakingService, languagesService)
	
	r := api.NewRouter(apiService)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
