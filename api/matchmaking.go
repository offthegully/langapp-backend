package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type QueueRequest struct {
	UserID           string `json:"user_id"`
	NativeLanguage   string `json:"native_language"`
	PracticeLanguage string `json:"practice_language"`
}

type QueueEntry struct {
	UserID           string    `json:"user_id"`
	NativeLanguage   string    `json:"native_language"`
	PracticeLanguage string    `json:"practice_language"`
	Timestamp        time.Time `json:"timestamp"`
}

type QueueResponse struct {
	Message  string    `json:"message"`
	QueuedAt time.Time `json:"queued_at"`
}

func JoinQueueHandler(w http.ResponseWriter, r *http.Request) {
	var req QueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ok, msg := validateQueueRequest(req)
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	response := QueueResponse{
		Message:  "Successfully joined matchmaking queue",
		QueuedAt: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func validateQueueRequest(req QueueRequest) (bool, string) {
	if req.UserID == "" || req.NativeLanguage == "" || req.PracticeLanguage == "" {
		return false, "Missing required fields: user_id, native_language, practice_language"
	}

	if strings.EqualFold(req.NativeLanguage, req.PracticeLanguage) {
		return false, "Native language and practice language cannot be the same"
	}

	return true, ""
}
