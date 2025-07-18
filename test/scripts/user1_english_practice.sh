#!/bin/bash

# Test script for User 1: Practices English, Native Spanish speaker
# This user should match with someone who practices Spanish and speaks English natively

BASE_URL="http://localhost:8080"
USER_ID="user1"
NATIVE_LANG="Spanish"
PRACTICE_LANG="English"

echo "🇪🇸→🇺🇸 User 1: Native Spanish speaker wanting to practice English"
echo "User ID: $USER_ID"
echo "Native Language: $NATIVE_LANG"
echo "Practice Language: $PRACTICE_LANG"
echo ""

# Join matchmaking queue
echo "📝 Joining matchmaking queue..."
RESPONSE=$(curl -s -X POST "$BASE_URL/queue" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER_ID\",
    \"native_language\": \"$NATIVE_LANG\",
    \"practice_language\": \"$PRACTICE_LANG\"
  }")

if [ $? -eq 0 ]; then
  echo "✅ Successfully joined queue"
  echo "Response: $RESPONSE"
else
  echo "❌ Failed to join queue"
  exit 1
fi

echo ""
echo "🔌 To receive match notifications, open WebSocket connection in another terminal:"
echo "websocat ws://localhost:8080/ws?user_id=$USER_ID"
echo ""
echo "To cancel matchmaking, run:"
echo "curl -X DELETE \"$BASE_URL/queue\" -H \"Content-Type: application/json\" -d '{\"user_id\": \"$USER_ID\"}'"