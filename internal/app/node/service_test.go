package node

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStreamSender struct {
	sentCommands []*node.Command
}

func (m *mockStreamSender) Send(cmd *node.Command) error {
	m.sentCommands = append(m.sentCommands, cmd)
	return nil
}

func setupTestNodeService(t *testing.T) (*Service, *memory.MemoryStore, *pki.InternalCA) {
	memStore := memory.NewMemoryStore()
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)
	connMgr := NewConnectionManager()

	svc := NewService(
		memStore.Nodes(),
		memStore.Enrollments(),
		ca,
		connMgr,
		memStore.Audit(),
		"127.0.0.1:8443",
	)

	return svc, memStore, ca
}

func generateTestCSR(t *testing.T, commonName, fqdn string) []byte {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Project Aurora Node"},
			CommonName:   commonName,
		},
		DNSNames: []string{fqdn},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
}

func TestNodeService_Enrollment_FullFlow(t *testing.T) {
	svc, _, ca := setupTestNodeService(t)
	ctx := context.Background()

	// 1. Create Enrollment Token
	adminID := "usr_superadmin_123"
	token, secret, err := svc.CreateEnrollmentToken(ctx, "loc_us_east", "*.us-east.local", 1*time.Hour, &adminID)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.NotEmpty(t, secret.ID)

	// 2. Generate local CSR
	csrPEM := generateTestCSR(t, "hv-01.us-east.local", "hv-01.us-east.local")

	// 3. Enroll Node using valid token
	nodeID, certPEM, caCertPEM, interval, err := svc.EnrollNode(ctx, token, "hv-01", "hv-01.us-east.local", csrPEM, map[string]interface{}{"incus": true})
	require.NoError(t, err)
	assert.NotEmpty(t, nodeID)
	assert.NotEmpty(t, certPEM)
	assert.NotEmpty(t, caCertPEM)
	assert.Equal(t, 10, interval)

	// Verify certificate
	verifiedID, fp, err := ca.VerifyCertificate(certPEM)
	require.NoError(t, err)
	assert.Equal(t, nodeID, verifiedID)
	assert.NotEmpty(t, fp)

	// 4. Attempt to RE-USE the same one-time enrollment token -> must fail!
	_, _, _, _, err = svc.EnrollNode(ctx, token, "hv-02", "hv-02.us-east.local", csrPEM, nil)
	assert.ErrorIs(t, err, node.ErrEnrollmentTokenUsed)

	// 5. Attempt enrollment with non-existent token -> must fail!
	_, _, _, _, err = svc.EnrollNode(ctx, "aur_enroll_fake_token_12345", "hv-03", "hv-03.local", csrPEM, nil)
	assert.ErrorIs(t, err, node.ErrEnrollmentTokenInvalid)
}

func TestNodeService_Enrollment_ExpiredToken(t *testing.T) {
	svc, _, _ := setupTestNodeService(t)
	ctx := context.Background()

	// Create already expired token (-1 minute)
	token, _, err := svc.CreateEnrollmentToken(ctx, "loc_1", "", -1*time.Minute, nil)
	require.NoError(t, err)

	csrPEM := generateTestCSR(t, "node-1", "node-1.local")
	_, _, _, _, err = svc.EnrollNode(ctx, token, "node-1", "node-1.local", csrPEM, nil)
	assert.ErrorIs(t, err, node.ErrEnrollmentTokenExpired)
}

func TestNodeService_StreamLifecycle_Heartbeat_And_Commands(t *testing.T) {
	svc, _, _ := setupTestNodeService(t)
	ctx := context.Background()

	// 1. Enroll Node
	token, _, _ := svc.CreateEnrollmentToken(ctx, "loc_1", "", 1*time.Hour, nil)
	csrPEM := generateTestCSR(t, "hv-stream-test", "hv-stream-test.local")
	nodeID, certPEM, _, _, err := svc.EnrollNode(ctx, token, "hv-stream-test", "hv-stream-test.local", csrPEM, nil)
	require.NoError(t, err)

	// 2. Authenticate Node Certificate
	nodeRecord, err := svc.AuthenticateNodeCertificate(ctx, certPEM)
	require.NoError(t, err)
	assert.Equal(t, nodeID, nodeRecord.ID)

	// 3. Connect Stream
	sender := &mockStreamSender{}
	err = svc.OnStreamConnected(ctx, nodeID, sender)
	require.NoError(t, err)

	n, err := svc.GetNode(ctx, nodeID)
	require.NoError(t, err)
	assert.Equal(t, node.StatusOnline, n.Status)

	// 4. Process Heartbeat
	err = svc.ProcessHeartbeat(ctx, nodeID, map[string]interface{}{"cpu_cores": 16})
	require.NoError(t, err)

	nUpdated, _ := svc.GetNode(ctx, nodeID)
	assert.NotNil(t, nUpdated.LastHeartbeatAt)
	assert.Equal(t, 16, nUpdated.Capabilities["cpu_cores"])

	// 5. Dispatch Command (Async with response handler)
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := &node.Command{
		CorrelationID: "cmd-correlation-1",
		Type:          "ping",
		Payload:       map[string]interface{}{"msg": "hello"},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Simulate agent replying with CommandResult
		svc.HandleCommandResult(&node.CommandResult{
			CorrelationID: "cmd-correlation-1",
			Success:       true,
			Payload:       map[string]interface{}{"pong": true},
			CompletedAt:   time.Now().UTC(),
		})
	}()

	res, err := svc.SendCommand(cmdCtx, nodeID, cmd)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, true, res.Payload["pong"])

	// 6. Maintenance mode
	err = svc.ToggleMaintenance(ctx, nodeID, true)
	require.NoError(t, err)
	nMaint, _ := svc.GetNode(ctx, nodeID)
	assert.Equal(t, node.StatusMaintenance, nMaint.Status)
	assert.True(t, nMaint.MaintenanceMode)

	// 7. Revoke Node -> Disconnects and rejects further commands
	err = svc.RevokeNode(ctx, nodeID)
	require.NoError(t, err)

	nRevoked, _ := svc.GetNode(ctx, nodeID)
	assert.Equal(t, node.StatusRevoked, nRevoked.Status)

	// Connecting stream from revoked node should fail
	err = svc.OnStreamConnected(ctx, nodeID, sender)
	assert.ErrorIs(t, err, node.ErrNodeRevoked)

	// Certificate auth for revoked node should fail
	_, err = svc.AuthenticateNodeCertificate(ctx, certPEM)
	assert.ErrorIs(t, err, node.ErrNodeRevoked)
}
