package auctions

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"encore.dev/beta/auth"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now - configure based on your needs
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// HandleWebSocket handles WebSocket connections - UPDATED IMPLEMENTATION
//
//encore:api public raw method=GET path=/auctions/:id/ws
func HandleWebSocket(w http.ResponseWriter, req *http.Request) {
	// Get auction ID from path parameter
	pathParts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	var auctionIDStr string
	for i, part := range pathParts {
		if part == "auctions" && i+1 < len(pathParts) {
			auctionIDStr = pathParts[i+1]
			break
		}
	}
	if auctionIDStr == "" {
		http.Error(w, "auction ID required", http.StatusBadRequest)
		return
	}

	auctionID, err := strconv.ParseInt(auctionIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid auction ID", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Get user ID from context (if authenticated)
	var userID *int64
	if uid, ok := auth.UserID(); ok {
		if uidInt64, err := strconv.ParseInt(string(uid), 10, 64); err == nil {
			userID = &uidInt64
		}
	}

	// Create WebSocket client
	client := &Client{
		ID:        generateClientID(),
		AuctionID: auctionID,
		UserID:    userID,
		WSConn:    conn,
		LastSeen:  time.Now().UTC(),
		IsWS:      true,
		Done:      make(chan bool),
	}

	// Get realtime service instance
	service := GetRealtimeService()
	if service == nil {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Service unavailable"))
		return
	}

	// Register client
	service.hub.register <- client

	// Send initial heartbeat
	initialEvent := &AuctionEvent{
		EventType: EventHeartbeat,
		Data:      map[string]interface{}{"timestamp": time.Now().UTC().Unix()},
	}
	service.sendWSEvent(client, initialEvent)

	// Handle incoming messages and connection lifecycle
	go handleWSClient(client, service)

	// Wait for client disconnect
	select {
	case <-req.Context().Done():
		service.hub.unregister <- client
	case <-client.Done:
		// Client was disconnected by server
	}
}

func handleWSClient(client *Client, service *RealtimeService) {
	conn := client.WSConn.(*websocket.Conn)
	
	// Set read deadline and pong handler for keepalive
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		client.LastSeen = time.Now().UTC()
		return nil
	})

	// Read pump - handle incoming messages
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		client.LastSeen = time.Now().UTC()

		// Handle different message types
		switch messageType {
		case websocket.TextMessage:
			// Parse and handle client messages if needed
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				handleClientMessage(client, msg, service)
			}
		case websocket.PingMessage:
			// Respond to ping with pong
			conn.WriteMessage(websocket.PongMessage, nil)
		}
	}

	// Unregister client when loop exits
	service.hub.unregister <- client
}

func handleClientMessage(client *Client, msg map[string]interface{}, service *RealtimeService) {
	// Handle client-side messages like:
	// - Subscription preferences
	// - Heartbeat responses
	// - Connection status updates
	
	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "ping":
		// Respond to client ping
		response := &AuctionEvent{
			EventType: EventHeartbeat,
			Data:      map[string]interface{}{"type": "pong", "timestamp": time.Now().UTC().Unix()},
		}
		service.sendWSEvent(client, response)
	case "subscribe":
		// Handle subscription updates if needed
		log.Printf("Client %s subscribed to additional events", client.ID)
	}
}

// sendWSEvent sends an event to a WebSocket client
func (s *RealtimeService) sendWSEvent(client *Client, event *AuctionEvent) {
	if !client.IsWS || client.WSConn == nil {
		return
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	conn := client.WSConn.(*websocket.Conn)
	
	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	
	// Send JSON message
	if err := conn.WriteJSON(event); err != nil {
		log.Printf("Error sending WebSocket message to client %s: %v", client.ID, err)
		// Mark client for disconnection
		select {
		case client.Done <- true:
		default:
		}
		return
	}

	client.LastSeen = time.Now().UTC()
}
