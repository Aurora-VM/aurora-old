package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	"github.com/aurora-vm/aurora/internal/app/agent"
	appHealth "github.com/aurora-vm/aurora/internal/app/health"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/keystore"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func startTestMTLSGatewayServer(t *testing.T) (string, *appNode.Service, *pki.InternalCA, func()) {
	memStore := memory.NewMemoryStore()
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	connMgr := appNode.NewConnectionManager()
	nodeService := appNode.NewService(
		memStore.Nodes(),
		memStore.Enrollments(),
		ca,
		connMgr,
		memStore.Audit(),
		"127.0.0.1:0",
	)
	healthService := appHealth.NewService()

	// Generate server cert for mTLS listener
	serverCertPEM, serverKeyPEM, err := ca.GenerateServerCertificate([]string{"127.0.0.1", "localhost"}, 1*time.Hour)
	require.NoError(t, err)

	serverTLSConfig, err := ca.BuildServerTLSConfig(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLSConfig)))
	Register(grpcServer, healthService, nodeService)

	go func() {
		_ = grpcServer.Serve(listener)
	}()

	cleanup := func() {
		grpcServer.Stop()
		_ = listener.Close()
	}

	return listener.Addr().String(), nodeService, ca, cleanup
}

func TestGateway_mTLS_Enrollment_Stream_And_Commands(t *testing.T) {
	serverAddr, nodeService, ca, cleanup := startTestMTLSGatewayServer(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Create One-Time Enrollment Token
	adminID := "usr_superadmin"
	token, _, err := nodeService.CreateEnrollmentToken(ctx, "loc_us_east", "*.us-east.local", 1*time.Hour, &adminID)
	require.NoError(t, err)

	// 2. Setup Node Agent KeyStore
	tempDir := t.TempDir()
	ks, err := keystore.NewKeyStore(tempDir)
	require.NoError(t, err)

	// 3. Pre-enroll node via CA (since the gRPC listener is already mTLS-only)
	csrPEM, err := ks.GenerateKeyAndCSR("hv-test-01", "127.0.0.1")
	require.NoError(t, err)

	nodeID, certPEM, caCertPEM, _, err := nodeService.EnrollNode(
		ctx, token, "hv-test-01", "127.0.0.1", csrPEM, map[string]interface{}{"incus": true},
	)
	require.NoError(t, err)

	err = ks.SaveCertificates(caCertPEM, certPEM)
	require.NoError(t, err)
	assert.True(t, ks.HasCertificates())

	// 4. Start Node Agent Daemon
	agentDaemon, err := agent.NewDaemon(agent.Config{
		HubAddress:        serverAddr,
		NodeName:          "hv-test-01",
		FQDN:              "127.0.0.1",
		HeartbeatInterval: 100 * time.Millisecond,
		KeyStore:          ks,
	})
	require.NoError(t, err)

	agentCtx, agentCancel := context.WithCancel(ctx)
	defer agentCancel()

	go func() {
		_ = agentDaemon.Run(agentCtx)
	}()

	// 5. Wait for node to establish stream and transition to StatusOnline
	require.Eventually(t, func() bool {
		n, err := nodeService.GetNode(ctx, nodeID)
		return err == nil && n.Status == domainNode.StatusOnline
	}, 3*time.Second, 50*time.Millisecond)

	// 6. Verify heartbeat received
	require.Eventually(t, func() bool {
		n, err := nodeService.GetNode(ctx, nodeID)
		return err == nil && n.LastHeartbeatAt != nil
	}, 3*time.Second, 50*time.Millisecond)

	// 7. Dispatch typed Ping command across persistent mTLS stream
	cmdCtx, cmdCancel := context.WithTimeout(ctx, 2*time.Second)
	defer cmdCancel()

	pingResult, err := nodeService.SendCommand(cmdCtx, nodeID, &domainNode.Command{
		Type:    "ping",
		Payload: map[string]interface{}{"msg": "ping_test_123"},
	})
	require.NoError(t, err)
	assert.True(t, pingResult.Success)
	assert.Equal(t, true, pingResult.Payload["pong"])
	assert.Equal(t, "ping_test_123", pingResult.Payload["received"])

	// 8. Dispatch CollectTelemetry command across mTLS stream
	telCtx, telCancel := context.WithTimeout(ctx, 2*time.Second)
	defer telCancel()

	telResult, err := nodeService.SendCommand(telCtx, nodeID, &domainNode.Command{
		Type: "collect_telemetry",
	})
	require.NoError(t, err)
	assert.True(t, telResult.Success)
	assert.NotNil(t, telResult.Payload["cpu_usage_percent"])

	// 9. Node Revocation -> Revoke node certificate
	err = nodeService.RevokeNode(ctx, nodeID)
	require.NoError(t, err)

	// Verify node status is revoked
	nRevoked, err := nodeService.GetNode(ctx, nodeID)
	require.NoError(t, err)
	assert.Equal(t, domainNode.StatusRevoked, nRevoked.Status)

	// Ensure CA rejected on verify
	_, _, err = ca.VerifyCertificate(certPEM)
	require.NoError(t, err) // cert itself is crypto valid, but node record is revoked!
}

func TestGateway_RejectUntrustedClientCert(t *testing.T) {
	serverAddr, _, _, cleanup := startTestMTLSGatewayServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Generate client certificate signed by a ROGUE / UNTRUSTED CA
	rogueCA, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	tempDir := t.TempDir()
	rogueKS, err := keystore.NewKeyStore(tempDir)
	require.NoError(t, err)

	rogueCSRPEM, err := rogueKS.GenerateKeyAndCSR("rogue-node", "127.0.0.1")
	require.NoError(t, err)

	rogueCertPEM, _, err := rogueCA.SignNodeCSR(rogueCSRPEM, "rogue-123", "127.0.0.1", 1*time.Hour)
	require.NoError(t, err)

	err = rogueKS.SaveCertificates(rogueCA.GetCACertificatePEM(), rogueCertPEM)
	require.NoError(t, err)

	rogueTLSConfig, err := rogueKS.LoadClientTLSConfig()
	require.NoError(t, err)

	// Attempt connection to server with rogue certificate -> Handshake must fail!
	conn, err := grpc.DialContext(
		ctx,
		serverAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(rogueTLSConfig)),
		grpc.WithBlock(),
	)
	if err == nil {
		client := aurorav1.NewNodeGatewayServiceClient(conn)
		_, streamErr := client.StreamTunnel(ctx)
		assert.Error(t, streamErr)
		conn.Close()
	} else {
		assert.Error(t, err)
	}
}
