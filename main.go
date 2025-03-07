package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Message defines the JSON message format exchanged between client and server.
type Message struct {
	Type     string `json:"type"`     // "setUsername", "chat", "error"
	Username string `json:"username"` // used for setUsername and chat messages
	Content  string `json:"content"`  // chat message content or error details
}

// Client represents a single chatting user.
type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
}

// Hub maintains the set of active clients and maps usernames.
type Hub struct {
	clients   map[*Client]bool
	usernames map[string]*Client
	broadcast chan []byte
	register  chan *Client
	unregister chan *Client
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		usernames:  make(map[string]*Client),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// run listens for register, unregister, and broadcast events.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.username != "" {
					delete(h.usernames, client.username)
				}
				close(client.send)
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
					if client.username != "" {
						delete(h.usernames, client.username)
					}
				}
			}
		}
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections for simplicity.
		return true
	},
}

// serveWs upgrades the HTTP connection to a WebSocket and registers the client.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	client := &Client{conn: conn, send: make(chan []byte, 256)}
	hub.register <- client
	go client.writePump()
	go client.readPump(hub)
}

// Timeouts and limits.
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var m Message
		err = json.Unmarshal(msg, &m)
		if err != nil {
			continue
		}

		switch m.Type {
		case "setUsername":
			if m.Username == "" {
				continue
			}
			// Check for duplicate username.
			if _, exists := hub.usernames[m.Username]; exists {
				errMsg := Message{
					Type:    "error",
					Content: "Username already taken, please choose another one.",
				}
				b, _ := json.Marshal(errMsg)
				c.conn.WriteMessage(websocket.TextMessage, b)
			} else {
				c.username = m.Username
				hub.usernames[m.Username] = c
				// Broadcast join message.
				joinMsg := Message{
					Type:     "chat",
					Username: "System",
					Content:  m.Username + " has joined the chat.",
				}
				b, _ := json.Marshal(joinMsg)
				hub.broadcast <- b
			}

		case "chat":
			// Ignore chat messages if username is not set.
			if c.username == "" {
				continue
			}
			chatMsg := Message{
				Type:     "chat",
				Username: c.username,
				Content:  m.Content,
			}
			b, _ := json.Marshal(chatMsg)
			hub.broadcast <- b
		}
	}
}

// writePump writes messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			// Write any queued messages.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveHome serves the chat UI.
func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "index.html")
}

func main() {
	hub := newHub()
	go hub.run()

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	log.Println("Server started on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
