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
- `main.go` - Entry point, starts the HTTP server
- `api/` - API layer containing routing and handlers
- `api/router.go` - Chi router setup and route definitions

## Coding Standards
- Use tabs for indentation (Go standard)
- Follow Go naming conventions (PascalCase for exported, camelCase for unexported)
- Use gofmt for code formatting
- Add JSON tags to structs for API responses
- Use meaningful variable names

## Common Workflows
- Start server: `go run main.go`
- Install dependencies: `go mod tidy`
- Format code: `go fmt ./...`
- Build: `go build`

## Architectural Patterns
- Monolith architecture with modular structure
- Separate routing logic from main application entry point
- Use Chi middleware for common functionality (logging, recovery)
- JSON responses for all API endpoints
- RESTful API design principles

## Development Guidelines
- All API endpoints should return JSON
- Use proper HTTP status codes
- Include request logging via Chi middleware
- Handle errors gracefully with proper HTTP responses
- Keep handlers focused and single-purpose