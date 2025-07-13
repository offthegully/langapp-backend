package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"langapp-backend/internal/queue"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type MatchHandler struct {
	queueManager *queue.Manager
}

func NewMatchHandler(queueManager *queue.Manager) *MatchHandler {
	return &MatchHandler{
		queueManager: queueManager,
	}
}

type MatchRequestBody struct {
	UserID           string   `json:"user_id"`
	NativeLanguages  []string `json:"native_languages"`
	PracticeLanguage string   `json:"practice_language"`
}

type MatchResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	Message   string    `json:"message"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (h *MatchHandler) RequestMatch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := h.generateRequestID()
	clientIP := h.getClientIP(r)
	
	log.Printf("[MATCH_REQUEST] %s - Starting match request from IP: %s, User-Agent: %s", 
		requestID, clientIP, r.Header.Get("User-Agent"))

	var reqBody MatchRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Printf("[MATCH_REQUEST] %s - Failed to decode request body: %v", requestID, err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	log.Printf("[MATCH_REQUEST] %s - Parsed request: UserID=%s, NativeLanguages=%v, PracticeLanguage=%s", 
		requestID, reqBody.UserID, reqBody.NativeLanguages, reqBody.PracticeLanguage)

	// Validate request
	log.Printf("[MATCH_REQUEST] %s - Validating request parameters", requestID)
	if err := h.validateMatchRequest(reqBody); err != nil {
		log.Printf("[MATCH_REQUEST] %s - Validation failed: %v", requestID, err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}
	log.Printf("[MATCH_REQUEST] %s - Validation passed", requestID)

	// Parse user ID
	log.Printf("[MATCH_REQUEST] %s - Parsing user ID: %s", requestID, reqBody.UserID)
	userID, err := uuid.Parse(reqBody.UserID)
	if err != nil {
		log.Printf("[MATCH_REQUEST] %s - Invalid UUID format: %s, error: %v", requestID, reqBody.UserID, err)
		h.writeError(w, http.StatusBadRequest, "invalid user_id", "user_id must be a valid UUID")
		return
	}
	log.Printf("[MATCH_REQUEST] %s - Successfully parsed user ID: %s", requestID, userID)

	// Create queue request
	queueReq := queue.QueueRequest{
		UserID:           userID,
		NativeLanguages:  reqBody.NativeLanguages,
		PracticeLanguage: reqBody.PracticeLanguage,
	}

	// Add to queue
	log.Printf("[MATCH_REQUEST] %s - Adding user %s to queue for practice language: %s", 
		requestID, userID, queueReq.PracticeLanguage)
	queueStart := time.Now()
	response, err := h.queueManager.AddToQueue(r.Context(), queueReq)
	queueDuration := time.Since(queueStart)
	if err != nil {
		log.Printf("[MATCH_REQUEST] %s - Failed to add to queue after %v: %v", 
			requestID, queueDuration, err)
		h.writeError(w, http.StatusInternalServerError, "failed to add to queue", err.Error())
		return
	}
	log.Printf("[MATCH_REQUEST] %s - Successfully added to queue in %v, Request ID: %s, Expires: %s", 
		requestID, queueDuration, response.RequestID, response.ExpiresAt.Format(time.RFC3339))

	matchResp := MatchResponse{
		RequestID: response.RequestID,
		Status:    response.Status,
		ExpiresAt: response.ExpiresAt,
		Message:   "Added to matchmaking queue. You will be notified when a match is found.",
	}

	totalDuration := time.Since(start)
	log.Printf("[MATCH_REQUEST] %s - Request completed successfully in %v, returning response", 
		requestID, totalDuration)
	h.writeJSON(w, http.StatusOK, matchResp)
	
	// Log final success metrics
	log.Printf("[MATCH_REQUEST_METRICS] RequestID=%s UserID=%s PracticeLanguage=%s Duration=%v QueueDuration=%v ClientIP=%s", 
		requestID, userID, reqBody.PracticeLanguage, totalDuration, queueDuration, clientIP)
}

func (h *MatchHandler) CancelMatch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := h.generateRequestID()
	clientIP := h.getClientIP(r)
	
	userID := chi.URLParam(r, "userID")
	practiceLanguage := r.URL.Query().Get("practice_language")
	
	log.Printf("[MATCH_CANCEL] %s - Cancel request from IP: %s, UserID: %s, PracticeLanguage: %s", 
		requestID, clientIP, userID, practiceLanguage)

	if userID == "" {
		log.Printf("[MATCH_CANCEL] %s - Missing user_id parameter", requestID)
		h.writeError(w, http.StatusBadRequest, "missing user_id", "user_id is required")
		return
	}

	if practiceLanguage == "" {
		log.Printf("[MATCH_CANCEL] %s - Missing practice_language parameter", requestID)
		h.writeError(w, http.StatusBadRequest, "missing practice_language", "practice_language query parameter is required")
		return
	}

	log.Printf("[MATCH_CANCEL] %s - Validation passed, proceeding with cancellation", requestID)

	removalStart := time.Now()
	if err := h.queueManager.RemoveFromQueue(r.Context(), userID, practiceLanguage); err != nil {
		removalDuration := time.Since(removalStart)
		log.Printf("[MATCH_CANCEL] %s - Failed to remove from queue after %v: %v", 
			requestID, removalDuration, err)
		h.writeError(w, http.StatusInternalServerError, "failed to cancel match", err.Error())
		return
	}
	removalDuration := time.Since(removalStart)
	log.Printf("[MATCH_CANCEL] %s - Successfully removed from queue in %v", requestID, removalDuration)

	response := map[string]string{
		"status":  "cancelled",
		"message": "Match request cancelled successfully",
	}

	totalDuration := time.Since(start)
	log.Printf("[MATCH_CANCEL] %s - Cancellation completed successfully in %v", requestID, totalDuration)
	h.writeJSON(w, http.StatusOK, response)
	
	// Log final metrics
	log.Printf("[MATCH_CANCEL_METRICS] RequestID=%s UserID=%s PracticeLanguage=%s Duration=%v RemovalDuration=%v ClientIP=%s", 
		requestID, userID, practiceLanguage, totalDuration, removalDuration, clientIP)
}

func (h *MatchHandler) GetQueueStatus(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := h.generateRequestID()
	clientIP := h.getClientIP(r)
	
	userID := chi.URLParam(r, "userID")
	
	log.Printf("[QUEUE_STATUS] %s - Status request from IP: %s, UserID: %s", 
		requestID, clientIP, userID)

	if userID == "" {
		log.Printf("[QUEUE_STATUS] %s - Missing user_id parameter", requestID)
		h.writeError(w, http.StatusBadRequest, "missing user_id", "user_id is required")
		return
	}

	log.Printf("[QUEUE_STATUS] %s - Fetching queue status for user: %s", requestID, userID)

	statusStart := time.Now()
	status, err := h.queueManager.GetQueueStatus(r.Context(), userID)
	statusDuration := time.Since(statusStart)
	if err != nil {
		log.Printf("[QUEUE_STATUS] %s - Failed to get queue status after %v: %v", 
			requestID, statusDuration, err)
		h.writeError(w, http.StatusInternalServerError, "failed to get queue status", err.Error())
		return
	}
	
	// Log detailed queue information
	totalUsers := 0
	for _, count := range status {
		totalUsers += count
	}
	log.Printf("[QUEUE_STATUS] %s - Retrieved queue status in %v: %d languages, %d total users", 
		requestID, statusDuration, len(status), totalUsers)
	log.Printf("[QUEUE_STATUS] %s - Queue details: %+v", requestID, status)

	response := map[string]interface{}{
		"queue_status": status,
		"timestamp":    time.Now().UTC(),
	}

	totalDuration := time.Since(start)
	log.Printf("[QUEUE_STATUS] %s - Status request completed successfully in %v", requestID, totalDuration)
	h.writeJSON(w, http.StatusOK, response)
	
	// Log final metrics
	log.Printf("[QUEUE_STATUS_METRICS] RequestID=%s UserID=%s Duration=%v StatusDuration=%v TotalUsers=%d Languages=%d ClientIP=%s", 
		requestID, userID, totalDuration, statusDuration, totalUsers, len(status), clientIP)
}

func (h *MatchHandler) validateMatchRequest(req MatchRequestBody) error {
	if req.UserID == "" {
		return &ValidationError{Field: "user_id", Message: "user_id is required"}
	}

	if len(req.NativeLanguages) == 0 {
		return &ValidationError{Field: "native_languages", Message: "at least one native language is required"}
	}

	if req.PracticeLanguage == "" {
		return &ValidationError{Field: "practice_language", Message: "practice_language is required"}
	}

	// Check that practice language is not in native languages
	for _, native := range req.NativeLanguages {
		if native == req.PracticeLanguage {
			return &ValidationError{Field: "practice_language", Message: "practice_language cannot be the same as native language"}
		}
	}

	return nil
}

func (h *MatchHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *MatchHandler) writeError(w http.ResponseWriter, status int, error, message string) {
	log.Printf("[ERROR] HTTP %d - %s: %s", status, error, message)
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, status, resp)
}

// Helper functions for logging and debugging
func (h *MatchHandler) generateRequestID() string {
	return fmt.Sprintf("req_%d_%s", time.Now().UnixNano(), uuid.New().String()[:8])
}

func (h *MatchHandler) getClientIP(r *http.Request) string {
	// Check for forwarded headers first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}