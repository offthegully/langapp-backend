package signaling

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"langapp-backend/websocket"

	"github.com/google/uuid"
)

// SignalingData represents the data payload for signaling messages
type SignalingData struct {
	SDP       string                 `json:"sdp,omitempty"`       // Session Description Protocol
	Type      string                 `json:"type,omitempty"`      // "offer" or "answer"
	Candidate map[string]interface{} `json:"candidate,omitempty"` // ICE candidate
	MatchID   string                 `json:"match_id,omitempty"`  // Match identifier
	UserID    string                 `json:"user_id,omitempty"`   // User identifier
}

// Match represents an active match between two users
type Match struct {
	ID        string
	UserA     string
	UserB     string
	CreatedAt time.Time
	Status    MatchStatus
	mutex     sync.RWMutex
}

type MatchStatus string

const (
	MatchStatusWaiting    MatchStatus = "waiting"    // Match created, waiting for connection
	MatchStatusConnecting MatchStatus = "connecting" // WebRTC signaling in progress
	MatchStatusActive     MatchStatus = "active"     // Call is active
	MatchStatusFailed     MatchStatus = "failed"     // Connection failed
	MatchStatusCompleted  MatchStatus = "completed"  // Call ended successfully
)

// SignalingService handles WebRTC signaling between matched users
type SignalingService struct {
	wsManager   *websocket.Manager
	matches     map[string]*Match
	userMatches map[string]string // userID -> matchID mapping
	matchmaking chan MatchRequest
	mutex       sync.RWMutex
	stopChan    chan struct{}
}

type MatchRequest struct {
	UserID string
}

// NewSignalingService creates a new signaling service
func NewSignalingService(wsManager *websocket.Manager) *SignalingService {
	return &SignalingService{
		wsManager:   wsManager,
		matches:     make(map[string]*Match),
		userMatches: make(map[string]string),
		matchmaking: make(chan MatchRequest, 100),
		stopChan:    make(chan struct{}),
	}
}

// Start begins the signaling service
func (s *SignalingService) Start() {
	go s.matchmakingLoop()
	go s.cleanupLoop()
}

// Stop gracefully shuts down the signaling service
func (s *SignalingService) Stop() {
	close(s.stopChan)
}

// RequestMatch adds a user to the matchmaking queue
func (s *SignalingService) RequestMatch(userID string) {
	s.matchmaking <- MatchRequest{UserID: userID}
}

// HandleSignalingMessage processes incoming signaling messages from clients
func (s *SignalingService) HandleSignalingMessage(userID string, msgType websocket.MessageType, data json.RawMessage) error {
	var sigData SignalingData
	if err := json.Unmarshal(data, &sigData); err != nil {
		return err
	}

	s.mutex.RLock()
	matchID, exists := s.userMatches[userID]
	s.mutex.RUnlock()

	if !exists {
		log.Printf("No active match for user %s", userID)
		return nil
	}

	match := s.getMatch(matchID)
	if match == nil {
		log.Printf("Match %s not found", matchID)
		return nil
	}

	// Determine the other user in the match
	var otherUserID string
	if match.UserA == userID {
		otherUserID = match.UserB
	} else {
		otherUserID = match.UserA
	}

	switch msgType {
	case websocket.SignalingOffer:
		return s.handleOffer(match, userID, otherUserID, sigData)
	case websocket.SignalingAnswer:
		return s.handleAnswer(match, userID, otherUserID, sigData)
	case websocket.SignalingICE:
		return s.handleICECandidate(match, userID, otherUserID, sigData)
	case websocket.InitiateConnection:
		return s.handleInitiateConnection(match, userID, otherUserID)
	case websocket.ConnectionSuccess:
		return s.handleConnectionSuccess(match, userID)
	case websocket.ConnectionFailure:
		return s.handleConnectionFailure(match, userID)
	}

	return nil
}

// matchmakingLoop handles the matchmaking process
func (s *SignalingService) matchmakingLoop() {
	var waitingUser *MatchRequest

	for {
		select {
		case <-s.stopChan:
			return
		case req := <-s.matchmaking:
			if waitingUser == nil {
				// First user, wait for a match
				waitingUser = &req
				log.Printf("User %s is waiting for a match", req.UserID)

				// Send "still searching" message
				s.wsManager.SendMessage(req.UserID, websocket.Message{
					Type: websocket.StillSearching,
					Data: SignalingData{},
				})
			} else {
				// Second user, create a match
				match := s.createMatch(waitingUser.UserID, req.UserID)
				log.Printf("Match created: %s between users %s and %s", match.ID, waitingUser.UserID, req.UserID)

				// Notify both users that a match was found
				matchData := SignalingData{
					MatchID: match.ID,
				}

				s.wsManager.SendMessage(waitingUser.UserID, websocket.Message{
					Type: websocket.MatchFound,
					Data: matchData,
				})

				s.wsManager.SendMessage(req.UserID, websocket.Message{
					Type: websocket.MatchFound,
					Data: matchData,
				})

				waitingUser = nil
			}
		}
	}
}

// createMatch creates a new match between two users
func (s *SignalingService) createMatch(userA, userB string) *Match {
	match := &Match{
		ID:        uuid.New().String(),
		UserA:     userA,
		UserB:     userB,
		CreatedAt: time.Now(),
		Status:    MatchStatusWaiting,
	}

	s.mutex.Lock()
	s.matches[match.ID] = match
	s.userMatches[userA] = match.ID
	s.userMatches[userB] = match.ID
	s.mutex.Unlock()

	return match
}

// getMatch retrieves a match by ID
func (s *SignalingService) getMatch(matchID string) *Match {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.matches[matchID]
}

// handleOffer processes WebRTC offer messages
func (s *SignalingService) handleOffer(match *Match, fromUser, toUser string, data SignalingData) error {
	match.mutex.Lock()
	match.Status = MatchStatusConnecting
	match.mutex.Unlock()

	log.Printf("Forwarding offer from %s to %s in match %s", fromUser, toUser, match.ID)

	return s.wsManager.SendMessage(toUser, websocket.Message{
		Type: websocket.SignalingMessage,
		Data: SignalingData{
			SDP:     data.SDP,
			Type:    "offer",
			MatchID: match.ID,
			UserID:  fromUser,
		},
	})
}

// handleAnswer processes WebRTC answer messages
func (s *SignalingService) handleAnswer(match *Match, fromUser, toUser string, data SignalingData) error {
	log.Printf("Forwarding answer from %s to %s in match %s", fromUser, toUser, match.ID)

	return s.wsManager.SendMessage(toUser, websocket.Message{
		Type: websocket.SignalingMessage,
		Data: SignalingData{
			SDP:     data.SDP,
			Type:    "answer",
			MatchID: match.ID,
			UserID:  fromUser,
		},
	})
}

// handleICECandidate processes ICE candidate messages
func (s *SignalingService) handleICECandidate(match *Match, fromUser, toUser string, data SignalingData) error {
	log.Printf("Forwarding ICE candidate from %s to %s in match %s", fromUser, toUser, match.ID)

	return s.wsManager.SendMessage(toUser, websocket.Message{
		Type: websocket.SignalingMessage,
		Data: SignalingData{
			Candidate: data.Candidate,
			MatchID:   match.ID,
			UserID:    fromUser,
		},
	})
}

// handleInitiateConnection processes connection initiation requests
func (s *SignalingService) handleInitiateConnection(match *Match, fromUser, toUser string) error {
	log.Printf("User %s initiating connection in match %s", fromUser, match.ID)

	return s.wsManager.SendMessage(toUser, websocket.Message{
		Type: websocket.ConnectionInitiated,
		Data: SignalingData{
			MatchID: match.ID,
			UserID:  fromUser,
		},
	})
}

// handleConnectionSuccess processes successful connection notifications
func (s *SignalingService) handleConnectionSuccess(match *Match, userID string) error {
	match.mutex.Lock()
	match.Status = MatchStatusActive
	match.mutex.Unlock()

	log.Printf("Connection success reported by user %s in match %s", userID, match.ID)

	// Notify both users that the call is now active
	var otherUserID string
	if match.UserA == userID {
		otherUserID = match.UserB
	} else {
		otherUserID = match.UserA
	}

	callActiveData := SignalingData{
		MatchID: match.ID,
	}

	s.wsManager.SendMessage(userID, websocket.Message{
		Type: websocket.CallActive,
		Data: callActiveData,
	})

	return s.wsManager.SendMessage(otherUserID, websocket.Message{
		Type: websocket.CallActive,
		Data: callActiveData,
	})
}

// handleConnectionFailure processes connection failure notifications
func (s *SignalingService) handleConnectionFailure(match *Match, userID string) error {
	match.mutex.Lock()
	match.Status = MatchStatusFailed
	match.mutex.Unlock()

	log.Printf("Connection failure reported by user %s in match %s", userID, match.ID)

	// Notify both users that the connection failed
	var otherUserID string
	if match.UserA == userID {
		otherUserID = match.UserB
	} else {
		otherUserID = match.UserA
	}

	failureData := SignalingData{
		MatchID: match.ID,
	}

	s.wsManager.SendMessage(userID, websocket.Message{
		Type: websocket.ConnectionFailed,
		Data: failureData,
	})

	s.wsManager.SendMessage(otherUserID, websocket.Message{
		Type: websocket.ConnectionFailed,
		Data: failureData,
	})

	// Clean up the match
	s.cleanupMatch(match.ID)
	return nil
}

// cleanupMatch removes a match and its associated user mappings
func (s *SignalingService) cleanupMatch(matchID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	match, exists := s.matches[matchID]
	if !exists {
		return
	}

	delete(s.userMatches, match.UserA)
	delete(s.userMatches, match.UserB)
	delete(s.matches, matchID)

	log.Printf("Match %s cleaned up", matchID)
}

// cleanupLoop periodically cleans up old matches
func (s *SignalingService) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanupOldMatches()
		}
	}
}

// cleanupOldMatches removes matches that are older than 30 minutes
func (s *SignalingService) cleanupOldMatches() {
	cutoff := time.Now().Add(-30 * time.Minute)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	var toDelete []string
	for matchID, match := range s.matches {
		if match.CreatedAt.Before(cutoff) && match.Status != MatchStatusActive {
			toDelete = append(toDelete, matchID)
		}
	}

	for _, matchID := range toDelete {
		match := s.matches[matchID]
		delete(s.userMatches, match.UserA)
		delete(s.userMatches, match.UserB)
		delete(s.matches, matchID)
		log.Printf("Cleaned up old match %s", matchID)
	}
}

// GetMatchStatus returns the current status of a user's match
func (s *SignalingService) GetMatchStatus(userID string) (MatchStatus, string) {
	s.mutex.RLock()
	matchID, exists := s.userMatches[userID]
	s.mutex.RUnlock()

	if !exists {
		return "", ""
	}

	match := s.getMatch(matchID)
	if match == nil {
		return "", ""
	}

	match.mutex.RLock()
	status := match.Status
	match.mutex.RUnlock()

	return status, matchID
}

// EndMatch manually ends a match (e.g., when a user disconnects)
func (s *SignalingService) EndMatch(userID string) {
	s.mutex.RLock()
	matchID, exists := s.userMatches[userID]
	s.mutex.RUnlock()

	if !exists {
		return
	}

	match := s.getMatch(matchID)
	if match == nil {
		return
	}

	match.mutex.Lock()
	match.Status = MatchStatusCompleted
	match.mutex.Unlock()

	log.Printf("Match %s ended by user %s", matchID, userID)
	s.cleanupMatch(matchID)
}
