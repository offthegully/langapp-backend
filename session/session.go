package session

import (
	"context"
	"time"

	"langapp-backend/storage"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionCancelled SessionStatus = "cancelled"
)

type Session struct {
	ID                    uuid.UUID     `json:"id"`
	User1ID               string        `json:"user1_id"`
	User2ID               string        `json:"user2_id"`
	User1NativeLanguage   string        `json:"user1_native_language"`
	User1PracticeLanguage string        `json:"user1_practice_language"`
	User2NativeLanguage   string        `json:"user2_native_language"`
	User2PracticeLanguage string        `json:"user2_practice_language"`
	Status                SessionStatus `json:"status"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	EndedAt               *time.Time    `json:"ended_at,omitempty"`
	DurationSeconds       *int32        `json:"duration_seconds,omitempty"`
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
	session := &Session{
		ID:                    uuid.New(),
		User1ID:               user1ID,
		User2ID:               user2ID,
		User1NativeLanguage:   user1Native,
		User1PracticeLanguage: user1Practice,
		User2NativeLanguage:   user2Native,
		User2PracticeLanguage: user2Practice,
		Status:                SessionActive,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	query := `
		INSERT INTO sessions (id, user1_id, user2_id, user1_native_language, user1_practice_language, 
		                      user2_native_language, user2_practice_language, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		session.ID,
		session.User1ID,
		session.User2ID,
		session.User1NativeLanguage,
		session.User1PracticeLanguage,
		session.User2NativeLanguage,
		session.User2PracticeLanguage,
		session.Status,
		session.CreatedAt,
		session.UpdatedAt,
	).Scan(&session.ID, &session.CreatedAt, &session.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return session, nil
}

func (r *Repository) GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	session := &Session{}

	query := `
		SELECT id, user1_id, user2_id, user1_native_language, user1_practice_language,
		       user2_native_language, user2_practice_language, status, created_at, updated_at,
		       ended_at, duration_seconds
		FROM sessions
		WHERE id = $1`

	err := r.db.QueryRow(ctx, query, sessionID).Scan(
		&session.ID,
		&session.User1ID,
		&session.User2ID,
		&session.User1NativeLanguage,
		&session.User1PracticeLanguage,
		&session.User2NativeLanguage,
		&session.User2PracticeLanguage,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.EndedAt,
		&session.DurationSeconds,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return session, nil
}

func (r *Repository) GetActiveSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	session := &Session{}

	query := `
		SELECT id, user1_id, user2_id, user1_native_language, user1_practice_language,
		       user2_native_language, user2_practice_language, status, created_at, updated_at,
		       ended_at, duration_seconds
		FROM sessions
		WHERE (user1_id = $1 OR user2_id = $1) AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1`

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&session.ID,
		&session.User1ID,
		&session.User2ID,
		&session.User1NativeLanguage,
		&session.User1PracticeLanguage,
		&session.User2NativeLanguage,
		&session.User2PracticeLanguage,
		&session.Status,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.EndedAt,
		&session.DurationSeconds,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return session, nil
}

func (r *Repository) UpdateSessionStatus(ctx context.Context, sessionID uuid.UUID, status SessionStatus) error {
	var query string
	var args []interface{}

	if status == SessionCompleted || status == SessionCancelled {
		query = `
			UPDATE sessions 
			SET status = $1, ended_at = CURRENT_TIMESTAMP, 
			    duration_seconds = EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - created_at))::INTEGER
			WHERE id = $2`
		args = []interface{}{status, sessionID}
	} else {
		query = `UPDATE sessions SET status = $1 WHERE id = $2`
		args = []interface{}{status, sessionID}
	}

	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *Repository) GetUserSessions(ctx context.Context, userID string, limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, user1_id, user2_id, user1_native_language, user1_practice_language,
		       user2_native_language, user2_practice_language, status, created_at, updated_at,
		       ended_at, duration_seconds
		FROM sessions
		WHERE user1_id = $1 OR user2_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := r.db.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		err := rows.Scan(
			&session.ID,
			&session.User1ID,
			&session.User2ID,
			&session.User1NativeLanguage,
			&session.User1PracticeLanguage,
			&session.User2NativeLanguage,
			&session.User2PracticeLanguage,
			&session.Status,
			&session.CreatedAt,
			&session.UpdatedAt,
			&session.EndedAt,
			&session.DurationSeconds,
		)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}