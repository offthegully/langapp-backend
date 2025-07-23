package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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
	Message      string    `json:"message"`
	QueuedAt     time.Time `json:"queued_at"`
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

	ok, msg := api.validateStartMatchmakingRequest(r.Context(), req)
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	userID := req.UserID
	nativeLanguage := req.NativeLanguage
	practiceLanguage := req.PracticeLanguage

	entry, err := api.matchmakingService.InitiateMatchmaking(r.Context(), userID, nativeLanguage, practiceLanguage)
	if err != nil {
		http.Error(w, "Failed to join queue", http.StatusInternalServerError)
		return
	}

	response := StartMatchmakingResponse{
		Message:      "Successfully joined matchmaking queue. Connect to the WebSocket URL to receive match notifications.",
		QueuedAt:     entry.Timestamp,
		WebSocketURL: api.getWebSocketURL(req.UserID, r),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (api *APIService) validateStartMatchmakingRequest(ctx context.Context, req StartMatchmakingRequest) (bool, string) {
	if req.UserID == "" || req.NativeLanguage == "" || req.PracticeLanguage == "" {
		return false, "Missing required fields: user_id, native_language, practice_language"
	}

	if strings.EqualFold(req.NativeLanguage, req.PracticeLanguage) {
		return false, "Native language and practice language cannot be the same"
	}

	nativeLanguage, err := api.languagesRepository.GetLanguageByName(ctx, req.NativeLanguage)
	if err != nil {
		return false, "Error validating native language"
	}
	if nativeLanguage == nil {
		return false, "Invalid native language"
	}

	practiceLanguage, err := api.languagesRepository.GetLanguageByName(ctx, req.PracticeLanguage)
	if err != nil {
		return false, "Error validating practice language"
	}
	if practiceLanguage == nil {
		return false, "Invalid practice language"
	}

	return true, ""
}

func (api *APIService) CancelMatchmaking(w http.ResponseWriter, r *http.Request) {
	var req CancelMatchmakingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ok, msg := api.validateCancelMatchmakingRequest(r.Context(), req)
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	err := api.matchmakingService.CancelMatchmaking(r.Context(), req.UserID)
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

func (api *APIService) validateCancelMatchmakingRequest(ctx context.Context, req CancelMatchmakingRequest) (bool, string) {
	if req.UserID == "" || req.PracticeLanguage == "" {
		return false, "Missing required fields: user_id, practice_language"
	}

	language, err := api.languagesRepository.GetLanguageByName(ctx, req.PracticeLanguage)
	if err != nil {
		return false, "Error validating practice language"
	}
	if language == nil {
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
