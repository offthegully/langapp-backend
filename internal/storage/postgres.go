package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresDB struct {
	pool *pgxpool.Pool
}

func NewPostgresDB(ctx context.Context, databaseURL string) (*PostgresDB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 20
	config.MaxConnIdleTime = 30 * time.Minute
	config.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return &PostgresDB{pool: pool}, nil
}

func (db *PostgresDB) Close() {
	db.pool.Close()
}

func (db *PostgresDB) CreateUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (email, username, native_languages)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`

	return db.pool.QueryRow(ctx, query, user.Email, user.Username, user.NativeLanguages).
		Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (db *PostgresDB) GetUser(ctx context.Context, userID string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, email, username, native_languages, created_at, updated_at
		FROM users WHERE id = $1`

	err := db.pool.QueryRow(ctx, query, userID).Scan(
		&user.ID, &user.Email, &user.Username, &user.NativeLanguages,
		&user.CreatedAt, &user.UpdatedAt,
	)

	return user, err
}

func (db *PostgresDB) CreateChatSession(ctx context.Context, session *ChatSession) error {
	query := `
		INSERT INTO chat_sessions (user_a_id, user_b_id, language_a, language_b, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	return db.pool.QueryRow(ctx, query,
		session.UserAID, session.UserBID, session.LanguageA, session.LanguageB, session.Status).
		Scan(&session.ID, &session.CreatedAt)
}

func (db *PostgresDB) UpdateChatSession(ctx context.Context, session *ChatSession) error {
	query := `
		UPDATE chat_sessions 
		SET status = $2, started_at = $3, ended_at = $4, duration_minutes = $5, completed_minimum = $6
		WHERE id = $1`

	_, err := db.pool.Exec(ctx, query,
		session.ID, session.Status, session.StartedAt, session.EndedAt,
		session.DurationMinutes, session.CompletedMinimum)

	return err
}

func (db *PostgresDB) GetChatSession(ctx context.Context, sessionID string) (*ChatSession, error) {
	session := &ChatSession{}
	query := `
		SELECT id, user_a_id, user_b_id, language_a, language_b, started_at, 
		       ended_at, duration_minutes, status, completed_minimum, created_at
		FROM chat_sessions WHERE id = $1`

	err := db.pool.QueryRow(ctx, query, sessionID).Scan(
		&session.ID, &session.UserAID, &session.UserBID, &session.LanguageA, &session.LanguageB,
		&session.StartedAt, &session.EndedAt, &session.DurationMinutes, &session.Status,
		&session.CompletedMinimum, &session.CreatedAt,
	)

	return session, err
}