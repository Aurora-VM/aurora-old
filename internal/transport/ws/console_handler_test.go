package ws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	appAuth "github.com/aurora-vm/aurora/internal/app/auth"
	appAuthz "github.com/aurora-vm/aurora/internal/app/authz"
	appConsole "github.com/aurora-vm/aurora/internal/app/console"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	transportHTTP "github.com/aurora-vm/aurora/internal/transport/http"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocket_ConsoleExec_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector("test-master-key-32-characters-long!")
	require.NoError(t, err)
	tokenMgr, err := jwt.NewTokenManager("test-jwt-secret-key-32-characters-long!")
	require.NoError(t, err)
	totpMgr := totp.NewTOTPManager()

	authService := appAuth.NewService(memStore.Users(), memStore.Roles(), memStore.Sessions(), hasher, protector, tokenMgr, totpMgr, memStore.Audit())
	authorizer := appAuthz.NewAuthorizer(memStore.Roles())

	adminUser, err := authService.Register(ctx, appAuth.RegisterRequest{
		Username: "admin",
		Email:    "admin@aurora.local",
		Password: "Password12345!",
	})
	require.NoError(t, err)

	adminToken, err := tokenMgr.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})
	require.NoError(t, err)

	// Create running instance
	inst := &domainCompute.Instance{
		ID:        "inst-ws-01",
		NodeID:    "node-ws-01",
		UserID:    adminUser.ID,
		Name:      "alpine-ws-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, memStore.Instances().Create(ctx, inst))

	var consoleManager *appConsole.Manager

	sendToNode := func(nodeID string, msg *aurorav1.ServerMessage) error {
		cMsg := msg.GetConsoleSessionMessage()
		if cMsg != nil {
			if cMsg.Type == aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_START {
				// Simulate node responding with banner
				go func() {
					time.Sleep(10 * time.Millisecond)
					consoleManager.HandleNodeMessage(&aurorav1.ConsoleSessionMessage{
						SessionId:  cMsg.SessionId,
						InstanceId: cMsg.InstanceId,
						Type:       aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA,
						Data:       []byte("root@alpine-ws-01:~# "),
					})
				}()
			} else if cMsg.Type == aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA {
				// Echo response
				go func() {
					consoleManager.HandleNodeMessage(&aurorav1.ConsoleSessionMessage{
						SessionId:  cMsg.SessionId,
						InstanceId: cMsg.InstanceId,
						Type:       aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA,
						Data:       append([]byte("output: "), cMsg.Data...),
					})
				}()
			}
		}
		return nil
	}

	consoleManager = appConsole.NewManager(memStore.Instances(), memStore.Nodes(), authorizer, memStore.Audit(), sendToNode)

	router := transportHTTP.NewRouter()
	consoleHandler := NewConsoleHandler(consoleManager, tokenMgr)
	consoleHandler.RegisterRoutes(router)

	srv := httptest.NewServer(router)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/instances/inst-ws-01/console/exec?token=" + adminToken

	// 1. Dial WebSocket
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, 101, resp.StatusCode)

	// 2. Read banner from simulated node
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "root@alpine-ws-01:~# ", string(msg))

	// 3. Send command over WebSocket
	err = conn.WriteMessage(websocket.TextMessage, []byte("uname -a\n"))
	require.NoError(t, err)

	// 4. Read echo output
	_, outMsg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "output: uname -a\n", string(outMsg))

	// 5. Send resize control message
	resizeJSON := []byte(`{"type":"resize","cols":120,"rows":40}`)
	err = conn.WriteMessage(websocket.TextMessage, resizeJSON)
	require.NoError(t, err)

	// 6. Close WebSocket normally
	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
	require.NoError(t, err)
}
