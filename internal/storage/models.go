package storage

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID `json:"id" db:"id"`
	Email           string    `json:"email" db:"email"`
	Username        string    `json:"username" db:"username"`
	NativeLanguages []string  `json:"native_languages" db:"native_languages"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

type MatchRequest struct {
	ID               uuid.UUID `json:"id" db:"id"`
	UserID           uuid.UUID `json:"user_id" db:"user_id"`
	NativeLanguages  []string  `json:"native_languages" db:"native_languages"`
	PracticeLanguage string    `json:"practice_language" db:"practice_language"`
	RequestedAt      time.Time `json:"requested_at" db:"requested_at"`
	ExpiresAt        time.Time `json:"expires_at" db:"expires_at"`
	Status           string    `json:"status" db:"status"`
}

type ChatSession struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	UserAID           uuid.UUID  `json:"user_a_id" db:"user_a_id"`
	UserBID           uuid.UUID  `json:"user_b_id" db:"user_b_id"`
	LanguageA         string     `json:"language_a" db:"language_a"`
	LanguageB         string     `json:"language_b" db:"language_b"`
	StartedAt         *time.Time `json:"started_at" db:"started_at"`
	EndedAt           *time.Time `json:"ended_at" db:"ended_at"`
	DurationMinutes   *int       `json:"duration_minutes" db:"duration_minutes"`
	Status            string     `json:"status" db:"status"`
	CompletedMinimum  bool       `json:"completed_minimum" db:"completed_minimum"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
}

// Session statuses
const (
	SessionWaiting    = "waiting"
	SessionConnecting = "connecting"
	SessionActive     = "active"
	SessionCanEnd     = "can_end"
	SessionEnded      = "ended"
)

// Match request statuses
const (
	MatchPending   = "pending"
	MatchMatched   = "matched"
	MatchCancelled = "cancelled"
	MatchExpired   = "expired"
)