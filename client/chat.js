class ChatApp {
    constructor() {
        this.ws = null;
        this.username = "";
        this.roomName = "";
        this.typingTimeout = null;
        this.isTyping = false;

        this.elements = {
            chat: document.getElementById("chat"),
            messageInput: document.getElementById("message"),
            sendButton: document.getElementById("send"),
            nameModal: document.getElementById("nameModal"),
            nameInput: document.getElementById("nameInput"),
            roomInput: document.getElementById("roomInput"),
            roomSelect: document.getElementById("roomSelect"),
            setNameButton: document.getElementById("setName"),
            errorMsg: document.getElementById("errorMsg"),
            sidebar: document.getElementById("sidebar"),
            userProfile: document.getElementById("user-profile"),
            userAvatar: document.getElementById("user-avatar"),
            userName: document.getElementById("user-name"),
            currentRoom: document.getElementById("current-room"),
            roomList: document.getElementById("room-list")
        };

        this.initialize();
    }

    initialize() {
        this.setupEventListeners();
        this.fetchRooms();
        this.showModal();
    }

    switchRoom(newRoom) {
    // Clear current typing indicator
    document.querySelectorAll(".typing-indicator").forEach(el => el.remove());
    
    if (this.ws) {
        this.ws.close();
    }
    
    this.roomName = newRoom;
    this.elements.currentRoom.textContent = newRoom;
    this.clearChat();
    this.connectWebSocket(newRoom);
    this.highlightCurrentRoom(newRoom);
}

    setupEventListeners() {
        this.elements.sendButton.onclick = () => this.sendMessage();

        this.elements.messageInput.addEventListener("keyup", (e) => {
            if (e.key === "Enter") {
                this.sendMessage();
            } else {
                this.handleTyping();
            }
        });

        this.elements.setNameButton.onclick = () => this.handleUserJoin();

        // Add click event for room items
        this.elements.roomList.addEventListener("click", (e) => {
            const roomItem = e.target.closest(".room-item");
            if (roomItem) {
                const roomName = roomItem.dataset.room;
                if (roomName && roomName !== this.roomName) {
                    this.switchRoom(roomName);
                }
            }
        });
    }

    showModal() {
        this.elements.nameModal.style.display = "flex";
    }

    hideModal() {
        this.elements.nameModal.style.display = "none";
    }

    showError(message) {
        this.elements.errorMsg.textContent = message;
        setTimeout(() => {
            this.elements.errorMsg.textContent = "";
        }, 5000);
    }

    async fetchRooms() {
        try {
            const res = await fetch("/rooms");
            if (!res.ok) throw new Error("Failed to fetch rooms");
            const rooms = await res.json();
            this.updateRoomList(rooms);
        } catch (error) {
            console.error("Error fetching rooms:", error);
            this.showError("Failed to load available rooms. Please refresh the page.");
        }
    }

    updateRoomList(rooms) {
        this.elements.roomList.innerHTML = "";

        rooms.forEach(room => {
            const roomItem = document.createElement("div");
            roomItem.className = "room-item";
            roomItem.dataset.room = room;

            const roomIcon = document.createElement("div");
            roomIcon.className = "room-icon";
            roomIcon.textContent = room.charAt(0).toUpperCase();

            const roomName = document.createElement("div");
            roomName.className = "room-name";
            roomName.textContent = room;

            roomItem.appendChild(roomIcon);
            roomItem.appendChild(roomName);
            this.elements.roomList.appendChild(roomItem);
        });
    }

    handleUserJoin() {
        const name = this.elements.nameInput.value.trim();
        const selectedRoom = this.elements.roomSelect.value.trim();
        const customRoom = this.elements.roomInput.value.trim();
        const finalRoom = customRoom || selectedRoom;

        if (name === "") {
            this.showError("Please enter your name.");
            return;
        }

        if (finalRoom === "") {
            this.showError("Please select or enter a room.");
            return;
        }

        if (name.length > 20) {
            this.showError("Username must be less than 20 characters.");
            return;
        }

        if (finalRoom.length > 30) {
            this.showError("Room name must be less than 30 characters.");
            return;
        }

        this.username = name;
        this.roomName = finalRoom;

        // Update UI
        this.elements.userName.textContent = name;
        this.elements.userAvatar.textContent = name.charAt(0).toUpperCase();
        this.elements.currentRoom.textContent = finalRoom;

        this.connectWebSocket(finalRoom);
        this.hideModal();
    }

    connectWebSocket(room) {
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        const wsUrl = `${protocol}//${window.location.host}/ws?room=${encodeURIComponent(room)}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log("Connected to WebSocket.");
            const msg = { type: "setUsername", username: this.username };
            this.ws.send(JSON.stringify(msg));

            // Add current room to the list if it's new
            const roomExists = Array.from(this.elements.roomList.children).some(
                item => item.dataset.room === room
            );

            if (!roomExists) {
                this.fetchRooms(); // Refresh room list
            }

            // Highlight current room
            this.highlightCurrentRoom(room);
        };

        this.ws.onmessage = (evt) => {
            try {
                const messageData = JSON.parse(evt.data);

                switch (messageData.type) {
                    case "error":
                        this.showError(messageData.content);
                        break;
                    case "chat":
                        this.appendMessage(messageData);
                        break;
                    case "userList":
                        this.updateUserList(messageData.users);
                        break;
                    case "typing":
                        this.showTypingIndicator(messageData.username);
                        break;
                }
            } catch (e) {
                console.error("Invalid message format", evt.data, e);
            }
        };

        this.ws.onerror = (err) => {
            this.showError("WebSocket error. Try again later.");
            console.error("WebSocket error:", err);
        };

        this.ws.onclose = () => {
            this.showError("Connection closed. Please refresh the page.");
        };
    }

    highlightCurrentRoom(roomName) {
        Array.from(this.elements.roomList.children).forEach(item => {
            item.classList.toggle("active", item.dataset.room === roomName);
        });
        this.elements.currentRoom.textContent = roomName;
    }


    clearChat() {
        this.elements.chat.innerHTML = "";
    }

    handleTyping() {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return; // Don't try to send if not connected
        }

        if (!this.isTyping) {
            this.isTyping = true;
            const msg = { type: "typing", username: this.username };
            try {
                this.ws.send(JSON.stringify(msg));
            } catch (error) {
                console.error("Error sending typing indicator:", error);
            }
        }

        clearTimeout(this.typingTimeout);
        this.typingTimeout = setTimeout(() => {
            this.isTyping = false;
        }, 2000);
    }

    showTypingIndicator(username) {
        // Remove any existing typing indicators
        document.querySelectorAll(".typing-indicator").forEach(el => el.remove());

        if (username === this.username) return;

        const typingDiv = document.createElement("div");
        typingDiv.className = "typing-indicator";
        typingDiv.innerHTML = `
            <div class="typing-dot"></div>
            <div class="typing-dot"></div>
            <div class="typing-dot"></div>
            <span style="margin-left: 8px;">${username} is typing...</span>
        `;

        this.elements.chat.appendChild(typingDiv);
        this.elements.chat.scrollTop = this.elements.chat.scrollHeight;
    }

    appendMessage(messageData) {
        // Remove typing indicator if present
        document.querySelectorAll(".typing-indicator").forEach(el => el.remove());

        const msgDiv = document.createElement("div");
        msgDiv.className = `message ${messageData.username === this.username ? "sent" : "received"}`;

        const messageContent = document.createElement("div");
        messageContent.className = "message-content";

        if (messageData.username !== this.username) {
            const avatar = document.createElement("div");
            avatar.className = "message-avatar";
            avatar.textContent = messageData.username.charAt(0).toUpperCase();
            messageContent.appendChild(avatar);
        }

        const bubbleDiv = document.createElement("div");
        bubbleDiv.className = "message-bubble";
        bubbleDiv.textContent = messageData.content;

        const metaDiv = document.createElement("div");
        metaDiv.className = "message-meta";

        const usernameSpan = document.createElement("span");
        usernameSpan.className = "message-username";
        usernameSpan.textContent = messageData.username;

        const timeSpan = document.createElement("span");
        timeSpan.className = "message-time";
        timeSpan.textContent = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        metaDiv.appendChild(usernameSpan);
        metaDiv.appendChild(timeSpan);

        messageContent.appendChild(bubbleDiv);
        messageContent.appendChild(metaDiv);
        msgDiv.appendChild(messageContent);

        this.elements.chat.appendChild(msgDiv);
        this.elements.chat.scrollTop = this.elements.chat.scrollHeight;
    }

    sendMessage() {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            this.showError("Connection is not established.");
            return;
        }

        const text = this.elements.messageInput.value.trim();
        if (text === "") return;

        try {
            const msg = { type: "chat", content: text };
            this.ws.send(JSON.stringify(msg));
            this.elements.messageInput.value = "";
        } catch (error) {
            console.error("Error sending message:", error);
            this.showError("Failed to send message.");
        }
    }
}


// Initialize the chat application when the DOM is loaded
document.addEventListener("DOMContentLoaded", () => {
    new ChatApp();
});