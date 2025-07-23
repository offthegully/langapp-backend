package postgres

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type PostgresClient struct {
	pool *pgxpool.Pool
}

func NewPostgresClient(ctx context.Context) *PostgresClient {
	// Default connection parameters for local development
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USER", "langapp")
	password := getEnv("POSTGRES_PASSWORD", "langapp_dev")
	dbname := getEnv("POSTGRES_DB", "langapp")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("Unable to parse postgres config: %v", err)
	}

	// Configure connection pool
	config.MaxConns = 25
	config.MinConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Unable to create postgres connection pool: %v", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Unable to connect to postgres: %v", err)
	}

	log.Printf("Connected to PostgreSQL database: %s", dbname)

	return &PostgresClient{
		pool: pool,
	}
}

func (pc *PostgresClient) Close() {
	pc.pool.Close()
}

func (pc *PostgresClient) GetPool() *pgxpool.Pool {
	return pc.pool
}

func (pc *PostgresClient) Ping(ctx context.Context) error {
	return pc.pool.Ping(ctx)
}

func (pc *PostgresClient) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return pc.pool.Query(ctx, sql, args...)
}

func (pc *PostgresClient) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return pc.pool.QueryRow(ctx, sql, args...)
}

func (pc *PostgresClient) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pc.pool.Exec(ctx, sql, args...)
}

func (pc *PostgresClient) Begin(ctx context.Context) (pgx.Tx, error) {
	return pc.pool.Begin(ctx)
}

//go:embed migrations/*.sql
var embedMigrations embed.FS

func (pc *PostgresClient) RunMigrations() error {
	goose.SetBaseFS(embedMigrations)

	// Set the dialect for Goose
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	db := stdlib.OpenDBFromPool(pc.pool)
	// Run migrations
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	err := db.Close()
	if err != nil {
		return fmt.Errorf("failed to close temp db connection: %w", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
