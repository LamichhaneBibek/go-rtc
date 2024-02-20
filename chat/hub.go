package chat

import (
	"bytes"
	"html/template"
	"log"
)

type Hub struct {
	clients    map[*Client]bool
	messages   []*Message
	broadcast  chan *Message
	register   chan *Client
	unregister chan *Client
}

type Message struct {
	ClientID string
	Text     string
}

type WSMessage struct {
	Text    string      `json:"text"`
	Headers interface{} `json:"HEADERS"`
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan *Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("Client %s connected", client.id)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				close(client.send)
				delete(h.clients, client)
			}
		case msg := <-h.broadcast:
			log.Printf("Message received: %s", msg.Text)
			h.messages = append(h.messages, msg)
			for client := range h.clients {
				select {
				case client.send <- getMessageTemplate(msg):
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func getMessageTemplate(msg *Message) []byte {
	tmpl, err := template.ParseFiles("templates/message.html")
	if err != nil {
		log.Println(err)
	}

	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, msg)
	if err != nil {
		log.Fatalf("template execution: %s", err)
	}
	return tpl.Bytes()
}
