package session

import (
	"context"
	"fmt"
	"time"

	"langapp-backend/storage"

	"github.com/google/uuid"
)

type SessionStatus string

const (
	SessionMatched    SessionStatus = "matched"    // Users matched, not yet connected
	SessionConnecting SessionStatus = "connecting" // WebRTC negotiation in progress
	SessionActive     SessionStatus = "active"     // Audio call in progress
	SessionCompleted  SessionStatus = "completed"  // Call ended normally
	SessionFailed     SessionStatus = "failed"     // Connection failed
)

type Session struct {
	ID              uuid.UUID     `json:"id"`
	PracticeUserID  string        `json:"practice_user_id"`
	NativeUserID    string        `json:"native_user_id"`
	Language        string        `json:"language"`
	Status          SessionStatus `json:"status"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	EndedAt         *time.Time    `json:"ended_at,omitempty"`
	DurationSeconds *int32        `json:"duration_seconds,omitempty"`
}

type Repository struct {
	db *storage.PostgresClient
}

func NewRepository(db *storage.PostgresClient) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) CreateSession(ctx context.Context, practiceUserID, nativeUserID, language string) (*Session, error) {
	session := Session{
		PracticeUserID: practiceUserID,
		NativeUserID:   nativeUserID,
		Language:       language,
	}

	err := r.db.QueryRow(
		ctx,
		"INSERT INTO sessions (practice_user_id, native_user_id, language) VALUES ($1, $2, $3) RETURNING id",
		session.PracticeUserID, session.NativeUserID, session.Language,
	).Scan(&session.ID)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %v", err)
	}

	return &session, nil
}

func (r *Repository) GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	var session Session
	err := r.db.QueryRow(
		ctx,
		"SELECT id, practice_user_id, native_user_id, language, status FROM sessions WHERE id = $1",
		sessionID,
	).Scan(&session.ID, &session.PracticeUserID, &session.NativeUserID, &session.Language, &session.Status)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %v", err)
	}

	return &session, nil
}

func (r *Repository) GetSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	var session Session
	err := r.db.QueryRow(
		ctx,
		"SELECT id, practice_user_id, native_user_id, language, status FROM sessions WHERE practice_user_id = $1 OR native_user_id = $1",
		userID,
	).Scan(&session.ID, &session.PracticeUserID, &session.NativeUserID, &session.Language, &session.Status)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %v", err)
	}

	return &session, nil
}

func (r *Repository) UpdateSession(ctx context.Context, sessionID uuid.UUID, status SessionStatus) error {
	_, err := r.db.Exec(
		ctx,
		"UPDATE sessions SET status = $1 WHERE id = $2",
		status, sessionID,
	)
	if err != nil {
		return fmt.Errorf("error querying database: %v", err)
	}

	return nil
}
