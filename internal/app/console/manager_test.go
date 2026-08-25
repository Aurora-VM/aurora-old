package console

import (
	"context"
	"testing"
	"time"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	"github.com/aurora-vm/aurora/internal/app/authz"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsoleManager_Lifecycle_And_Resizing(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())

	// Create running instance
	inst := &domainCompute.Instance{
		ID:        "inst-test-console-01",
		NodeID:    "node-01",
		UserID:    "usr-admin",
		Name:      "test-instance-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, memStore.Instances().Create(ctx, inst))

	adminSub := &identity.Subject{
		UserID:      "usr-admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	var sentMsgs []*aurorav1.ServerMessage
	sendToNode := func(nodeID string, msg *aurorav1.ServerMessage) error {
		sentMsgs = append(sentMsgs, msg)
		return nil
	}

	mgr := NewManager(memStore.Instances(), memStore.Nodes(), authorizer, memStore.Audit(), sendToNode)

	// 1. Start Session
	pipe, err := mgr.StartSession(ctx, adminSub, inst.ID, domainCompute.ConsoleTypeExec, "/bin/bash", 80, 24)
	require.NoError(t, err)
	assert.NotEmpty(t, pipe.SessionID)
	assert.Equal(t, inst.ID, pipe.InstanceID)
	assert.Equal(t, inst.NodeID, pipe.NodeID)

	require.Len(t, sentMsgs, 1)
	startMsg := sentMsgs[0].GetConsoleSessionMessage()
	require.NotNil(t, startMsg)
	assert.Equal(t, aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_START, startMsg.Type)
	assert.Equal(t, int32(80), startMsg.Cols)
	assert.Equal(t, int32(24), startMsg.Rows)

	// 2. Client sends data -> Forwarded to Node
	err = mgr.SendData(pipe.SessionID, []byte("ls -la\n"))
	require.NoError(t, err)
	require.Len(t, sentMsgs, 2)
	dataMsg := sentMsgs[1].GetConsoleSessionMessage()
	require.NotNil(t, dataMsg)
	assert.Equal(t, aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA, dataMsg.Type)
	assert.Equal(t, []byte("ls -la\n"), dataMsg.Data)

	// 3. Client resizes window -> Forwarded to Node
	err = mgr.ResizeTerminal(pipe.SessionID, 120, 40)
	require.NoError(t, err)
	require.Len(t, sentMsgs, 3)
	resizeMsg := sentMsgs[2].GetConsoleSessionMessage()
	require.NotNil(t, resizeMsg)
	assert.Equal(t, aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_RESIZE, resizeMsg.Type)
	assert.Equal(t, int32(120), resizeMsg.Cols)
	assert.Equal(t, int32(40), resizeMsg.Rows)

	// 4. Inbound message from Node -> Sent into pipe.Inbound
	mgr.HandleNodeMessage(&aurorav1.ConsoleSessionMessage{
		SessionId:  pipe.SessionID,
		InstanceId: inst.ID,
		Type:       aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA,
		Data:       []byte("total 0\r\n"),
	})

	select {
	case in := <-pipe.Inbound:
		assert.Equal(t, []byte("total 0\r\n"), in.Data)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for inbound terminal message")
	}

	// 5. Close Session
	mgr.CloseSession(pipe.SessionID)
	require.Len(t, sentMsgs, 4)
	closeMsg := sentMsgs[3].GetConsoleSessionMessage()
	assert.Equal(t, aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_CLOSE, closeMsg.Type)
}
