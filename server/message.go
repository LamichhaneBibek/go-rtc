package server

type Message struct {
	Type     string   `json:"type"` // "setUsername", "chat", "error", "roomList", "typing", "roomAdded"
	Username string   `json:"username"`
	Content  string   `json:"content"`
	Users    []string `json:"users"`
}

func NewErrorMessage(content string) Message {
	return Message{
		Type:    "error",
		Content: content,
	}
}

func NewSystemMessage(content string) Message {
	return Message{
		Type:     "chat",
		Username: "System",
		Content:  content,
	}
}

func NewChatMessage(username, content string) Message {
	return Message{
		Type:     "chat",
		Username: username,
		Content:  content,
	}
}

func NewRoomListMessage(rooms []string) Message {
	return Message{
		Type:  "roomList",
		Users: rooms,
	}
}

// Add new constructor
func NewTypingMessage(username string) Message {
	return Message{
		Type:     "typing",
		Username: username,
	}
}

func NewRoomAddedMessage(roomName string) Message {
	return Message{
		Type:    "roomAdded",
		Content: roomName,
	}
}
