package chat

import (
	"sync"
)

type BroadcastMessage struct {
	ConversationID string      `json:"conversation_id"`
	Message        interface{} `json:"message"`
	SenderID       string      `json:"sender_id"`
	// We need to know who to send this to.
	// The Service layer should likely pass the list of recipient IDs here.
	RecipientIDs []string `json:"recipient_ids"`
}

type Hub struct {
	// Global Map: UserID -> *Client (Tracks all online users)
	Clients map[string]*Client

	// Room Map: ConversationID -> Map[UserID] -> true (Tracks active room subscriptions)
	// Optional: Only needed if you want to broadcast only to people "viewing" the chat
	Rooms map[string]map[string]bool

	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan BroadcastMessage
	Mutex      sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[string]*Client),
		Rooms:      make(map[string]map[string]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan BroadcastMessage),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			// 1. REGISTER CLIENT
			h.Mutex.Lock()
			h.Clients[client.UserID] = client
			h.Mutex.Unlock()

		case client := <-h.Unregister:
			// 2. UNREGISTER CLIENT
			h.Mutex.Lock()
			if _, ok := h.Clients[client.UserID]; ok {
				delete(h.Clients, client.UserID)
				close(client.Send) // Close the channel to stop the write pump
			}
			h.Mutex.Unlock()

		case msg := <-h.Broadcast:
			// 3. BROADCAST MESSAGE
			h.Mutex.RLock()

			// Strategy: Iterate over the intended recipients (passed from Service layer)
			// and send the message if they are currently connected.
			for _, recipientID := range msg.RecipientIDs {
				if client, ok := h.Clients[recipientID]; ok {
					select {
					case client.Send <- msg:
					default:
						// If client's buffer is full, close connection (prevent blocking hub)
						close(client.Send)
						delete(h.Clients, recipientID)
					}
				}
			}
			h.Mutex.RUnlock()
		}
	}
}
