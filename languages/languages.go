package languages

import (
	"context"
	"time"

	"langapp-backend/storage/postgres"

	"github.com/jackc/pgx/v5"
)

type Language struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	ShortName string    `json:"short_name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Repository struct {
	db *postgres.PostgresClient
}

func NewRepository(db *postgres.PostgresClient) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) GetAllLanguages(ctx context.Context) ([]Language, error) {
	query := `
		SELECT id, name, short_name, is_active, created_at, updated_at
		FROM languages
		WHERE is_active = true
		ORDER BY name ASC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var languages []Language
	for rows.Next() {
		var lang Language
		err := rows.Scan(
			&lang.ID,
			&lang.Name,
			&lang.ShortName,
			&lang.IsActive,
			&lang.CreatedAt,
			&lang.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		languages = append(languages, lang)
	}

	return languages, nil
}

func (r *Repository) GetLanguageByName(ctx context.Context, name string) (*Language, error) {
	query := `
		SELECT id, name, short_name, is_active, created_at, updated_at
		FROM languages
		WHERE (name = $1 OR short_name = $1) AND is_active = true
		LIMIT 1`

	var lang Language
	err := r.db.QueryRow(ctx, query, name).Scan(
		&lang.ID,
		&lang.Name,
		&lang.ShortName,
		&lang.IsActive,
		&lang.CreatedAt,
		&lang.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &lang, nil
}
