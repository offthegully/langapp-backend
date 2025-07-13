package storage

import (
	"context"
)

type Storage struct {
	DB    *PostgresDB
	Redis *RedisClient
}

func NewStorage(ctx context.Context, databaseURL, redisURL string) (*Storage, error) {
	db, err := NewPostgresDB(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	redisClient, err := NewRedisClient(ctx, redisURL)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Storage{
		DB:    db,
		Redis: redisClient,
	}, nil
}

func (s *Storage) Close() error {
	s.DB.Close()
	return s.Redis.Close()
}