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

## Testing

### Local Development Testing Scripts

**⚠️ Note: These scripts are for local development testing only and should not be used in production.**

The `test/scripts/` directory contains shell scripts to test the matchmaking functionality locally:

#### Prerequisites
- Server running locally (`go run main.go`)
- Redis running (`docker-compose up -d redis`)
- Optional: `websocat` for WebSocket testing (`brew install websocat`)

#### Available Test Scripts

**Individual User Scripts:**
- `./test/scripts/user1_english_practice.sh` - Simulates Spanish native speaker wanting to practice English
- `./test/scripts/user2_spanish_practice.sh` - Simulates English native speaker wanting to practice Spanish

**WebSocket Connection Scripts:**
- `./test/scripts/websocket_user1.sh` - Opens WebSocket connection for User 1
- `./test/scripts/websocket_user2.sh` - Opens WebSocket connection for User 2

**Complete Test:**
- `./test/scripts/test_match.sh` - Runs full matching test with both users

#### How to Test Matchmaking

1. **Basic API Test:**
   ```bash
   ./test/scripts/test_match.sh
   ```

2. **Real-time WebSocket Test:**
   ```bash
   # Terminal 1: Connect User 1's WebSocket
   ./test/scripts/websocket_user1.sh
   
   # Terminal 2: Connect User 2's WebSocket  
   ./test/scripts/websocket_user2.sh
   
   # Terminal 3: Run the matchmaking test
   ./test/scripts/test_match.sh
   ```

The scripts simulate two complementary users who should match with each other:
- User 1: Native Spanish speaker practicing English
- User 2: Native English speaker practicing Spanish

When both users join the queue, they should be automatically matched and receive WebSocket notifications.