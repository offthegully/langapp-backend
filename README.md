# Language Exchange App Backend

A monolith backend API for a language exchange application built with Go and Chi framework.

## About the Application

This platform connects language learners for practice sessions through audio calls. Here's how it works:

1. **User Profile Setup**: Users specify their native language and the language they want to practice
2. **Smart Matching**: The system pairs users with complementary language needs:
   - Your practice language = Their native language
   - Your native language = Their practice language
3. **Audio Calls**: Once matched, users engage in real-time audio conversations to practice with native speakers

**Example**: An English speaker learning Spanish gets matched with a Spanish speaker learning English. Both users benefit by practicing their target language with a native speaker.

## Prerequisites

- Go 1.19 or later installed on your system
- Docker and Docker Compose for Redis

## Installation

1. Clone the repository
2. Install dependencies:

```bash
go mod tidy
```

3. Start Redis using Docker Compose:

```bash
docker-compose up -d redis
```

## Running the Application

1. Ensure Redis is running:

```bash
docker-compose up -d redis
```

2. Start the API server:

```bash
go run main.go
```

The server will start on port 8080 and connect to Redis on localhost:6379.

## API Endpoints

- `POST /queue` - Join the matchmaking queue
- `DELETE /queue` - Cancel queue participation

### Examples

**Join Queue:**
```bash
curl -X POST http://localhost:8080/queue \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user123", "native_language": "English", "practice_language": "Spanish"}'
```

**Cancel Queue:**
```bash
curl -X DELETE http://localhost:8080/queue \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user123"}'
```