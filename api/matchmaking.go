package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"langapp-backend/matchmaking"
)

type StartMatchmakingRequest struct {
	UserID           string `json:"user_id"`
	NativeLanguage   string `json:"native_language"`
	PracticeLanguage string `json:"practice_language"`
}

type CancelMatchmakingRequest struct {
	UserID           string `json:"user_id"`
	PracticeLanguage string `json:"practice_language"`
}

type StartMatchmakingResponse struct {
	Message     string    `json:"message"`
	QueuedAt    time.Time `json:"queued_at"`
	WebSocketURL string    `json:"websocket_url"`
}

type CancelMatchmakingResponse struct {
	Message string `json:"message"`
}

func (api *APIService) StartMatchmaking(w http.ResponseWriter, r *http.Request) {
	var req StartMatchmakingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ok, msg := api.validateStartMatchmakingRequest(req)
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	queueEntry := matchmaking.QueueEntry{
		UserID:           req.UserID,
		NativeLanguage:   req.NativeLanguage,
		PracticeLanguage: req.PracticeLanguage,
		Timestamp:        time.Now(),
	}

	err := api.matchmakingService.AddToQueue(r.Context(), queueEntry)
	if err != nil {
		http.Error(w, "Failed to join queue", http.StatusInternalServerError)
		return
	}

	response := StartMatchmakingResponse{
		Message:      "Successfully joined matchmaking queue. Connect to the WebSocket URL to receive match notifications.",
		QueuedAt:     queueEntry.Timestamp,
		WebSocketURL: api.getWebSocketURL(req.UserID, r),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (api *APIService) CancelMatchmaking(w http.ResponseWriter, r *http.Request) {
	var req CancelMatchmakingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ok, msg := api.validateCancelMatchmakingRequest(req)
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	err := api.matchmakingService.RemoveFromQueue(r.Context(), req.UserID, req.PracticeLanguage)
	if err != nil {
		http.Error(w, "Failed to remove from queue", http.StatusInternalServerError)
		return
	}

	response := CancelMatchmakingResponse{
		Message: "Successfully removed from matchmaking queue",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (api *APIService) validateCancelMatchmakingRequest(req CancelMatchmakingRequest) (bool, string) {
	if req.UserID == "" || req.PracticeLanguage == "" {
		return false, "Missing required fields: user_id, practice_language"
	}

	valid, err := api.languagesService.IsValidLanguage(req.PracticeLanguage)
	if err != nil {
		return false, "Error validating practice language"
	}
	if !valid {
		return false, "Invalid practice language"
	}

	return true, ""
}

func (api *APIService) validateStartMatchmakingRequest(req StartMatchmakingRequest) (bool, string) {
	if req.UserID == "" || req.NativeLanguage == "" || req.PracticeLanguage == "" {
		return false, "Missing required fields: user_id, native_language, practice_language"
	}

	if strings.EqualFold(req.NativeLanguage, req.PracticeLanguage) {
		return false, "Native language and practice language cannot be the same"
	}

	valid, err := api.languagesService.IsValidLanguage(req.NativeLanguage)
	if err != nil {
		return false, "Error validating native language"
	}
	if !valid {
		return false, "Invalid native language"
	}

	valid, err = api.languagesService.IsValidLanguage(req.PracticeLanguage)
	if err != nil {
		return false, "Error validating practice language"
	}
	if !valid {
		return false, "Invalid practice language"
	}

	return true, ""
}

func (api *APIService) getWebSocketURL(userID string, r *http.Request) string {
	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	
	return fmt.Sprintf("%s://%s/ws?user_id=%s", scheme, host, userID)
}
