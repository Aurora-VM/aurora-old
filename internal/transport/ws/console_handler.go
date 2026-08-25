package ws

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	appConsole "github.com/aurora-vm/aurora/internal/app/console"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	transportHTTP "github.com/aurora-vm/aurora/internal/transport/http"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for API gateway
	},
}

type ClientControlMessage struct {
	Type string `json:"type"` // "resize" or "ping"
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// ConsoleHandler manages WebSocket connections for interactive PTY shell and VNC console.
type ConsoleHandler struct {
	consoleManager *appConsole.Manager
	tokenManager   identity.TokenManager
}

// NewConsoleHandler constructs a ConsoleHandler.
func NewConsoleHandler(consoleManager *appConsole.Manager, tokenManager identity.TokenManager) *ConsoleHandler {
	return &ConsoleHandler{
		consoleManager: consoleManager,
		tokenManager:   tokenManager,
	}
}

// RegisterRoutes registers WebSocket console endpoints on Chi router.
func (h *ConsoleHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/instances/{id}/console", func(r chi.Router) {
		r.Get("/exec", h.HandleExec)
		r.Get("/vnc", h.HandleVNC)
	})
}

// HandleExec handles interactive PTY bash/sh terminal sessions.
func (h *ConsoleHandler) HandleExec(w http.ResponseWriter, r *http.Request) {
	h.serveConsole(w, r, domainCompute.ConsoleTypeExec)
}

// HandleVNC handles graphical remote desktop / VNC framebuffer streams.
func (h *ConsoleHandler) HandleVNC(w http.ResponseWriter, r *http.Request) {
	h.serveConsole(w, r, domainCompute.ConsoleTypeVNC)
}

func (h *ConsoleHandler) serveConsole(w http.ResponseWriter, r *http.Request, sessionType domainCompute.ConsoleSessionType) {
	instanceID := chi.URLParam(r, "id")
	if instanceID == "" {
		http.Error(w, "instance ID is required", http.StatusBadRequest)
		return
	}

	// 1. Authenticate Subject (via Context, query param ?token=, or Authorization header)
	sub := transportHTTP.GetSubject(r.Context())
	if sub == nil {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}
		if tokenStr != "" && h.tokenManager != nil {
			parsedSub, err := h.tokenManager.ValidateAccessToken(tokenStr)
			if err == nil && parsedSub != nil {
				sub = parsedSub
			}
		}
	}

	if sub == nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	// 2. Parse Terminal Geometry & Command
	cols := 80
	rows := 24
	if cStr := r.URL.Query().Get("cols"); cStr != "" {
		if c, err := strconv.Atoi(cStr); err == nil && c > 0 {
			cols = c
		}
	}
	if rStr := r.URL.Query().Get("rows"); rStr != "" {
		if row, err := strconv.Atoi(rStr); err == nil && row > 0 {
			rows = row
		}
	}
	command := r.URL.Query().Get("cmd")
	if command == "" {
		command = "/bin/bash"
	}

	// 3. Start Session Pipe with Node
	pipe, err := h.consoleManager.StartSession(r.Context(), sub, instanceID, sessionType, command, cols, rows)
	if err != nil {
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			http.Error(w, "forbidden: cannot access instance console", http.StatusForbidden)
			return
		}
		if errors.Is(err, appConsole.ErrInstanceNotRunning) {
			http.Error(w, "cannot open console for stopped instance", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer h.consoleManager.CloseSession(pipe.SessionID)

	// 4. Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WARN] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// 5. Goroutine: Forward Inbound Node Messages -> WebSocket Client
	stopChan := make(chan struct{})
	go func() {
		defer close(stopChan)
		for {
			select {
			case <-pipe.CloseChan:
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session closed"))
				return
			case msg, ok := <-pipe.Inbound:
				if !ok {
					return
				}
				if msg.Type == aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA {
					if err := conn.WriteMessage(websocket.BinaryMessage, msg.Data); err != nil {
						return
					}
				} else if msg.Type == aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_CLOSE {
					_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, msg.CloseReason))
					return
				}
			}
		}
	}()

	// 6. Loop: Read WebSocket Client frames -> Forward to Node Agent
	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if msgType == websocket.TextMessage {
			// Check if control JSON (e.g. resize)
			var ctrl ClientControlMessage
			if err := json.Unmarshal(payload, &ctrl); err == nil && ctrl.Type == "resize" {
				if ctrl.Cols > 0 && ctrl.Rows > 0 {
					_ = h.consoleManager.ResizeTerminal(pipe.SessionID, ctrl.Cols, ctrl.Rows)
				}
				continue
			}
			// Regular text input
			_ = h.consoleManager.SendData(pipe.SessionID, payload)
		} else if msgType == websocket.BinaryMessage {
			_ = h.consoleManager.SendData(pipe.SessionID, payload)
		} else if msgType == websocket.PingMessage {
			_ = conn.WriteControl(websocket.PongMessage, payload, time.Now().Add(5*time.Second))
		}
	}
}
