package server

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

type Room struct {
	name       string
	clients    map[*Client]bool
	usernames  map[string]*Client
	broadcast  chan []byte
	Register   chan *Client
	unregister chan *Client
	lastActive time.Time
	mu         sync.Mutex
	hub        *Hub // Add reference to Hub
}

func NewRoom(name string, hub *Hub) *Room {
	room := &Room{
		name:       name,
		clients:    make(map[*Client]bool),
		usernames:  make(map[string]*Client),
		broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		unregister: make(chan *Client),
		lastActive: time.Now(),
		hub:        hub, // Set the Hub reference
	}
	go room.run()
	return room
}

func (r *Room) run() {
	for {
		select {
		case client := <-r.Register:
			r.handleRegister(client)

		case client := <-r.unregister:
			r.handleUnregister(client)

		case message := <-r.broadcast:
			r.handleBroadcast(message)
		}
	}
}

func (r *Room) handleRegister(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[client] = true
	r.lastActive = time.Now()
	log.Printf("Client registered in room %s", r.name)
}

func (r *Room) handleUnregister(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.clients[client]; ok {
		delete(r.clients, client)
		if client.username != "" {
			delete(r.usernames, client.username)
			leaveMsg := NewSystemMessage(client.username + " has left the chat.")
			b, _ := json.Marshal(leaveMsg)
			r.broadcast <- b
		}
		close(client.send)
	}
	r.lastActive = time.Now()
	log.Printf("Client unregistered from room %s", r.name)
}

func (r *Room) handleBroadcast(message []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for client := range r.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(r.clients, client)
		}
	}
	r.lastActive = time.Now()
}

func (r *Room) ClientCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.clients)
}

func (r *Room) Broadcast(message []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for client := range r.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(r.clients, client)
		}
	}
	r.lastActive = time.Now()
}
