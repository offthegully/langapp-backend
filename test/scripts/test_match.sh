#!/bin/bash

# Complete matchmaking test script
# This script demonstrates the full matching flow

echo "🧪 Language Exchange Matchmaking Test"
echo "======================================"
echo ""

# Check if server is running
echo "🔍 Checking if server is running..."
if ! curl -s http://localhost:8080/languages > /dev/null; then
  echo "❌ Server not running. Start with: go run main.go"
  exit 1
fi
echo "✅ Server is running"
echo ""

echo "📋 Test Plan:"
echo "1. User 1 joins queue (Spanish native, wants to practice English)"
echo "2. User 2 joins queue (English native, wants to practice Spanish)"
echo "3. System should match them automatically"
echo ""

read -p "Press Enter to start test..."
echo ""

# User 1 joins queue
echo "👤 User 1 joining queue..."
./test/scripts/user1_english_practice.sh
echo ""

echo "⏱️  Waiting 2 seconds..."
sleep 2
echo ""

# User 2 joins queue (should trigger match)
echo "👤 User 2 joining queue (should trigger match)..."
./test/scripts/user2_spanish_practice.sh
echo ""

echo "🎉 Test complete!"
echo ""
echo "💡 To test WebSocket notifications:"
echo "   Terminal 1: ./test/scripts/websocket_user1.sh"
echo "   Terminal 2: ./test/scripts/websocket_user2.sh"
echo "   Then run this test script again"