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
  - `matching.go` - MatchingService for real-time match discovery and WebSocket notifications
- `storage/` - Data layer
  - `redis.go` - Redis client configuration and factory
- `languages/` - Language validation and constants
  - `languages.go` - Supported languages list and validation functions
- `websocket/` - WebSocket connection management
  - `manager.go` - WebSocket client management and real-time messaging
- `docker-compose.yml` - Redis container for local development
- `openapi.yaml` - OpenAPI 3.0 specification for all endpoints

## Development Commands
- Start Redis: `docker-compose up -d redis`
- Start server: `go run main.go`
- Install dependencies: `go mod tidy`
- Format code: `go fmt ./...`
- Build: `go build`
- Run tests: `go test ./...` (no tests currently exist)
- Test specific package: `go test ./matchmaking`
- Stop services: `docker-compose down`

## Test Scripts
Located in `test/scripts/` directory for local development testing:
- `./test/scripts/test_match.sh` - Complete matchmaking test with complementary users
- `./test/scripts/user1_english_practice.sh` - Spanish native speaker practicing English
- `./test/scripts/user2_spanish_practice.sh` - English native speaker practicing Spanish
- `./test/scripts/websocket_user1.sh` - WebSocket connection for User 1
- `./test/scripts/websocket_user2.sh` - WebSocket connection for User 2

**Prerequisites for WebSocket tests**: Install `websocat` with `brew install websocat`

## Dependencies
- Go version: 1.23.2
- Key packages: Chi router (v5.2.2), Redis client (v9.11.0), Gorilla WebSocket (v1.5.3)
- External services: Redis 7 (containerized via Docker)

## Architecture Patterns
- **Clean dependency injection**: main.go creates and wires all dependencies with proper initialization
- **Interface segregation**: RedisClient interface in matchmaking package only exposes needed Redis methods
- **Defensive initialization**: Each component ensures its dependencies are properly initialized (robust pattern)
- **Separation of concerns**: API handlers in api/, business logic in matchmaking/, data access in storage/
- **Chi middleware**: Logging and panic recovery at HTTP layer
- **JSON validation**: Request/response validation with struct tags
- **Language validation**: Using predefined constants and validation functions

**Code Quality Note**: The current dependency injection setup and clean architecture patterns should be maintained. Strive to keep code organized with clear interfaces, proper initialization, and separation of concerns.

## API Endpoints
- `GET /languages` - Returns list of supported languages
- `POST /queue` - Join matchmaking queue (requires user_id, native_language, practice_language)
- `DELETE /queue` - Cancel queue participation (requires user_id)
- `GET /ws?user_id={id}` - WebSocket connection for real-time match notifications

## Key Types
- `matchmaking.QueueEntry` - User queue data with languages and timestamp
- `matchmaking.MatchmakingService` - Service with Redis dependency for queue operations
- `matchmaking.MatchingService` - Service for real-time match discovery and notifications
- `languages.Language` - Language struct with Name and ShortName fields
- `websocket.Manager` - WebSocket connection manager for real-time messaging

## Service Integration
- API handlers call MatchmakingService methods (note: method calls should be on service instance, not package functions)
- Language validation happens in API layer using languages.IsValidLanguage()
- Redis operations abstracted through MatchmakingService interface

## Current Implementation Status
- ✅ Complete API structure with validation
- ✅ Redis client setup and configuration
- ✅ Language validation with 20 supported languages
- ✅ Redis pub/sub system with language-specific channels
- ✅ AddToQueue implemented with Redis storage and pub/sub publishing
- ✅ Real-time matching service with complementary algorithm
- ✅ WebSocket support for instant match notifications
- ✅ OpenAPI specification documentation
- ✅ RemoveFromQueue implemented with Redis queue search and removal
- ✅ Test scripts for local development and WebSocket testing
- ❌ No formal test coverage exists (only manual test scripts)