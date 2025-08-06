package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/LamichhaneBibek/go-rtc/server"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		allowedOrigins := []string{"http://localhost:8080", "https://chat.lamichhanebibek.com.np"}
		origin := r.Header.Get("Origin")
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
	},
}

func serveWs(hub *server.Hub, w http.ResponseWriter, r *http.Request) {
	roomName := r.URL.Query().Get("room")
	if roomName == "" {
		http.Error(w, "Room name required", http.StatusBadRequest)
		log.Println("WebSocket connection attempt without room name")
		return
	}

	room, _ := hub.GetOrCreateRoom(roomName)
	if room.ClientCount() >= server.MaxClientsPerRoom {
		http.Error(w, "Room is full (max 3 users)", http.StatusForbidden)
		log.Printf("Room %s is full", roomName)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	client := server.NewClient(conn, room)
	room.Register <- client

	go client.WritePump()
	go client.ReadPump()

	log.Printf("New client connected to room %s", roomName)
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
	http.ServeFile(w, r, "client/index.html")
}

func main() {
	// Initialize logging
	log.SetOutput(os.Stdout)
	log.Println("Starting chat server...")

	hub := server.NewHub()

	// Create a file server that properly handles MIME types
	fs := http.FileServer(neuteredFileSystem{http.Dir("client")})

	// Serve static files with proper MIME types
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Explicit handlers for CSS and JS files
	http.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		http.ServeFile(w, r, filepath.Join("client", "style.css"))
	})

	http.HandleFunc("/chat.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, filepath.Join("client", "chat.js"))
	})

	// Routes
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/rooms", hub.ListRoomsHandler)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	port := ":8080"
	log.Printf("Server started on %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}

// neuteredFileSystem prevents directory listing
type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if s.IsDir() {
		index := filepath.Join(path, "index.html")
		if _, err := nfs.fs.Open(index); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return nil, closeErr
			}
			return nil, err
		}
	}

	return f, nil
}
