package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type     string `json:"type"`     // "setUsername", "chat", "error"
	Username string `json:"username"` // username of sender
	Content  string `json:"content"`  // chat content or error message
}

type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
	room     *Room
}

type Room struct {
	name       string
	clients    map[*Client]bool
	usernames  map[string]*Client
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	lastActive time.Time
	mu         sync.Mutex
}

type Hub struct {
	rooms map[string]*Room
	mu    sync.Mutex
}

func newHub() *Hub {
	h := &Hub{
		rooms: make(map[string]*Room),
	}
	go h.cleanupRooms()
	return h
}

func newRoom(name string) *Room {
	room := &Room{
		name:       name,
		clients:    make(map[*Client]bool),
		usernames:  make(map[string]*Client),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		lastActive: time.Now(),
	}
	go room.run()
	return room
}

func (h *Hub) listRoomsHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	rooms := make([]string, 0, len(h.rooms))
	for name := range h.rooms {
		rooms = append(rooms, name)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

func (r *Room) run() {
	for {
		select {
		case client := <-r.register:
			r.mu.Lock()
			r.clients[client] = true
			r.lastActive = time.Now()
			r.mu.Unlock()

		case client := <-r.unregister:
			r.mu.Lock()
			if _, ok := r.clients[client]; ok {
				delete(r.clients, client)
				if client.username != "" {
					delete(r.usernames, client.username)
					leaveMsg := Message{
						Type:     "chat",
						Username: "System",
						Content:  client.username + " has left the chat.",
					}
					b, _ := json.Marshal(leaveMsg)
					r.broadcast <- b
				}
				close(client.send)
			}
			r.lastActive = time.Now()
			r.mu.Unlock()

		case message := <-r.broadcast:
			r.mu.Lock()
			for client := range r.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(r.clients, client)
				}
			}
			r.lastActive = time.Now()
			r.mu.Unlock()
		}
	}
}

func (h *Hub) cleanupRooms() {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		<-ticker.C
		h.mu.Lock()
		for name, room := range h.rooms {
			room.mu.Lock()
			if time.Since(room.lastActive) > 5*time.Minute {
				for client := range room.clients {
					client.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","content":"Room closed due to inactivity."}`))
					client.conn.Close()
				}
				delete(h.rooms, name)
				log.Println("Deleted inactive room:", name)
			}
			room.mu.Unlock()
		}
		h.mu.Unlock()
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	roomName := r.URL.Query().Get("room")
	if roomName == "" {
		http.Error(w, "Room name required", http.StatusBadRequest)
		return
	}

	hub.mu.Lock()
	room, exists := hub.rooms[roomName]
	if !exists {
		room = newRoom(roomName)
		hub.rooms[roomName] = room
	}
	room.mu.Lock()
	if len(room.clients) >= 3 {
		room.mu.Unlock()
		hub.mu.Unlock()
		http.Error(w, "Room is full (max 3 users)", http.StatusForbidden)
		return
	}
	room.mu.Unlock()
	hub.mu.Unlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		room: room,
	}
	room.register <- client

	go client.writePump()
	go client.readPump()
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

func (c *Client) readPump() {
	defer func() {
		c.room.unregister <- c
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
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}

		switch m.Type {
		case "setUsername":
			if m.Username == "" {
				continue
			}
			c.room.mu.Lock()
			if _, exists := c.room.usernames[m.Username]; exists {
				errMsg := Message{
					Type:    "error",
					Content: "Username already taken, please choose another one.",
				}
				b, _ := json.Marshal(errMsg)
				c.conn.WriteMessage(websocket.TextMessage, b)
			} else {
				c.username = m.Username
				c.room.usernames[m.Username] = c
				joinMsg := Message{
					Type:     "chat",
					Username: "System",
					Content:  m.Username + " has joined the chat.",
				}
				b, _ := json.Marshal(joinMsg)
				c.room.broadcast <- b
			}
			c.room.mu.Unlock()

		case "chat":
			if c.username == "" {
				continue
			}
			chatMsg := Message{
				Type:     "chat",
				Username: c.username,
				Content:  m.Content,
			}
			b, _ := json.Marshal(chatMsg)
			c.room.broadcast <- b
		}
	}
}

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

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/rooms", hub.listRoomsHandler)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	log.Println("Server started on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
