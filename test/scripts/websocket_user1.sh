#!/bin/bash

# WebSocket connection script for User 1
# Requires websocat to be installed: brew install websocat

USER_ID="user1"
WS_URL="ws://localhost:8080/ws?user_id=$USER_ID"

echo "🔌 Connecting to WebSocket for User 1 (Spanish native, English practice)..."
echo "URL: $WS_URL"
echo ""
echo "Waiting for match notifications..."
echo "Press Ctrl+C to disconnect"
echo ""

if command -v websocat &> /dev/null; then
  websocat "$WS_URL"
else
  echo "❌ websocat not found. Install with: brew install websocat"
  echo ""
  echo "Alternative: Use any WebSocket client to connect to:"
  echo "$WS_URL"
fi