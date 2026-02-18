package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/openpaw/openpaw/internal/auth"
	"github.com/openpaw/openpaw/internal/logger"
)

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string
	topics map[string]bool // subscribed topics for filtered broadcasts
	topMu  sync.Mutex
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	done       chan struct{}
	mu         sync.RWMutex
	auth       *auth.Service
	port       int
}

func NewHub(authService *auth.Service, port int) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
		auth:       authService,
		port:       port,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.done:
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logger.WS("connected", client.userID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			logger.WS("disconnected", client.userID)

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// Stop signals the Hub.Run goroutine to exit.
func (h *Hub) Stop() {
	select {
	case <-h.done:
	default:
		close(h.done)
	}
}

func (h *Hub) Broadcast(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error("Failed to marshal broadcast message: %v", err)
		return
	}
	h.broadcast <- data
}

// BroadcastToTopic sends a message only to clients subscribed to the given topic.
func (h *Hub) BroadcastToTopic(topic string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error("Failed to marshal topic broadcast: %v", err)
		return
	}
	h.mu.Lock()
	for client := range h.clients {
		client.topMu.Lock()
		subscribed := client.topics[topic]
		client.topMu.Unlock()
		if !subscribed {
			continue
		}
		select {
		case client.send <- data:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
	h.mu.Unlock()
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	// Authenticate via cookie or Authorization header (no query params)
	tokenStr := ""
	if cookie, err := r.Cookie("openpaw_token"); err == nil {
		tokenStr = cookie.Value
	}
	if tokenStr == "" {
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 {
				tokenStr = parts[1]
			}
		}
	}

	userID := ""
	if tokenStr != "" {
		claims, err := h.auth.ValidateToken(tokenStr)
		if err == nil {
			userID = claims.UserID
		}
	}
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Create upgrader with origin checking
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(req *http.Request) bool {
			origin := req.Header.Get("Origin")
			if origin == "" {
				return true // Allow non-browser clients
			}
			allowed := []string{
				fmt.Sprintf("http://localhost:%d", h.port),
				fmt.Sprintf("http://127.0.0.1:%d", h.port),
				fmt.Sprintf("https://localhost:%d", h.port),
				fmt.Sprintf("https://127.0.0.1:%d", h.port),
				"http://localhost:5173",
			}
			for _, a := range allowed {
				if origin == a {
					return true
				}
			}
			return false
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
		topics: make(map[string]bool),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// Handle subscribe/unsubscribe messages from client
		var msg struct {
			Type  string `json:"type"`
			Topic string `json:"topic"`
		}
		if json.Unmarshal(data, &msg) == nil {
			switch msg.Type {
			case "subscribe":
				if msg.Topic != "" {
					c.topMu.Lock()
					c.topics[msg.Topic] = true
					c.topMu.Unlock()
				}
			case "unsubscribe":
				if msg.Topic != "" {
					c.topMu.Lock()
					delete(c.topics, msg.Topic)
					c.topMu.Unlock()
				}
			}
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			break
		}
	}
}
