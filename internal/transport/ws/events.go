package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	appEvents "github.com/aurora-vm/aurora/internal/app/events"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/gorilla/websocket"
)

// EventStreamHandler manages tenant-scoped real-time WebSocket event feeds.
type EventStreamHandler struct {
	eventBus     *appEvents.EventBus
	tokenManager identity.TokenManager
	upgrader     websocket.Upgrader
}

func NewEventStreamHandler(eventBus *appEvents.EventBus, tokenManager identity.TokenManager) *EventStreamHandler {
	return &EventStreamHandler{
		eventBus:     eventBus,
		tokenManager: tokenManager,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *EventStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Sec-WebSocket-Protocol")
	}
	if token == "" {
		http.Error(w, "missing authentication token", http.StatusUnauthorized)
		return
	}

	claims, err := h.tokenManager.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ERROR] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Tenant or Superadmin
	tenantID := claims.UserID
	isAdmin := false
	for _, p := range claims.Permissions {
		if p == "*" || p == "node:read" {
			isAdmin = true
			break
		}
	}

	eventChan := make(chan *domainEvents.Event, 64)

	// Subscribe to event bus
	subID := h.eventBus.SubscribeTenant(tenantID, "*", func(ctx context.Context, event *domainEvents.Event) error {
		select {
		case eventChan <- event:
		default:
		}
		return nil
	})
	defer h.eventBus.Unsubscribe(subID)

	// Also subscribe to global broadcast if admin
	var adminSubID string
	if isAdmin {
		adminSubID = h.eventBus.Subscribe("*", func(ctx context.Context, event *domainEvents.Event) error {
			if event.TenantID != tenantID {
				select {
				case eventChan <- event:
				default:
				}
			}
			return nil
		})
		defer h.eventBus.Unsubscribe(adminSubID)
	}

	// Ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Client read loop to detect disconnection
	closeChan := make(chan struct{})
	go func() {
		defer close(closeChan)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-closeChan:
			return
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case evt := <-eventChan:
			msgBytes, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
				return
			}
		}
	}
}
