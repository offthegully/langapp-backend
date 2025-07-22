package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Manager struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

type Client struct {
	ID      string
	conn    *websocket.Conn
	send    chan []byte
	manager *Manager
}

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type MatchNotification struct {
	MatchID   string `json:"match_id"`
	PartnerID string `json:"partner_id"`
	Language  string `json:"language"`
	Message   string `json:"message"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewManager() *Manager {
	return &Manager{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (m *Manager) Start() {
	for {
		select {
		case client := <-m.register:
			m.mutex.Lock()
			m.clients[client.ID] = client
			m.mutex.Unlock()
			log.Printf("Client %s connected", client.ID)

		case client := <-m.unregister:
			m.mutex.Lock()
			if _, exists := m.clients[client.ID]; exists {
				delete(m.clients, client.ID)
				close(client.send)
				log.Printf("Client %s disconnected", client.ID)
			}
			m.mutex.Unlock()
		}
	}
}

func (m *Manager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:      userID,
		conn:    conn,
		send:    make(chan []byte, 256),
		manager: m,
	}

	m.register <- client

	go client.writePump()
	go client.readPump()
}

func (m *Manager) NotifyMatch(userID string, notification MatchNotification) error {
	message := Message{
		Type: "match_found",
		Data: notification,
	}

	return m.SendMessage(userID, message)
}

func (m *Manager) SendMessage(userID string, message Message) error {
	m.mutex.RLock()
	client, exists := m.clients[userID]
	m.mutex.RUnlock()

	if !exists {
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	select {
	case client.send <- data:
	default:
		close(client.send)
		delete(m.clients, userID)
	}

	return nil
}

func (c *Client) readPump() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}
