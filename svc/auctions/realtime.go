package auctions

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"encore.dev/beta/auth"

	"encore.dev/storage/sqldb"
)

// EventType represents the type of auction event
type EventType string

const (
	EventBidPlaced       EventType = "bid_placed"
	EventOutbid          EventType = "outbid"
	EventExtended        EventType = "extended"
	EventEnded           EventType = "ended"
	EventBidRemoved      EventType = "bid_removed"
	EventPriceRecomputed EventType = "price_recomputed"
	EventHeartbeat       EventType = "heartbeat"
)

// AuctionEvent represents a real-time auction event
type AuctionEvent struct {
	EventType EventType   `json:"event"`
	Data      interface{} `json:"data"`
}

// Client represents a connected client
type Client struct {
	ID        string
	AuctionID int64
	UserID    *int64 // nil for anonymous
	SSEWriter http.ResponseWriter
	WSConn    interface{} // WebSocket connection (to be implemented)
	LastSeen  time.Time
	IsSSE     bool
	IsWS      bool
	Done      chan bool
	mu        sync.Mutex
}

// Hub manages all client connections and broadcasts
type Hub struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan *AuctionEvent
	mu         sync.RWMutex
	db         *sqldb.Database
}

// RealtimeService handles real-time auction events
type RealtimeService struct {
	hub *Hub
	db  *sqldb.Database
	mu  sync.RWMutex // Protects service state
}

// NewRealtimeService creates a new realtime service
func NewRealtimeService(db *sqldb.Database) *RealtimeService {
	hub := &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *AuctionEvent),
		db:         db,
	}

	service := &RealtimeService{
		hub: hub,
		db:  db,
	}

	// Start the hub
	go hub.run()

	// Start heartbeat
	go service.startHeartbeat()

	return service
}

// HandleSSE handles Server-Sent Events connections
//
//encore:api public raw method=GET path=/auctions/:id/events
func HandleSSE(w http.ResponseWriter, req *http.Request) {
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

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Get user ID from context (if authenticated)
	var userID *int64
	if uid, ok := auth.UserID(); ok {
		if uidInt64, err := strconv.ParseInt(string(uid), 10, 64); err == nil {
			userID = &uidInt64
		}
	}

	// Create client
	client := &Client{
		ID:        generateClientID(),
		AuctionID: auctionID,
		UserID:    userID,
		SSEWriter: w,
		LastSeen:  time.Now().UTC(),
		IsSSE:     true,
		Done:      make(chan bool),
	}

	// Get realtime service instance - ensure it's initialized
	service := GetRealtimeService()
	if service == nil {
		http.Error(w, "realtime service not available", http.StatusServiceUnavailable)
		return
	}

	// Register client
	service.hub.register <- client

	// Keep connection alive
	_, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial heartbeat
	service.sendSSEEvent(client, &AuctionEvent{
		EventType: EventHeartbeat,
		Data:      map[string]interface{}{"timestamp": time.Now().UTC().Unix()},
	})

	// Wait for client disconnect
	select {
	case <-req.Context().Done():
		service.hub.unregister <- client
	case <-client.Done:
		// Client was disconnected by server
	}
}

// NOTE: HandleWebSocket is now implemented in websocket_implementation.go

// BroadcastBidPlaced broadcasts a bid placed event
func (s *RealtimeService) BroadcastBidPlaced(ctx context.Context, auctionID int64, bid *BidWithDetails, currentPrice float64) error {
	event := &AuctionEvent{
		EventType: EventBidPlaced,
		Data: map[string]interface{}{
			"auction_id":    auctionID,
			"bid_id":        bid.ID,
			"amount":        bid.Amount,
			"current_price": currentPrice,
			"bidder_name":   bid.BidderNameSnapshot,
			"bidder_city":   bid.BidderCityName,
			"timestamp":     time.Now().UTC().Unix(),
		},
	}

	return s.broadcastToAuction(auctionID, event)
}

// BroadcastOutbid broadcasts outbid notifications to specific users
func (s *RealtimeService) BroadcastOutbid(ctx context.Context, auctionID int64, outbidUserIDs []int64, currentPrice float64) error {
	event := &AuctionEvent{
		EventType: EventOutbid,
		Data: map[string]interface{}{
			"auction_id":    auctionID,
			"current_price": currentPrice,
			"timestamp":     time.Now().UTC().Unix(),
		},
	}

	return s.broadcastToUsers(auctionID, outbidUserIDs, event)
}

// BroadcastExtended broadcasts auction extension event
func (s *RealtimeService) BroadcastExtended(ctx context.Context, auctionID int64, oldEndAt, newEndAt time.Time, extensionsCount int) error {
	event := &AuctionEvent{
		EventType: EventExtended,
		Data: map[string]interface{}{
			"auction_id":       auctionID,
			"old_end_at":       oldEndAt.Unix(),
			"new_end_at":       newEndAt.Unix(),
			"extensions_count": extensionsCount,
			"timestamp":        time.Now().UTC().Unix(),
		},
	}

	return s.broadcastToAuction(auctionID, event)
}

// BroadcastEnded broadcasts auction ended event
func (s *RealtimeService) BroadcastEnded(ctx context.Context, auctionID int64, result *AuctionEndResult) error {
	eventData := map[string]interface{}{
		"auction_id": auctionID,
		"outcome":    result.Outcome,
		"message":    result.Message,
		"timestamp":  result.EndedAt.Unix(),
	}

	if result.WinnerBid != nil {
		eventData["winner_bid"] = map[string]interface{}{
			"bid_id":      result.WinnerBid.ID,
			"amount":      result.WinnerBid.Amount,
			"bidder_name": result.WinnerBid.BidderNameSnapshot,
			"bidder_city": result.WinnerBid.BidderCityName,
		}
	}

	if result.ReservePrice != nil {
		eventData["reserve_price"] = *result.ReservePrice
	}

	event := &AuctionEvent{
		EventType: EventEnded,
		Data:      eventData,
	}

	return s.broadcastToAuction(auctionID, event)
}

// BroadcastBidRemoved broadcasts bid removal event
func (s *RealtimeService) BroadcastBidRemoved(ctx context.Context, auctionID int64, removedBidID int64, reason string, removedBy string) error {
	event := &AuctionEvent{
		EventType: EventBidRemoved,
		Data: map[string]interface{}{
			"auction_id":     auctionID,
			"removed_bid_id": removedBidID,
			"reason":         reason,
			"removed_by":     removedBy,
			"timestamp":      time.Now().UTC().Unix(),
		},
	}

	return s.broadcastToAuction(auctionID, event)
}

// BroadcastPriceRecomputed broadcasts price recomputation event
func (s *RealtimeService) BroadcastPriceRecomputed(ctx context.Context, auctionID int64, newCurrentPrice float64, extensionsCount int, reason string) error {
	event := &AuctionEvent{
		EventType: EventPriceRecomputed,
		Data: map[string]interface{}{
			"auction_id":       auctionID,
			"current_price":    newCurrentPrice,
			"extensions_count": extensionsCount,
			"reason":           reason,
			"timestamp":        time.Now().UTC().Unix(),
		},
	}

	return s.broadcastToAuction(auctionID, event)
}

// GetActiveConnections returns the number of active connections for an auction
func (s *RealtimeService) GetActiveConnections(auctionID int64) int {
	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()

	count := 0
	for _, client := range s.hub.clients {
		if client.AuctionID == auctionID {
			count++
		}
	}
	return count
}

// Private methods

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()
			log.Printf("Client %s connected to auction %d", client.ID, client.AuctionID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Done)
			}
			h.mu.Unlock()
			log.Printf("Client %s disconnected from auction %d", client.ID, client.AuctionID)

		case event := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case <-client.Done:
					// Client is already disconnected
				default:
					// Get service instance using the hub's reference
					service := &RealtimeService{hub: h, db: h.db}
					if client.IsSSE {
						service.sendSSEEvent(client, event)
					} else if client.IsWS {
						service.sendWSEvent(client, event)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (s *RealtimeService) broadcastToAuction(auctionID int64, event *AuctionEvent) error {
	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()

	for _, client := range s.hub.clients {
		if client.AuctionID == auctionID {
			if client.IsSSE {
				s.sendSSEEvent(client, event)
			} else if client.IsWS {
				s.sendWSEvent(client, event)
			}
		}
	}

	return nil
}

func (s *RealtimeService) broadcastToUsers(auctionID int64, userIDs []int64, event *AuctionEvent) error {
	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()

	userIDMap := make(map[int64]bool)
	for _, userID := range userIDs {
		userIDMap[userID] = true
	}

	for _, client := range s.hub.clients {
		if client.AuctionID == auctionID && client.UserID != nil && userIDMap[*client.UserID] {
			if client.IsSSE {
				s.sendSSEEvent(client, event)
			} else if client.IsWS {
				s.sendWSEvent(client, event)
			}
		}
	}

	return nil
}

func (s *RealtimeService) sendSSEEvent(client *Client, event *AuctionEvent) {
	client.mu.Lock()
	defer client.mu.Unlock()

	data, err := json.Marshal(event.Data)
	if err != nil {
		log.Printf("Error marshaling event data: %v", err)
		return
	}

	// Send SSE formatted message
	fmt.Fprintf(client.SSEWriter, "event: %s\n", event.EventType)
	fmt.Fprintf(client.SSEWriter, "data: %s\n\n", data)

	if flusher, ok := client.SSEWriter.(http.Flusher); ok {
		flusher.Flush()
	}

	client.LastSeen = time.Now().UTC()
}

// sendWSEvent is implemented in websocket_implementation.go

func (s *RealtimeService) startHeartbeat() {
    // Align with FRD: heartbeat every 30 seconds
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sendHeartbeat()
		}
	}
}

func (s *RealtimeService) sendHeartbeat() {
	event := &AuctionEvent{
		EventType: EventHeartbeat,
		Data:      map[string]interface{}{"timestamp": time.Now().UTC().Unix()},
	}

	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()

	for _, client := range s.hub.clients {
		// Check if client is stale (no activity for 30 seconds)
		if time.Since(client.LastSeen) > 30*time.Second {
			// Disconnect stale client
			go func(c *Client) {
				s.hub.unregister <- c
			}(client)
			continue
		}

		if client.IsSSE {
			s.sendSSEEvent(client, event)
		} else if client.IsWS {
			s.sendWSEvent(client, event)
		}
	}
}

func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UTC().UnixNano())
}

// Global service instance - managed by service initialization
var globalRealtimeService *RealtimeService
var realtimeServiceMutex sync.RWMutex

// InitRealtimeService initializes the global realtime service with proper error handling
func InitRealtimeService(db *sqldb.Database) {
	realtimeServiceMutex.Lock()
	defer realtimeServiceMutex.Unlock()
	
	if db == nil {
		log.Printf("Warning: Cannot initialize RealtimeService with nil database")
		return
	}
	
	globalRealtimeService = NewRealtimeService(db)
	log.Printf("RealtimeService initialized successfully")
}

// GetRealtimeService returns the global realtime service instance with safety checks
func GetRealtimeService() *RealtimeService {
	realtimeServiceMutex.RLock()
	defer realtimeServiceMutex.RUnlock()
	
	if globalRealtimeService == nil {
		log.Printf("Warning: RealtimeService not initialized - call InitRealtimeService during startup")
		return nil
	}
	return globalRealtimeService
}

// GetRealtimeServiceOrPanic returns the service instance or panics (for critical paths)
func GetRealtimeServiceOrPanic() *RealtimeService {
	service := GetRealtimeService()
	if service == nil {
		panic("RealtimeService not initialized - ensure InitRealtimeService is called during startup")
	}
	return service
}
