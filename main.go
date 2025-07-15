package main

import (
	"log"
	"net/http"

	"langapp-backend/api"
)

func main() {
	r := api.NewRouter()

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}