package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MaxClientsPerRoom = 3
	roomInactiveTime  = 5 * time.Minute
	cleanupInterval   = 1 * time.Minute
)

type Hub struct {
	rooms map[string]*Room
	mu    sync.Mutex
}

func NewHub() *Hub {
	h := &Hub{
		rooms: make(map[string]*Room),
	}
	go h.cleanupRooms()
	return h
}

func (h *Hub) ListRoomsHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	rooms := make([]string, 0, len(h.rooms))
	for name := range h.rooms {
		rooms = append(rooms, name)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rooms); err != nil {
		http.Error(w, "Failed to encode rooms", http.StatusInternalServerError)
		log.Printf("Error encoding rooms: %v", err)
	}
}

func (h *Hub) cleanupRooms() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		for name, room := range h.rooms {
			room.mu.Lock()
			if time.Since(room.lastActive) > roomInactiveTime {
				for client := range room.clients {
					client.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","content":"Room closed due to inactivity."}`))
					client.conn.Close()
				}
				delete(h.rooms, name)
				log.Printf("Deleted inactive room: %s", name)
			}
			room.mu.Unlock()
		}
		h.mu.Unlock()
	}
}

func (h *Hub) GetOrCreateRoom(roomName string) (*Room, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.rooms[roomName]
	if !exists {
		room = NewRoom(roomName, h)
		h.rooms[roomName] = room
		log.Printf("Created new room: %s", roomName)
		go h.BroadcastRoomList() // Broadcast updated room list
	}

	return room, exists
}

// Add this method to Hub
func (h *Hub) BroadcastRoomList() {
	h.mu.Lock()
	defer h.mu.Unlock()

	rooms := make([]string, 0, len(h.rooms))
	for name := range h.rooms {
		rooms = append(rooms, name)
	}

	roomListMsg := NewRoomListMessage(rooms)
	b, _ := json.Marshal(roomListMsg)

	// Broadcast to all rooms
	for _, room := range h.rooms {
		room.Broadcast(b)
	}
}
