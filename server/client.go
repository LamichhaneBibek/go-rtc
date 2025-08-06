package server

import (
	"encoding/json"
	"log"
	"regexp"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
	room     *Room
}

func NewClient(conn *websocket.Conn, room *Room) *Client {
	return &Client{
		conn: conn,
		send: make(chan []byte, 256),
		room: room,
	}
}

func (c *Client) ReadPump() {
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var m Message
		if err := json.Unmarshal(msg, &m); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		switch m.Type {
		case "setUsername":
			c.handleSetUsername(m.Username)
		case "chat":
			c.handleChatMessage(m.Content)
		case "getRoomList":
			c.handleGetRoomList()
		case "typing":
			c.handleTypingNotification(m.Username)
		default:
			log.Printf("Unknown message type: %s", m.Type)
		}
	}
}

func (c *Client) handleTypingNotification(username string) {
	if username == "" {
		return
	}

	typingMsg := Message{
		Type:     "typing",
		Username: username,
	}
	b, _ := json.Marshal(typingMsg)
	c.room.broadcast <- b
}

func (c *Client) handleSetUsername(username string) {
	if username == "" || len(username) > 20 || !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(username) {
		errMsg := NewErrorMessage("Invalid username. Use 1-20 alphanumeric characters or underscores.")
		b, _ := json.Marshal(errMsg)
		c.conn.WriteMessage(websocket.TextMessage, b)
		return
	}

	c.room.mu.Lock()
	defer c.room.mu.Unlock()

	if _, exists := c.room.usernames[username]; exists {
		errMsg := NewErrorMessage("Username already taken, please choose another one.")
		b, _ := json.Marshal(errMsg)
		c.conn.WriteMessage(websocket.TextMessage, b)
	} else {
		c.username = username
		c.room.usernames[username] = c
		joinMsg := NewSystemMessage(username + " has joined the chat.")
		b, _ := json.Marshal(joinMsg)
		c.room.broadcast <- b
	}
}

func (c *Client) handleChatMessage(content string) {
	if c.username == "" {
		return
	}

	chatMsg := NewChatMessage(c.username, content)
	b, _ := json.Marshal(chatMsg)
	c.room.broadcast <- b
}

func (c *Client) handleGetRoomList() {
	if c.room.hub != nil {
		c.room.hub.BroadcastRoomList()
	}
}

// Enhance WritePump with better error handling
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		// Ensure connection is closed
		c.conn.WriteMessage(websocket.CloseMessage, []byte{})
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// Channel closed
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Printf("Write error: %v", err)
				return
			}

			// Drain remaining messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				err := c.conn.WriteMessage(websocket.TextMessage, <-c.send)
				if err != nil {
					log.Printf("Write error: %v", err)
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Ping error: %v", err)
				return
			}
		}
	}
}
