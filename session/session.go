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

func (r *Repository) CreateSession(ctx context.Context, practiceUserID, nativeUserID, language string) (*Session, error) {
	session := &Session{
		ID:             uuid.New(),
		PracticeUserID: practiceUserID,
		NativeUserID:   nativeUserID,
		Language:       language,
		Status:         SessionMatched,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	query := `
		INSERT INTO sessions (id, practice_user_id, native_user_id, language, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.Exec(ctx, query,
		session.ID,
		session.PracticeUserID,
		session.NativeUserID,
		session.Language,
		session.Status,
		session.CreatedAt,
		session.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return session, nil
}

func (r *Repository) GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	query := `
		SELECT id, practice_user_id, native_user_id, language, status, 
		       created_at, updated_at, ended_at, duration_seconds
		FROM sessions 
		WHERE id = $1
	`

	var session Session
	err := r.db.QueryRow(ctx, query, sessionID).Scan(
		&session.ID,
		&session.PracticeUserID,
		&session.NativeUserID,
		&session.Language,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.EndedAt,
		&session.DurationSeconds,
	)

	if err != nil {
		return nil, err
	}

	return &session, nil
}

func (r *Repository) GetActiveSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	query := `
		SELECT id, practice_user_id, native_user_id, language, status, 
		       created_at, updated_at, ended_at, duration_seconds
		FROM sessions 
		WHERE (practice_user_id = $1 OR native_user_id = $1)
		  AND status IN ('matched', 'connecting', 'active')
		ORDER BY created_at DESC
		LIMIT 1
	`

	var session Session
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&session.ID,
		&session.PracticeUserID,
		&session.NativeUserID,
		&session.Language,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.EndedAt,
		&session.DurationSeconds,
	)

	if err != nil {
		return nil, err
	}

	return &session, nil
}

func (r *Repository) GetSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	query := `
		SELECT id, practice_user_id, native_user_id, language, status, 
		       created_at, updated_at, ended_at, duration_seconds
		FROM sessions 
		WHERE (practice_user_id = $1 OR native_user_id = $1)
		ORDER BY created_at DESC
		LIMIT 1
	`

	var session Session
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&session.ID,
		&session.PracticeUserID,
		&session.NativeUserID,
		&session.Language,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.EndedAt,
		&session.DurationSeconds,
	)

	if err != nil {
		return nil, err
	}

	return &session, nil
}

func (r *Repository) UpdateSession(ctx context.Context, sessionID uuid.UUID, status SessionStatus) error {
	var query string
	var args []interface{}

	if status == SessionCompleted || status == SessionFailed {
		// Calculate duration if ending the session
		endedAt := time.Now()
		query = `
			UPDATE sessions 
			SET status = $1, ended_at = $2, duration_seconds = EXTRACT(EPOCH FROM ($2 - created_at))::INTEGER
			WHERE id = $3
		`
		args = []interface{}{status, endedAt, sessionID}
	} else {
		// Just update status for other transitions
		query = `UPDATE sessions SET status = $1 WHERE id = $2`
		args = []interface{}{status, sessionID}
	}

	_, err := r.db.Exec(ctx, query, args...)
	return err
}
