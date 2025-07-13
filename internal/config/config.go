package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Queue    QueueConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxConnections  int
	MaxIdleTime     time.Duration
	MaxLifetime     time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type QueueConfig struct {
	DefaultTimeout    time.Duration
	MatchingInterval  time.Duration
	CleanupInterval   time.Duration
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  getDuration("READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("WRITE_TIMEOUT", 10*time.Second),
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", "postgres://langapp:password@localhost:5432/language_exchange?sslmode=disable"),
			MaxConnections:  getInt("DB_MAX_CONNECTIONS", 20),
			MaxIdleTime:     getDuration("DB_MAX_IDLE_TIME", 30*time.Minute),
			MaxLifetime:     getDuration("DB_MAX_LIFETIME", time.Hour),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Queue: QueueConfig{
			DefaultTimeout:   getDuration("QUEUE_DEFAULT_TIMEOUT", 5*time.Minute),
			MatchingInterval: getDuration("MATCHING_INTERVAL", 2*time.Second),
			CleanupInterval:  getDuration("CLEANUP_INTERVAL", 30*time.Second),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}