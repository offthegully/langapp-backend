package session

import (
	"context"
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

func (r *Repository) CreateSession(ctx context.Context, user1ID, user2ID string, user1Native, user1Practice, user2Native, user2Practice string) (*Session, error) {
	return nil, nil
}

func (r *Repository) GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	return nil, nil
}

func (r *Repository) GetActiveSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	return nil, nil
}

func (r *Repository) UpdateSessionStatus(ctx context.Context, sessionID uuid.UUID, status SessionStatus) error {
	return nil
}

func (r *Repository) GetUserSessions(ctx context.Context, userID string, limit int) ([]*Session, error) {
	return nil, nil
}
