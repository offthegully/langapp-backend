package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"langapp-backend/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking for production
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WSManager struct {
	storage           *storage.Storage
	connections       map[string]*websocket.Conn // userID -> connection
	connectionMetrics map[string]*ConnectionMetrics
	mu                sync.RWMutex
}

type ConnectionMetrics struct {
	UserID        string
	ConnectedAt   time.Time
	LastPing      time.Time
	LastPong      time.Time
	MessagesSent  int64
	MessagesRecv  int64
	ClientIP      string
	UserAgent     string
}

func NewWSManager(storage *storage.Storage) *WSManager {
	return &WSManager{
		storage:           storage,
		connections:       make(map[string]*websocket.Conn),
		connectionMetrics: make(map[string]*ConnectionMetrics),
	}
}

type WSMessage struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// HandleMatchWebSocket handles WebSocket connections for match notifications
func (wm *WSManager) HandleMatchWebSocket(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	connectionID := fmt.Sprintf("ws_%d_%s", time.Now().UnixNano(), generateShortID())
	clientIP := wm.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")
	
	userID := chi.URLParam(r, "userID")
	log.Printf("[WS_CONNECT] %s - WebSocket connection attempt from IP: %s, UserID: %s, User-Agent: %s", 
		connectionID, clientIP, userID, userAgent)
	
	if userID == "" {
		log.Printf("[WS_CONNECT] %s - Missing user_id parameter", connectionID)
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}

	upgradeStart := time.Now()
	conn, err := upgrader.Upgrade(w, r, nil)
	upgradeDuration := time.Since(upgradeStart)
	if err != nil {
		log.Printf("[WS_CONNECT] %s - WebSocket upgrade failed after %v: %v", connectionID, upgradeDuration, err)
		return
	}
	log.Printf("[WS_CONNECT] %s - WebSocket upgrade successful in %v", connectionID, upgradeDuration)
	defer conn.Close()

	// Register connection and metrics
	wm.mu.Lock()
	// Check if user already has a connection
	if existingConn, exists := wm.connections[userID]; exists {
		log.Printf("[WS_CONNECT] %s - Closing existing connection for user %s", connectionID, userID)
		existingConn.Close()
		delete(wm.connectionMetrics, userID)
	}
	
	wm.connections[userID] = conn
	wm.connectionMetrics[userID] = &ConnectionMetrics{
		UserID:       userID,
		ConnectedAt:  time.Now(),
		LastPing:     time.Now(),
		ClientIP:     clientIP,
		UserAgent:    userAgent,
	}
	totalConnections := len(wm.connections)
	wm.mu.Unlock()
	
	log.Printf("[WS_CONNECT] %s - User %s connected successfully, total connections: %d", 
		connectionID, userID, totalConnections)

	// Cleanup on disconnect
	defer func() {
		connectionDuration := time.Since(start)
		wm.mu.Lock()
		metrics := wm.connectionMetrics[userID]
		delete(wm.connections, userID)
		delete(wm.connectionMetrics, userID)
		totalConnections := len(wm.connections)
		wm.mu.Unlock()
		
		if metrics != nil {
			log.Printf("[WS_DISCONNECT] %s - User %s disconnected after %v, sent: %d msgs, recv: %d msgs, total connections: %d", 
				connectionID, userID, connectionDuration, metrics.MessagesSent, metrics.MessagesRecv, totalConnections)
			log.Printf("[WS_DISCONNECT_METRICS] ConnectionID=%s UserID=%s Duration=%v MessagesSent=%d MessagesRecv=%d ClientIP=%s", 
				connectionID, userID, connectionDuration, metrics.MessagesSent, metrics.MessagesRecv, clientIP)
		} else {
			log.Printf("[WS_DISCONNECT] %s - User %s disconnected after %v, total connections: %d", 
				connectionID, userID, connectionDuration, totalConnections)
		}
	}()

	// Subscribe to Redis pub/sub for this user
	subscribeStart := time.Now()
	pubsub := wm.storage.Redis.SubscribeToUserEvents(r.Context(), userID)
	subscribeDuration := time.Since(subscribeStart)
	log.Printf("[WS_CONNECT] %s - Subscribed to Redis events for user %s in %v", 
		connectionID, userID, subscribeDuration)
	defer func() {
		log.Printf("[WS_CONNECT] %s - Closing Redis subscription for user %s", connectionID, userID)
		pubsub.Close()
	}()

	// Handle incoming messages and Redis notifications
	log.Printf("[WS_CONNECT] %s - Starting Redis message handler for user %s", connectionID, userID)
	go wm.handleRedisMessages(connectionID, userID, pubsub, conn)

	// Keep connection alive and handle client messages
	log.Printf("[WS_CONNECT] %s - Starting connection handler for user %s", connectionID, userID)
	totalConnectionTime := time.Since(start)
	log.Printf("[WS_CONNECT_METRICS] ConnectionID=%s UserID=%s SetupDuration=%v UpgradeDuration=%v SubscribeDuration=%v ClientIP=%s", 
		connectionID, userID, totalConnectionTime, upgradeDuration, subscribeDuration, clientIP)
	wm.handleConnection(connectionID, userID, conn)
}

func (wm *WSManager) handleRedisMessages(connectionID, userID string, pubsub *storage.RedisSubscriber, conn *websocket.Conn) {
	log.Printf("[WS_REDIS] %s - Starting Redis message handler for user %s", connectionID, userID)
	messagesProcessed := 0
	
	for {
		receiveStart := time.Now()
		msg, err := pubsub.ReceiveMessage(context.Background())
		receiveDuration := time.Since(receiveStart)
		
		if err != nil {
			log.Printf("[WS_REDIS] %s - Redis pubsub error for user %s after %v (processed %d messages): %v", 
				connectionID, userID, receiveDuration, messagesProcessed, err)
			return
		}

		log.Printf("[WS_REDIS] %s - Received Redis message for user %s in %v: %s", 
			connectionID, userID, receiveDuration, msg.Payload)

		var data map[string]interface{}
		unmarshalStart := time.Now()
		if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
			log.Printf("[WS_REDIS] %s - Failed to unmarshal Redis message for user %s after %v: %v", 
				connectionID, userID, time.Since(unmarshalStart), err)
			continue
		}

		wsMsg := WSMessage{
			Type:      data["type"].(string),
			Data:      data,
			Timestamp: time.Now().UTC(),
		}

		sendStart := time.Now()
		if err := conn.WriteJSON(wsMsg); err != nil {
			sendDuration := time.Since(sendStart)
			log.Printf("[WS_REDIS] %s - Failed to send WebSocket message to user %s after %v: %v", 
				connectionID, userID, sendDuration, err)
			return
		}
		sendDuration := time.Since(sendStart)
		
		messagesProcessed++
		wm.incrementMessagesSent(userID)
		
		log.Printf("[WS_REDIS] %s - Successfully sent message to user %s in %v (type: %s)", 
			connectionID, userID, sendDuration, wsMsg.Type)
	}
}

func (wm *WSManager) handleConnection(connectionID, userID string, conn *websocket.Conn) {
	log.Printf("[WS_HANDLER] %s - Starting connection handler for user %s", connectionID, userID)
	
	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	
	// Set pong handler to reset read deadline
	conn.SetPongHandler(func(string) error {
		wm.updateLastPong(userID)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		log.Printf("[WS_PONG] %s - Received pong from user %s", connectionID, userID)
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	messagesReceived := 0

	// Read messages from client
	go func() {
		defer close(done)
		log.Printf("[WS_READER] %s - Starting message reader for user %s", connectionID, userID)
		
		for {
			readStart := time.Now()
			var msg WSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				readDuration := time.Since(readStart)
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[WS_READER] %s - Unexpected WebSocket error for user %s after %v (received %d messages): %v", 
						connectionID, userID, readDuration, messagesReceived, err)
				} else {
					log.Printf("[WS_READER] %s - WebSocket closed for user %s after %v (received %d messages)", 
						connectionID, userID, readDuration, messagesReceived)
				}
				return
			}
			readDuration := time.Since(readStart)
			messagesReceived++
			
			log.Printf("[WS_READER] %s - Received message from user %s in %v (type: %s)", 
				connectionID, userID, readDuration, msg.Type)
			
			wm.incrementMessagesReceived(userID)
			
			// Handle client messages (heartbeat, status requests, etc.)
			wm.handleClientMessage(connectionID, userID, msg, conn)
		}
	}()

	// Send periodic pings
	pingCount := 0
	log.Printf("[WS_HANDLER] %s - Starting ping loop for user %s", connectionID, userID)
	
	for {
		select {
		case <-ticker.C:
			pingStart := time.Now()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				pingDuration := time.Since(pingStart)
				log.Printf("[WS_PING] %s - Failed to send ping to user %s after %v (sent %d pings): %v", 
					connectionID, userID, pingDuration, pingCount, err)
				return
			}
			pingDuration := time.Since(pingStart)
			pingCount++
			wm.updateLastPing(userID)
			log.Printf("[WS_PING] %s - Sent ping %d to user %s in %v", connectionID, pingCount, userID, pingDuration)
			
		case <-done:
			log.Printf("[WS_HANDLER] %s - Connection handler finished for user %s (pings: %d, messages: %d)", 
				connectionID, userID, pingCount, messagesReceived)
			return
		}
	}
}

func (wm *WSManager) handleClientMessage(connectionID, userID string, msg WSMessage, conn *websocket.Conn) {
	start := time.Now()
	log.Printf("[WS_MESSAGE] %s - Handling client message from user %s (type: %s)", connectionID, userID, msg.Type)
	
	switch msg.Type {
	case "ping":
		response := WSMessage{
			Type:      "pong",
			Data:      map[string]interface{}{"user_id": userID},
			Timestamp: time.Now().UTC(),
		}
		
		writeStart := time.Now()
		if err := conn.WriteJSON(response); err != nil {
			log.Printf("[WS_MESSAGE] %s - Failed to send pong to user %s after %v: %v", 
				connectionID, userID, time.Since(writeStart), err)
			return
		}
		writeDuration := time.Since(writeStart)
		wm.incrementMessagesSent(userID)
		log.Printf("[WS_MESSAGE] %s - Sent pong to user %s in %v", connectionID, userID, writeDuration)

	case "queue_status":
		response := WSMessage{
			Type:      "queue_status_response",
			Data:      map[string]interface{}{"message": "use /api/v1/queue/status endpoint"},
			Timestamp: time.Now().UTC(),
		}
		
		writeStart := time.Now()
		if err := conn.WriteJSON(response); err != nil {
			log.Printf("[WS_MESSAGE] %s - Failed to send queue_status_response to user %s after %v: %v", 
				connectionID, userID, time.Since(writeStart), err)
			return
		}
		writeDuration := time.Since(writeStart)
		wm.incrementMessagesSent(userID)
		log.Printf("[WS_MESSAGE] %s - Sent queue_status_response to user %s in %v", connectionID, userID, writeDuration)

	default:
		log.Printf("[WS_MESSAGE] %s - Unknown message type from user %s: %s", connectionID, userID, msg.Type)
	}
	
	totalDuration := time.Since(start)
	log.Printf("[WS_MESSAGE] %s - Message handling completed for user %s in %v (type: %s)", 
		connectionID, userID, totalDuration, msg.Type)
}

// SendMatchNotification sends a match notification to a specific user
func (wm *WSManager) SendMatchNotification(userID, sessionID string) error {
	start := time.Now()
	notificationID := fmt.Sprintf("notify_%d_%s", time.Now().UnixNano(), generateShortID())
	
	log.Printf("[WS_NOTIFY] %s - Sending match notification to user %s for session %s", 
		notificationID, userID, sessionID)
	
	wm.mu.RLock()
	conn, exists := wm.connections[userID]
	wm.mu.RUnlock()

	if !exists {
		log.Printf("[WS_NOTIFY] %s - User %s not connected via WebSocket, relying on Redis pub/sub", 
			notificationID, userID)
		return nil
	}

	msg := WSMessage{
		Type: "match_found",
		Data: map[string]interface{}{
			"session_id": sessionID,
			"message":    "Match found! You have 30 seconds to accept.",
		},
		Timestamp: time.Now().UTC(),
	}

	writeStart := time.Now()
	err := conn.WriteJSON(msg)
	writeDuration := time.Since(writeStart)
	totalDuration := time.Since(start)
	
	if err != nil {
		log.Printf("[WS_NOTIFY] %s - Failed to send match notification to user %s after %v: %v", 
			notificationID, userID, writeDuration, err)
		return err
	}
	
	wm.incrementMessagesSent(userID)
	log.Printf("[WS_NOTIFY] %s - Successfully sent match notification to user %s in %v (total: %v)", 
		notificationID, userID, writeDuration, totalDuration)
	log.Printf("[WS_NOTIFY_METRICS] NotificationID=%s UserID=%s SessionID=%s Duration=%v WriteDuration=%v", 
		notificationID, userID, sessionID, totalDuration, writeDuration)
	
	return nil
}

// BroadcastToSession sends a message to all users in a chat session
func (wm *WSManager) BroadcastToSession(sessionID string, msgType string, data map[string]interface{}) {
	start := time.Now()
	broadcastID := fmt.Sprintf("broadcast_%d_%s", time.Now().UnixNano(), generateShortID())
	
	log.Printf("[WS_BROADCAST] %s - Broadcasting to session %s (type: %s)", 
		broadcastID, sessionID, msgType)
	
	// This would be used for chat session management
	// Implementation depends on how you track session participants
	// For now, just log the operation
	
	totalDuration := time.Since(start)
	log.Printf("[WS_BROADCAST] %s - Broadcast completed for session %s in %v", 
		broadcastID, sessionID, totalDuration)
	log.Printf("[WS_BROADCAST_METRICS] BroadcastID=%s SessionID=%s MessageType=%s Duration=%v", 
		broadcastID, sessionID, msgType, totalDuration)
}

// GetConnectedUsers returns the list of currently connected user IDs
func (wm *WSManager) GetConnectedUsers() []string {
	start := time.Now()
	
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	users := make([]string, 0, len(wm.connections))
	for userID := range wm.connections {
		users = append(users, userID)
	}
	
	duration := time.Since(start)
	log.Printf("[WS_STATS] Retrieved %d connected users in %v", len(users), duration)
	
	return users
}

// Helper functions for logging and metrics
func generateShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
}

func (wm *WSManager) getClientIP(r *http.Request) string {
	// Check for forwarded headers first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

func (wm *WSManager) incrementMessagesSent(userID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if metrics, exists := wm.connectionMetrics[userID]; exists {
		metrics.MessagesSent++
	}
}

func (wm *WSManager) incrementMessagesReceived(userID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if metrics, exists := wm.connectionMetrics[userID]; exists {
		metrics.MessagesRecv++
	}
}

func (wm *WSManager) updateLastPing(userID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if metrics, exists := wm.connectionMetrics[userID]; exists {
		metrics.LastPing = time.Now()
	}
}

func (wm *WSManager) updateLastPong(userID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if metrics, exists := wm.connectionMetrics[userID]; exists {
		metrics.LastPong = time.Now()
	}
}

// GetConnectionMetrics returns connection metrics for debugging
func (wm *WSManager) GetConnectionMetrics() map[string]*ConnectionMetrics {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	
	metrics := make(map[string]*ConnectionMetrics)
	for userID, connMetrics := range wm.connectionMetrics {
		// Create a copy to avoid race conditions
		metrics[userID] = &ConnectionMetrics{
			UserID:        connMetrics.UserID,
			ConnectedAt:   connMetrics.ConnectedAt,
			LastPing:      connMetrics.LastPing,
			LastPong:      connMetrics.LastPong,
			MessagesSent:  connMetrics.MessagesSent,
			MessagesRecv:  connMetrics.MessagesRecv,
			ClientIP:      connMetrics.ClientIP,
			UserAgent:     connMetrics.UserAgent,
		}
	}
	
	log.Printf("[WS_METRICS] Retrieved metrics for %d connections", len(metrics))
	return metrics
}