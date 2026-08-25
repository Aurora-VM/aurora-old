package console

import (
	"context"
	"errors"
	"sync"
	"time"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/google/uuid"
)

var (
	ErrSessionNotFound   = errors.New("console session not found")
	ErrInstanceNotRunning = errors.New("cannot attach console to instance that is not running")
)

// SessionPipe buffers bidirectional terminal messages between a WebSocket client and hypervisor node.
type SessionPipe struct {
	SessionID   string
	NodeID      string
	InstanceID  string
	UserID      string
	SessionType domainCompute.ConsoleSessionType
	Inbound     chan *aurorav1.ConsoleSessionMessage // From Node to WebSocket client
	Outbound    chan *aurorav1.ConsoleSessionMessage // From WebSocket client to Node
	CloseChan   chan struct{}
	once        sync.Once
}

func NewSessionPipe(sessionID, nodeID, instanceID, userID string, sessionType domainCompute.ConsoleSessionType) *SessionPipe {
	return &SessionPipe{
		SessionID:   sessionID,
		NodeID:      nodeID,
		InstanceID:  instanceID,
		UserID:      userID,
		SessionType: sessionType,
		Inbound:     make(chan *aurorav1.ConsoleSessionMessage, 128),
		Outbound:    make(chan *aurorav1.ConsoleSessionMessage, 128),
		CloseChan:   make(chan struct{}),
	}
}

func (p *SessionPipe) Close() {
	p.once.Do(func() {
		close(p.CloseChan)
	})
}

// Manager manages active interactive terminal and VNC sessions on the Control Plane Hub.
type Manager struct {
	mu         sync.RWMutex
	sessions   map[string]*SessionPipe
	instRepo   domainCompute.InstanceRepository
	nodeRepo   domainNode.NodeRepository
	authorizer identity.Authorizer
	auditRepo  audit.Repository
	sendToNode func(nodeID string, msg *aurorav1.ServerMessage) error
}

func NewManager(
	instRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
	sendToNode func(nodeID string, msg *aurorav1.ServerMessage) error,
) *Manager {
	return &Manager{
		sessions:   make(map[string]*SessionPipe),
		instRepo:   instRepo,
		nodeRepo:   nodeRepo,
		authorizer: authorizer,
		auditRepo:  auditRepo,
		sendToNode: sendToNode,
	}
}

// StartSession validates permissions, creates a session pipe, and requests node agent to launch console.
func (m *Manager) StartSession(
	ctx context.Context,
	sub *identity.Subject,
	instanceID string,
	sessionType domainCompute.ConsoleSessionType,
	command string,
	cols, rows int,
) (*SessionPipe, error) {
	inst, err := m.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := m.authorizer.Authorize(ctx, sub, "instance:console", inst.Resource()); err != nil {
		return nil, err
	}

	if inst.Status != domainCompute.StatusRunning {
		return nil, ErrInstanceNotRunning
	}

	if command == "" {
		command = "/bin/bash"
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	sessionID := uuid.New().String()
	pipe := NewSessionPipe(sessionID, inst.NodeID, inst.ID, sub.UserID, sessionType)

	m.mu.Lock()
	m.sessions[sessionID] = pipe
	m.mu.Unlock()

	// Send START message across mTLS to node agent
	if m.sendToNode != nil {
		srvMsg := &aurorav1.ServerMessage{
			CorrelationId: sessionID,
			Payload: &aurorav1.ServerMessage_ConsoleSessionMessage{
				ConsoleSessionMessage: &aurorav1.ConsoleSessionMessage{
					SessionId:    sessionID,
					InstanceId:   inst.ID,
					InstanceName: inst.Name,
					Type:         aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_START,
					Command:      command,
					Cols:         int32(cols),
					Rows:         int32(rows),
					Env: map[string]string{
						"TERM": "xterm-256color",
					},
				},
			},
		}
		if err := m.sendToNode(inst.NodeID, srvMsg); err != nil {
			m.CloseSession(sessionID)
			return nil, err
		}
	}

	m.logAudit(ctx, sub, "instance:console:open", inst.ID, map[string]interface{}{
		"sessionId": sessionID,
		"type":      string(sessionType),
		"command":   command,
	})

	return pipe, nil
}

// HandleNodeMessage routes an inbound console message from a node to the active session pipe.
func (m *Manager) HandleNodeMessage(msg *aurorav1.ConsoleSessionMessage) {
	if msg == nil {
		return
	}
	m.mu.RLock()
	pipe, ok := m.sessions[msg.SessionId]
	m.mu.RUnlock()

	if !ok || pipe == nil {
		return
	}

	select {
	case <-pipe.CloseChan:
	case pipe.Inbound <- msg:
	default:
	}
}

// SendData forwards terminal input bytes from WebSocket client to the node agent.
func (m *Manager) SendData(sessionID string, data []byte) error {
	m.mu.RLock()
	pipe, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok || pipe == nil {
		return ErrSessionNotFound
	}

	if m.sendToNode != nil {
		srvMsg := &aurorav1.ServerMessage{
			CorrelationId: sessionID,
			Payload: &aurorav1.ServerMessage_ConsoleSessionMessage{
				ConsoleSessionMessage: &aurorav1.ConsoleSessionMessage{
					SessionId:  sessionID,
					InstanceId: pipe.InstanceID,
					Type:       aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA,
					Data:       data,
				},
			},
		}
		return m.sendToNode(pipe.NodeID, srvMsg)
	}
	return nil
}

// ResizeTerminal notifies node agent of window geometry changes.
func (m *Manager) ResizeTerminal(sessionID string, cols, rows int) error {
	m.mu.RLock()
	pipe, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok || pipe == nil {
		return ErrSessionNotFound
	}

	if m.sendToNode != nil {
		srvMsg := &aurorav1.ServerMessage{
			CorrelationId: sessionID,
			Payload: &aurorav1.ServerMessage_ConsoleSessionMessage{
				ConsoleSessionMessage: &aurorav1.ConsoleSessionMessage{
					SessionId:  sessionID,
					InstanceId: pipe.InstanceID,
					Type:       aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_RESIZE,
					Cols:       int32(cols),
					Rows:       int32(rows),
				},
			},
		}
		return m.sendToNode(pipe.NodeID, srvMsg)
	}
	return nil
}

// CloseSession terminates an active session and informs the node agent.
func (m *Manager) CloseSession(sessionID string) {
	m.mu.Lock()
	pipe, ok := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	if ok && pipe != nil {
		pipe.Close()
		if m.sendToNode != nil {
			srvMsg := &aurorav1.ServerMessage{
				CorrelationId: sessionID,
				Payload: &aurorav1.ServerMessage_ConsoleSessionMessage{
					ConsoleSessionMessage: &aurorav1.ConsoleSessionMessage{
						SessionId:   sessionID,
						InstanceId:  pipe.InstanceID,
						Type:        aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_CLOSE,
						CloseReason: "client_disconnected",
					},
				},
			}
			_ = m.sendToNode(pipe.NodeID, srvMsg)
		}
	}
}

func (m *Manager) logAudit(ctx context.Context, sub *identity.Subject, action, resourceID string, details map[string]interface{}) {
	if m.auditRepo == nil {
		return
	}
	var actorID *string
	if sub != nil && sub.UserID != "" {
		actorID = &sub.UserID
	}
	var rID *string
	if resourceID != "" {
		rID = &resourceID
	}
	_ = m.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:    actorID,
		Action:     action,
		ResourceID: rID,
		Details:    details,
		CreatedAt:  time.Now().UTC(),
	})
}
