# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Language Exchange App Backend

A monolith Go backend API for a language exchange application using Chi framework.

## Application Concept
This is a language exchange platform that connects users for language practice through audio calls. Users specify:
- **Native Language**: The language they speak fluently
- **Practice Language**: The language they want to learn/practice

The matching algorithm pairs users where:
- User A's practice language = User B's native language, AND
- User A's native language = User B's practice language

Once matched, users engage in audio calls to practice their target languages with native speakers.

## Project Structure
- `main.go` - Entry point, starts HTTP server and initializes services
- `api/` - API layer containing routing and handlers
  - `router.go` - Chi router setup with middleware and route definitions
  - `matchmaking.go` - Matchmaking API handlers (StartMatchmaking, CancelMatchmaking)
  - `languages.go` - Languages API handler (GetLanguagesHandler)
- `matchmaking/` - Core matchmaking business logic
  - `queue.go` - MatchmakingService and QueueEntry structs with Redis interface
- `storage/` - Data layer
  - `redis.go` - Redis client configuration and factory
- `languages/` - Language validation and constants
  - `languages.go` - Supported languages list and validation functions
- `docker-compose.yml` - Redis container for local development

## Development Commands
- Start Redis: `docker-compose up -d redis`
- Start server: `go run main.go`
- Install dependencies: `go mod tidy`
- Format code: `go fmt ./...`
- Build: `go build`
- Stop services: `docker-compose down`

## Architecture Patterns
- Dependency injection pattern: main.go creates RedisClient and MatchmakingService
- Interface segregation: RedisClient interface in matchmaking package only exposes needed Redis methods
- Separation of concerns: API handlers in api/, business logic in matchmaking/, data access in storage/
- Chi middleware for logging and panic recovery
- JSON request/response validation with struct tags
- Language validation using predefined constants

## API Endpoints
- `GET /languages` - Returns list of supported languages
- `POST /queue` - Join matchmaking queue (requires user_id, native_language, practice_language)
- `DELETE /queue` - Cancel queue participation (requires user_id)

## Key Types
- `matchmaking.QueueEntry` - User queue data with languages and timestamp
- `matchmaking.MatchmakingService` - Service with Redis dependency for queue operations
- `languages.Language` - Language struct with Name and ShortName fields

## Service Integration
- API handlers call MatchmakingService methods (note: method calls should be on service instance, not package functions)
- Language validation happens in API layer using languages.IsValidLanguage()
- Redis operations abstracted through MatchmakingService interface