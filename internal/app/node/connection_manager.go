package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/google/uuid"
)

// ActiveConnection represents a live bidirectional gRPC stream tunnel to a node agent.
type ActiveConnection struct {
	NodeID      string
	ConnectedAt time.Time
	Sender      node.StreamSender
}

// MemoryConnectionManager manages active gRPC stream connections and command dispatching.
type MemoryConnectionManager struct {
	mu           sync.RWMutex
	connections  map[string]*ActiveConnection
	pendingCmds  map[string]chan *node.CommandResult // key: correlationID
}

// NewConnectionManager initializes an in-memory connection coordinator.
func NewConnectionManager() *MemoryConnectionManager {
	return &MemoryConnectionManager{
		connections: make(map[string]*ActiveConnection),
		pendingCmds: make(map[string]chan *node.CommandResult),
	}
}

// RegisterConnection records a newly established live stream.
func (m *MemoryConnectionManager) RegisterConnection(nodeID string, sender node.StreamSender) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connections[nodeID] = &ActiveConnection{
		NodeID:      nodeID,
		ConnectedAt: time.Now().UTC(),
		Sender:      sender,
	}
	return nil
}

// UnregisterConnection removes a terminated stream connection and fails any pending commands for that node.
func (m *MemoryConnectionManager) UnregisterConnection(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.connections, nodeID)
}

// GetConnection returns the active StreamSender for the specified node ID.
func (m *MemoryConnectionManager) GetConnection(nodeID string) (node.StreamSender, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[nodeID]
	if !exists {
		return nil, false
	}
	return conn.Sender, true
}

// ListConnectedNodeIDs returns the list of all currently connected active node IDs.
func (m *MemoryConnectionManager) ListConnectedNodeIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.connections))
	for id := range m.connections {
		ids = append(ids, id)
	}
	return ids
}

// DispatchCommand sends a typed command to the specified node and blocks until a matching response is received or context deadline fires.
func (m *MemoryConnectionManager) DispatchCommand(ctx context.Context, nodeID string, cmd *node.Command) (*node.CommandResult, error) {
	if cmd.CorrelationID == "" {
		cmd.CorrelationID = uuid.New().String()
	}
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = time.Now().UTC()
	}

	m.mu.Lock()
	conn, exists := m.connections[nodeID]
	if !exists {
		m.mu.Unlock()
		return nil, node.ErrNodeOffline
	}

	resChan := make(chan *node.CommandResult, 1)
	m.pendingCmds[cmd.CorrelationID] = resChan
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pendingCmds, cmd.CorrelationID)
		m.mu.Unlock()
	}()

	// Send command across stream
	if err := conn.Sender.Send(cmd); err != nil {
		return nil, fmt.Errorf("failed to transmit command to node: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resChan:
		if !res.Success {
			return res, fmt.Errorf("%w: %s", node.ErrCommandRejected, res.ErrorMessage)
		}
		return res, nil
	}
}

// HandleCommandResult correlates a received command response with its waiting dispatch caller.
func (m *MemoryConnectionManager) HandleCommandResult(result *node.CommandResult) {
	m.mu.RLock()
	ch, exists := m.pendingCmds[result.CorrelationID]
	m.mu.RUnlock()

	if exists {
		select {
		case ch <- result:
		default:
		}
	}
}
