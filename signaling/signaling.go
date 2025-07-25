package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"langapp-backend/session"
	"langapp-backend/websocket"

	"github.com/google/uuid"
)

type MessageType string

const (
	Offer         MessageType = "offer"
	Answer        MessageType = "answer"
	ICECandidate  MessageType = "ice-candidate"
	ConnectionError MessageType = "connection-error"
)

type SignalingMessage struct {
	Type      MessageType     `json:"type"`
	SessionID uuid.UUID       `json:"session_id"`
	FromUser  string          `json:"from_user"`
	ToUser    string          `json:"to_user"`
	Data      json.RawMessage `json:"data"`
}

type OfferData struct {
	SDP string `json:"sdp"`
}

type AnswerData struct {
	SDP string `json:"sdp"`
}

type ICECandidateData struct {
	Candidate     string `json:"candidate"`
	SDPMLineIndex int    `json:"sdpMLineIndex"`
	SDPMid        string `json:"sdpMid"`
}

type ConnectionErrorData struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type SessionRepository interface {
	GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*session.Session, error)
	GetSessionByUserID(ctx context.Context, userID string) (*session.Session, error)
	UpdateSession(ctx context.Context, sessionID uuid.UUID, status session.SessionStatus) error
}

type Service struct {
	wsManager         *websocket.Manager
	sessionRepository SessionRepository
}

func NewService(wsManager *websocket.Manager, sessionRepository SessionRepository) *Service {
	return &Service{
		wsManager:         wsManager,
		sessionRepository: sessionRepository,
	}
}

// ProcessSignalingMessage handles incoming WebRTC signaling messages
func (s *Service) ProcessSignalingMessage(ctx context.Context, message SignalingMessage) error {
	// Validate the session exists and user is authorized
	sess, err := s.validateSessionAndUser(ctx, message.SessionID, message.FromUser)
	if err != nil {
		return fmt.Errorf("session validation failed: %w", err)
	}

	// Update session status based on message type
	if err := s.updateSessionStatus(ctx, message, sess); err != nil {
		log.Printf("Warning: failed to update session status: %v", err)
	}

	// Forward the message to the target user
	if err := s.forwardMessage(ctx, message); err != nil {
		return fmt.Errorf("failed to forward signaling message: %w", err)
	}

	log.Printf("Signaling message %s forwarded from %s to %s for session %s", 
		message.Type, message.FromUser, message.ToUser, message.SessionID)

	return nil
}

// InitiateConnection starts the WebRTC negotiation process
func (s *Service) InitiateConnection(ctx context.Context, sessionID uuid.UUID, userID string) error {
	sess, err := s.sessionRepository.GetSessionByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Verify user is part of this session
	if sess.PracticeUserID != userID && sess.NativeUserID != userID {
		return fmt.Errorf("user %s is not part of session %s", userID, sessionID)
	}

	// Update session to connecting status
	if err := s.sessionRepository.UpdateSession(ctx, sessionID, session.SessionConnecting); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Notify both users that connection initiation has started
	initiationMessage := websocket.Message{
		Type: websocket.ConnectionInitiated,
		Data: map[string]interface{}{
			"session_id": sessionID,
			"initiator":  userID,
			"message":    "WebRTC connection initiated",
		},
	}

	// Send to both users
	if err := s.wsManager.SendMessage(sess.PracticeUserID, initiationMessage); err != nil {
		log.Printf("Failed to notify practice user %s of connection initiation: %v", sess.PracticeUserID, err)
	}
	if err := s.wsManager.SendMessage(sess.NativeUserID, initiationMessage); err != nil {
		log.Printf("Failed to notify native user %s of connection initiation: %v", sess.NativeUserID, err)
	}

	log.Printf("Connection initiated for session %s by user %s", sessionID, userID)
	return nil
}

// HandleConnectionSuccess marks a session as active
func (s *Service) HandleConnectionSuccess(ctx context.Context, sessionID uuid.UUID, userID string) error {
	sess, err := s.validateSessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return fmt.Errorf("session validation failed: %w", err)
	}

	// Update session to active status
	if err := s.sessionRepository.UpdateSession(ctx, sessionID, session.SessionActive); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Notify both users that the call is now active
	activeMessage := websocket.Message{
		Type: websocket.CallActive,
		Data: map[string]interface{}{
			"session_id": sessionID,
			"message":    "Audio call is now active",
			"language":   sess.Language,
		},
	}

	if err := s.wsManager.SendMessage(sess.PracticeUserID, activeMessage); err != nil {
		log.Printf("Failed to notify practice user %s of active call: %v", sess.PracticeUserID, err)
	}
	if err := s.wsManager.SendMessage(sess.NativeUserID, activeMessage); err != nil {
		log.Printf("Failed to notify native user %s of active call: %v", sess.NativeUserID, err)
	}

	log.Printf("Session %s is now active", sessionID)
	return nil
}

// HandleConnectionFailure marks a session as failed
func (s *Service) HandleConnectionFailure(ctx context.Context, sessionID uuid.UUID, userID string, errorMsg string) error {
	sess, err := s.validateSessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return fmt.Errorf("session validation failed: %w", err)
	}

	// Update session to failed status
	if err := s.sessionRepository.UpdateSession(ctx, sessionID, session.SessionFailed); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Notify both users of the connection failure
	failureMessage := websocket.Message{
		Type: websocket.ConnectionFailed,
		Data: map[string]interface{}{
			"session_id": sessionID,
			"error":      errorMsg,
			"message":    "Connection failed - you may try to reconnect",
		},
	}

	if err := s.wsManager.SendMessage(sess.PracticeUserID, failureMessage); err != nil {
		log.Printf("Failed to notify practice user %s of connection failure: %v", sess.PracticeUserID, err)
	}
	if err := s.wsManager.SendMessage(sess.NativeUserID, failureMessage); err != nil {
		log.Printf("Failed to notify native user %s of connection failure: %v", sess.NativeUserID, err)
	}

	log.Printf("Session %s marked as failed: %s", sessionID, errorMsg)
	return nil
}

// validateSessionAndUser checks if session exists and user is authorized
func (s *Service) validateSessionAndUser(ctx context.Context, sessionID uuid.UUID, userID string) (*session.Session, error) {
	sess, err := s.sessionRepository.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if sess.PracticeUserID != userID && sess.NativeUserID != userID {
		return nil, fmt.Errorf("user %s is not part of session %s", userID, sessionID)
	}

	return sess, nil
}

// updateSessionStatus updates session status based on signaling message type
func (s *Service) updateSessionStatus(ctx context.Context, message SignalingMessage, sess *session.Session) error {
	var newStatus session.SessionStatus

	switch message.Type {
	case Offer:
		// First offer means we're starting to connect
		newStatus = session.SessionConnecting
	case Answer:
		// Answer received, still connecting
		newStatus = session.SessionConnecting
	case ICECandidate:
		// ICE candidates being exchanged, keep as connecting
		return nil // No status change needed
	case ConnectionError:
		// Connection error, mark as failed
		newStatus = session.SessionFailed
	default:
		return nil // Unknown message type, no status change
	}

	// Only update if status actually changed
	if sess.Status != newStatus {
		return s.sessionRepository.UpdateSession(ctx, message.SessionID, newStatus)
	}

	return nil
}

// forwardMessage sends the signaling message to the target user
func (s *Service) forwardMessage(ctx context.Context, message SignalingMessage) error {
	wsMessage := websocket.Message{
		Type: websocket.SignalingMessage,
		Data: message,
	}

	return s.wsManager.SendMessage(message.ToUser, wsMessage)
}