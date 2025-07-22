# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Language Exchange App Backend

A monolith Go backend API for a language exchange application using Chi framework with PostgreSQL and Redis.

## Application Concept
This is a language exchange platform that connects users for language practice through audio calls. Users specify:
- **Native Language**: The language they speak fluently
- **Practice Language**: The language they want to learn/practice

The matching algorithm pairs users where:
- User A's practice language = User B's native language, AND
- User A's native language = User B's practice language

**Single Language Sessions**: Each match results in a session focused on practicing ONE language only. When two users with reciprocal language needs are matched, the system selects the practice language of the user who has been waiting in the queue longer. This ensures:
- One user practices their target language
- The other user helps as a native speaker
- Clear focus on a single language per session

**Example**: If User 1 (Spanish native, practicing English) has been queued for 2 minutes and User 2 (English native, practicing Spanish) joins the queue, the session language will be English (User 1's practice language). User 1 will practice English while User 2 helps as a native English speaker.

## Project Structure
- `main.go` - Entry point, starts HTTP server and initializes services with database migrations
- `api/` - API layer containing routing and handlers
  - `router.go` - Chi router setup with middleware and route definitions
  - `matchmaking.go` - Matchmaking API handlers (StartMatchmaking, CancelMatchmaking)
  - `languages.go` - Languages API handler (GetLanguagesHandler)
- `matchmaking/` - Core matchmaking business logic
  - `queue.go` - MatchmakingService and QueueEntry structs with Redis interface
  - `matching.go` - MatchingService for real-time match discovery and WebSocket notifications
- `storage/` - Data layer
  - `postgres.go` - PostgreSQL client with connection pooling and migrations
  - `redis.go` - Redis client configuration and factory
  - `migrations/` - Database migration files (embedded)
- `session/` - Session management
  - `session.go` - Session repository with CRUD operations and status tracking
- `languages/` - Language validation and constants
  - `languages.go` - Supported languages list and validation functions
- `websocket/` - WebSocket connection management
  - `manager.go` - WebSocket client management and real-time messaging
- `docker-compose.yml` - Redis and PostgreSQL containers for local development
- `openapi.yaml` - OpenAPI 3.0 specification for all endpoints

## Development Commands
- Start services: `docker-compose up -d`
- Start server: `go run main.go`
- Install dependencies: `go mod tidy`
- Format code: `go fmt ./...`
- Lint code: `go vet ./...` 
- Build: `go build`
- Build binary: `go build -o langapp-backend`
- Run tests: `go test ./...` (no tests currently exist)
- Test specific package: `go test ./matchmaking`
- Test with verbose output: `go test -v ./...`
- Stop services: `docker-compose down`

## Testing Guidelines
- **Manual Testing**: Do NOT run manual tests using the test scripts when making code changes. Only run unit tests with `go test ./...`
- **Test Scripts**: The scripts in `test/scripts/` are for user verification only, not for automated testing during development
- **Future Testing**: Once unit tests are implemented, use `go test ./...` to verify changes instead of manual testing

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
- Key packages: Chi router (v5.2.2), PostgreSQL driver (pgx/v5), Redis client (v9.11.0), Gorilla WebSocket (v1.5.3), Goose migrations (v3.24.3), UUID generation (google/uuid)
- External services: PostgreSQL (local development), Redis 7 (containerized via Docker)
- No external linting tools configured - use standard Go tooling

## Database Configuration
**PostgreSQL Environment Variables** (defaults for local development):
- `POSTGRES_HOST=localhost`
- `POSTGRES_PORT=5432`
- `POSTGRES_USER=langapp`
- `POSTGRES_PASSWORD=langapp_dev`
- `POSTGRES_DB=langapp`

**Connection Pool**: 25 max connections, 5 min connections

## Architecture Patterns
- **Clean dependency injection**: main.go creates and wires all dependencies with proper initialization
- **Database migrations**: Embedded SQL migrations run automatically on startup using Goose
- **Dual storage**: PostgreSQL for persistent data (sessions, languages), Redis for temporary matchmaking queue
- **Interface segregation**: Clean interfaces for database operations
- **Defensive initialization**: Each component ensures its dependencies are properly initialized
- **Separation of concerns**: API handlers in api/, business logic in matchmaking/, data access in storage/
- **API Handler Location**: ALL API handlers MUST be placed in the /api package - no exceptions
- **Chi middleware**: Logging and panic recovery at HTTP layer
- **JSON validation**: Request/response validation with struct tags
- **Language validation**: Database-driven language validation with repository pattern

**Code Quality Note**: The current dependency injection setup and clean architecture patterns should be maintained. Strive to keep code organized with clear interfaces, proper initialization, and separation of concerns.

## API Endpoints
- `GET /languages` - Returns list of supported languages from database
- `POST /queue` - Join matchmaking queue (requires user_id, native_language, practice_language)
- `DELETE /queue` - Cancel queue participation (requires user_id)
- `GET /ws?user_id={id}` - WebSocket connection for real-time match notifications

## Key Types
- `matchmaking.QueueEntry` - User queue data with languages and timestamp
- `matchmaking.MatchmakingService` - Service with Redis dependency for queue operations
- `matchmaking.MatchingService` - Service for real-time match discovery and notifications
- `session.Session` - Session entity with status tracking and duration calculation
- `session.Repository` - Database repository for session CRUD operations
- `languages.Language` - Language struct with Name and ShortName fields
- `websocket.Manager` - WebSocket connection manager for real-time messaging

## Service Integration
- API handlers call service methods (note: method calls should be on service instance, not package functions)
- Language validation happens via database repository using languages service
- Redis operations abstracted through MatchmakingService interface
- PostgreSQL operations abstracted through repository pattern
- Session creation and tracking handled by session repository

## Redis Architecture
The system uses Redis for real-time matchmaking with the following patterns:
- **Queue Storage**: Users are stored in language-specific queues (`queue:{language}`) using Redis lists
- **User Data Hash**: User details stored in a central hash (`users:data`) with user_id as key
- **Pub/Sub Channels**: Language-specific channels (`matchmaking:{language}`) for real-time matching
- **FIFO Matching**: Users are matched in first-in-first-out order using `LPOP` operations
- **Atomic Operations**: Pipeline operations ensure data consistency during queue operations

## WebSocket System
- **Connection Management**: WebSocket manager maintains active connections per user
- **Match Notifications**: Real-time notifications sent to both users when matched
- **Connection URL**: Dynamic WebSocket URL generation based on request context
- **Error Handling**: Graceful handling of connection failures and cleanup

## Matching Algorithm Details
1. **Queue Addition**: User joins queue for their practice language and publishes to their native language channel
2. **Real-time Listening**: Matching service listens to all language channels simultaneously
3. **Complementary Matching**: When a user publishes, system looks for complementary users in appropriate queue
4. **Session Creation**: Successful matches create database sessions before user notification
5. **Queue Cleanup**: Both users removed from all queues after successful matching
6. **Failure Recovery**: Practice users restored to queue if session creation fails

## Current Implementation Status
- ✅ Complete API structure with validation
- ✅ PostgreSQL client with connection pooling and embedded migrations
- ✅ Redis client setup and configuration
- ✅ Database-driven language validation and repository pattern
- ✅ Redis pub/sub system with language-specific channels
- ✅ Session management with status tracking and duration calculation
- ✅ AddToQueue implemented with Redis storage and pub/sub publishing
- ✅ Real-time matching service with complementary algorithm
- ✅ WebSocket support for instant match notifications
- ✅ OpenAPI specification documentation
- ✅ RemoveFromQueue implemented with Redis queue search and removal
- ✅ Test scripts for local development and WebSocket testing
- ❌ No formal test coverage exists (only manual test scripts)