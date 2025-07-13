# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a backend monolith for a language exchange application that provides matchmaking services to connect users for audio chats based on their native and practice languages.

## Tech Stack

- **Language**: Go 1.23
- **Web Framework**: Chi router (lightweight, fast)
- **Database**: PostgreSQL (user data, chat sessions)
- **Cache/Queue**: Redis (matchmaking queue, real-time data)
- **Background Jobs**: Asynq (Redis-backed job processing)
- **WebSockets**: Gorilla WebSocket (real-time notifications)
- **Database Driver**: pgx/v5 (high-performance PostgreSQL driver)

## Development Commands

```bash
# Start development environment
docker-compose up -d

# Run the server
go run cmd/server/main.go

# Run database migrations
# Manual execution needed - migrations are in migrations/ folder

# Build the application
go build -o bin/server cmd/server/main.go

# Run tests (when implemented)
go test ./...
```

## Project Structure

```
cmd/server/           # Application entry point
internal/
├── api/handlers/     # HTTP handlers
├── queue/           # Matchmaking queue logic
├── sessions/        # Chat session management
├── storage/         # Database models and operations
└── config/          # Configuration management
migrations/          # Database schema migrations
```

## Core Architecture

### Matchmaking Flow
1. User requests match via HTTP POST to `/api/v1/match/request`
2. Request added to Redis queue with language preferences
3. Background job processes queue to find compatible matches
4. WebSocket notifications sent when match found
5. Chat session created and managed through WebSocket connections

### Database Design
- **users**: User profiles with native languages
- **match_requests**: Temporary match requests (main queue in Redis)
- **chat_sessions**: Persistent chat session records

### Real-time Components
- Redis pub/sub for match notifications
- WebSocket connections for live chat coordination
- Background job processing for continuous matching

## Environment Variables

```
PORT=8080
DATABASE_URL=postgres://langapp:password@localhost:5432/language_exchange?sslmode=disable
REDIS_URL=redis://localhost:6379
QUEUE_DEFAULT_TIMEOUT=5m
MATCHING_INTERVAL=2s
```