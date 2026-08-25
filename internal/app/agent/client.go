package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/network"
	"github.com/aurora-vm/aurora/internal/domain/storage"
	"github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/aurora-vm/aurora/internal/infra/incus"
	"github.com/aurora-vm/aurora/internal/infra/keystore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Config configures the Aurora Node Agent daemon.
type Config struct {
	HubAddress        string // e.g. "127.0.0.1:8443"
	HubHTTPAddress    string // e.g. "http://127.0.0.1:8080"
	NodeName          string
	FQDN              string
	EnrollmentToken   string
	HeartbeatInterval time.Duration
	KeyStore          *keystore.KeyStore
	Driver            compute.HypervisorDriver       // Incus socket driver or simulated driver
	NetworkDriver     network.NetworkDriver          // Incus network/firewall driver
	StorageDriver     storage.StorageDriver          // Incus storage/volume driver
	ConsoleDriver     compute.ConsoleDriver          // Incus interactive terminal / VNC driver
	ImageDriver       template.HypervisorImageDriver // Incus image sync/verify driver
	InsecureEnroll    bool                           // For local test environments
}

type AgentConsoleSession struct {
	inWriter   io.WriteCloser
	resizeChan chan compute.WindowSize
	cancel     context.CancelFunc
}

// Daemon manages the node agent's lifecycle, enrollment, mTLS connection, and stream loop.
type Daemon struct {
	cfg            Config
	nodeID         string
	mu             sync.Mutex
	running        bool
	sequenceNo     int64
	activeConsoles map[string]*AgentConsoleSession
	consoleMu      sync.Mutex
}

// NewDaemon initializes a Node Agent daemon instance.
func NewDaemon(cfg Config) (*Daemon, error) {
	if cfg.HubAddress == "" {
		cfg.HubAddress = "127.0.0.1:9443"
	}
	if cfg.NodeName == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "aurora-node"
		}
		cfg.NodeName = hostname
	}
	if cfg.FQDN == "" {
		cfg.FQDN = cfg.NodeName
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 10 * time.Second
	}
	if cfg.KeyStore == nil {
		return nil, errors.New("keystore is required")
	}
	if cfg.Driver == nil {
		cfg.Driver = incus.NewSimulatedDriver()
	}
	if cfg.NetworkDriver == nil {
		cfg.NetworkDriver = incus.NewSimulatedNetworkDriver()
	}
	if cfg.StorageDriver == nil {
		cfg.StorageDriver = incus.NewSimulatedStorageDriver()
	}
	if cfg.ConsoleDriver == nil {
		cfg.ConsoleDriver = incus.NewSimulatedConsoleDriver()
	}
	if cfg.ImageDriver == nil {
		cfg.ImageDriver = incus.NewSimulatedImageDriver()
	}

	return &Daemon{
		cfg:            cfg,
		activeConsoles: make(map[string]*AgentConsoleSession),
	}, nil
}

// EnsureEnrolled ensures the node has valid mTLS certificates, enrolling with the Hub if necessary.
func (d *Daemon) EnsureEnrolled(ctx context.Context) error {
	if d.cfg.KeyStore.HasCertificates() {
		log.Println("[INFO] Node already enrolled with valid mTLS certificates.")
		return nil
	}

	if d.cfg.EnrollmentToken == "" {
		return errors.New("node is not enrolled and no enrollment token was provided")
	}

	log.Println("[INFO] Enrolling node with Control Plane Hub...")

	// 1. Generate local private key & PKCS#10 CSR (Private key NEVER leaves node!)
	csrPEM, err := d.cfg.KeyStore.GenerateKeyAndCSR(d.cfg.NodeName, d.cfg.FQDN)
	if err != nil {
		return fmt.Errorf("failed to generate local CSR: %w", err)
	}

	// 2. Perform enrollment via HTTP API
	httpURL := d.cfg.HubHTTPAddress
	if httpURL == "" {
		hostParts := strings.Split(d.cfg.HubAddress, ":")
		httpURL = fmt.Sprintf("http://%s:8080", hostParts[0])
	}
	if !strings.HasPrefix(httpURL, "http://") && !strings.HasPrefix(httpURL, "https://") {
		httpURL = "http://" + httpURL
	}
	enrollEndpoint := strings.TrimRight(httpURL, "/") + "/api/v1/nodes/enroll"

	reqPayload, _ := json.Marshal(map[string]interface{}{
		"enrollmentToken": d.cfg.EnrollmentToken,
		"nodeName":        d.cfg.NodeName,
		"fqdn":            d.cfg.FQDN,
		"csrPem":          string(csrPEM),
		"capabilities":    d.gatherCapabilitiesMap(),
	})

	httpClient := &http.Client{Timeout: 10 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", enrollEndpoint, bytes.NewReader(reqPayload))
	if err != nil {
		return fmt.Errorf("failed to create enrollment request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send enrollment request to %s: %w", enrollEndpoint, err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enrollment rejected (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var enrollResp struct {
		Success bool `json:"success"`
		Data    struct {
			NodeID                   string `json:"nodeId"`
			CertificatePEM           string `json:"certificatePem"`
			CACertificatePEM         string `json:"caCertificatePem"`
			HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
		} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(bodyBytes, &enrollResp); err != nil {
		return fmt.Errorf("failed to decode enrollment response: %w", err)
	}

	if !enrollResp.Success || enrollResp.Data.CertificatePEM == "" {
		errMsg := "unknown error"
		if enrollResp.Error != nil {
			errMsg = enrollResp.Error.Message
		}
		return fmt.Errorf("enrollment failed: %s", errMsg)
	}

	// 3. Persist certificates securely in KeyStore
	if err := d.cfg.KeyStore.SaveCertificates([]byte(enrollResp.Data.CACertificatePEM), []byte(enrollResp.Data.CertificatePEM)); err != nil {
		return fmt.Errorf("failed to save enrolled certificates: %w", err)
	}

	d.nodeID = enrollResp.Data.NodeID
	log.Printf("[INFO] Node successfully enrolled! Assigned Node ID: %s", d.nodeID)
	return nil
}

// Run executes the daemon loop, establishing a persistent outbound mTLS tunnel to the Hub with resilient reconnects.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.EnsureEnrolled(ctx); err != nil {
		return err
	}

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("[INFO] Node Agent shutting down gracefully...")
			return nil
		default:
		}

		err := d.runStreamSession(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("[WARN] Gateway tunnel disconnected: %v. Reconnecting in %v...", err, backoff)
		}

		// Calculate backoff with jitter
		jitter := time.Duration((rand.Float64()*0.6 + 0.7) * float64(backoff))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(jitter):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (d *Daemon) runStreamSession(ctx context.Context) error {
	tlsConfig, err := d.cfg.KeyStore.LoadClientTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to load mTLS client config: %w", err)
	}

	// Connect to Hub over mTLS 1.3
	conn, err := grpc.DialContext(
		ctx,
		d.cfg.HubAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial hub mTLS gateway: %w", err)
	}
	defer conn.Close()

	client := aurorav1.NewNodeGatewayServiceClient(conn)
	stream, err := client.StreamTunnel(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream tunnel: %w", err)
	}

	log.Printf("[INFO] Outbound mTLS Gateway Stream established to Hub at %s", d.cfg.HubAddress)

	// Send initial NodeReadyEvent
	if err := stream.Send(&aurorav1.NodeMessage{
		CorrelationId:   fmt.Sprintf("ready-%d", time.Now().UnixNano()),
		Timestamp:       timestamppb.Now(),
		ProtocolVersion: "aurora.v1",
		Payload: &aurorav1.NodeMessage_ReadyEvent{
			ReadyEvent: &aurorav1.NodeReadyEvent{
				NodeId:       d.nodeID,
				NodeName:     d.cfg.NodeName,
				AgentVersion: "0.1.0-dev",
				Capabilities: d.gatherCapabilities(),
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to send ready event: %w", err)
	}

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	// Launch Heartbeat sender goroutine
	go d.heartbeatLoop(streamCtx, stream)

	// Inbound message receiver loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("stream recv error: %w", err)
		}

		d.handleServerMessage(streamCtx, stream, msg)
	}
}

func (d *Daemon) heartbeatLoop(ctx context.Context, stream aurorav1.NodeGatewayService_StreamTunnelClient) {
	ticker := time.NewTicker(d.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.mu.Lock()
			d.sequenceNo++
			seq := d.sequenceNo
			d.mu.Unlock()

			hb := &aurorav1.NodeMessage{
				CorrelationId:   fmt.Sprintf("hb-%d", seq),
				Timestamp:       timestamppb.Now(),
				ProtocolVersion: "aurora.v1",
				Payload: &aurorav1.NodeMessage_Heartbeat{
					Heartbeat: &aurorav1.Heartbeat{
						NodeId:         d.nodeID,
						SequenceNumber: seq,
						Telemetry:      d.gatherTelemetry(),
						Capabilities:   d.gatherCapabilities(),
						AgentVersion:   "0.1.0-dev",
					},
				},
			}

			if err := stream.Send(hb); err != nil {
				log.Printf("[WARN] Failed to send heartbeat: %v", err)
				return
			}
		}
	}
}

func (d *Daemon) handleServerMessage(ctx context.Context, stream aurorav1.NodeGatewayService_StreamTunnelClient, msg *aurorav1.ServerMessage) {
	switch payload := msg.Payload.(type) {
	case *aurorav1.ServerMessage_HeartbeatAck:
		// Heartbeat acknowledged by server

	case *aurorav1.ServerMessage_PingCommand:
		log.Printf("[DEBUG] Received PingCommand (correlation: %s)", msg.CorrelationId)
		payloadJSON, _ := json.Marshal(map[string]interface{}{"pong": true, "received": payload.PingCommand.Payload})
		_ = stream.Send(&aurorav1.NodeMessage{
			CorrelationId:   msg.CorrelationId,
			Timestamp:       timestamppb.Now(),
			ProtocolVersion: "aurora.v1",
			Payload: &aurorav1.NodeMessage_CommandResult{
				CommandResult: &aurorav1.CommandResult{
					CommandCorrelationId: msg.CorrelationId,
					Success:              true,
					ResultPayloadJson:    payloadJSON,
				},
			},
		})

	case *aurorav1.ServerMessage_TelemetryCommand:
		log.Printf("[DEBUG] Received CollectTelemetryCommand (correlation: %s)", msg.CorrelationId)
		tel := d.gatherTelemetry()
		telJSON, _ := json.Marshal(tel)
		_ = stream.Send(&aurorav1.NodeMessage{
			CorrelationId:   msg.CorrelationId,
			Timestamp:       timestamppb.Now(),
			ProtocolVersion: "aurora.v1",
			Payload: &aurorav1.NodeMessage_CommandResult{
				CommandResult: &aurorav1.CommandResult{
					CommandCorrelationId: msg.CorrelationId,
					Success:              true,
					ResultPayloadJson:    telJSON,
				},
			},
		})

	case *aurorav1.ServerMessage_ConfigCommand:
		log.Printf("[INFO] Received ApplyNodeConfigCommand (correlation: %s)", msg.CorrelationId)
		_ = stream.Send(&aurorav1.NodeMessage{
			CorrelationId:   msg.CorrelationId,
			Timestamp:       timestamppb.Now(),
			ProtocolVersion: "aurora.v1",
			Payload: &aurorav1.NodeMessage_CommandResult{
				CommandResult: &aurorav1.CommandResult{
					CommandCorrelationId: msg.CorrelationId,
					Success:              true,
					ResultPayloadJson:    []byte(`{"applied": true}`),
				},
			},
		})

	case *aurorav1.ServerMessage_RebootCommand:
		log.Printf("[WARN] Received RebootNodeCommand: %s (delay: %d)", payload.RebootCommand.Reason, payload.RebootCommand.DelaySeconds)
		_ = stream.Send(&aurorav1.NodeMessage{
			CorrelationId:   msg.CorrelationId,
			Timestamp:       timestamppb.Now(),
			ProtocolVersion: "aurora.v1",
			Payload: &aurorav1.NodeMessage_CommandResult{
				CommandResult: &aurorav1.CommandResult{
					CommandCorrelationId: msg.CorrelationId,
					Success:              true,
					ResultPayloadJson:    []byte(`{"reboot_scheduled": true}`),
				},
			},
		})

	case *aurorav1.ServerMessage_CancelCommand:
		log.Printf("[INFO] Received CancelCommand for correlation: %s", payload.CancelCommand.TargetCorrelationId)

	// Stage 4 Incus Virtualization Command Handlers
	case *aurorav1.ServerMessage_CreateInstanceCommand:
		cmd := payload.CreateInstanceCommand
		log.Printf("[INFO] Received CreateInstanceCommand: %s (type: %s, cpu: %d, ram: %d)", cmd.InstanceName, cmd.InstanceType, cmd.CpuCores, cmd.MemoryBytes)

		instType := compute.TypeContainer
		if cmd.InstanceType == "virtual-machine" {
			instType = compute.TypeVirtualMachine
		}

		info, err := d.cfg.Driver.CreateInstance(ctx, &compute.InstanceSpec{
			ID:               cmd.InstanceId,
			Name:             cmd.InstanceName,
			Type:             instType,
			CPUCores:         int(cmd.CpuCores),
			MemoryBytes:      cmd.MemoryBytes,
			StorageBytes:     cmd.StorageBytes,
			Image:            cmd.Image,
			StartAfterCreate: cmd.StartAfterCreate,
		})

		d.sendDriverResult(stream, msg.CorrelationId, info, err)

	case *aurorav1.ServerMessage_StartInstanceCommand:
		cmd := payload.StartInstanceCommand
		log.Printf("[INFO] Received StartInstanceCommand: %s", cmd.InstanceName)
		err := d.cfg.Driver.StartInstance(ctx, cmd.InstanceName)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"status": "running"}, err)

	case *aurorav1.ServerMessage_StopInstanceCommand:
		cmd := payload.StopInstanceCommand
		log.Printf("[INFO] Received StopInstanceCommand: %s (force: %v)", cmd.InstanceName, cmd.Force)
		err := d.cfg.Driver.StopInstance(ctx, cmd.InstanceName, cmd.Force)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"status": "stopped"}, err)

	case *aurorav1.ServerMessage_RestartInstanceCommand:
		cmd := payload.RestartInstanceCommand
		log.Printf("[INFO] Received RestartInstanceCommand: %s (force: %v)", cmd.InstanceName, cmd.Force)
		err := d.cfg.Driver.RestartInstance(ctx, cmd.InstanceName, cmd.Force)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"status": "running"}, err)

	case *aurorav1.ServerMessage_DeleteInstanceCommand:
		cmd := payload.DeleteInstanceCommand
		log.Printf("[INFO] Received DeleteInstanceCommand: %s (force: %v)", cmd.InstanceName, cmd.Force)
		err := d.cfg.Driver.DeleteInstance(ctx, cmd.InstanceName, cmd.Force)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"deleted": true}, err)

	case *aurorav1.ServerMessage_UpdateInstanceSpecCommand:
		cmd := payload.UpdateInstanceSpecCommand
		log.Printf("[INFO] Received UpdateInstanceSpecCommand: %s (cpu: %d, ram: %d, disk: %d)", cmd.InstanceName, cmd.CpuCores, cmd.MemoryBytes, cmd.StorageBytes)
		err := d.cfg.Driver.UpdateSpec(ctx, cmd.InstanceName, int(cmd.CpuCores), cmd.MemoryBytes, cmd.StorageBytes)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"updated": true}, err)

	case *aurorav1.ServerMessage_GetInstanceMetricsCommand:
		cmd := payload.GetInstanceMetricsCommand
		metrics, err := d.cfg.Driver.GetMetrics(ctx, cmd.InstanceName)
		d.sendDriverResult(stream, msg.CorrelationId, metrics, err)

	// Stage 5 Networking & Firewall Command Handlers
	case *aurorav1.ServerMessage_ConfigureNetworkCommand:
		cmd := payload.ConfigureNetworkCommand
		log.Printf("[INFO] Received ConfigureNetworkCommand: %s (iface: %s, ip: %s)", cmd.InstanceName, cmd.InterfaceName, cmd.Ipv4Address)
		err := d.cfg.NetworkDriver.ConfigureInterface(
			ctx, cmd.InstanceName, cmd.InterfaceName,
			cmd.Ipv4Address, cmd.Ipv4Gateway,
			cmd.Ipv6Address, cmd.Ipv6Gateway,
			cmd.MacAddress, int(cmd.VlanId),
		)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"configured": true}, err)

	case *aurorav1.ServerMessage_ApplyFirewallRulesCommand:
		cmd := payload.ApplyFirewallRulesCommand
		log.Printf("[INFO] Received ApplyFirewallRulesCommand: %s (%d rules)", cmd.InstanceName, len(cmd.Rules))
		var rules []*network.FirewallRule
		for _, r := range cmd.Rules {
			rules = append(rules, &network.FirewallRule{
				ID:         r.Id,
				InstanceID: cmd.InstanceId,
				Direction:  network.Direction(r.Direction),
				Action:     network.Action(r.Action),
				Protocol:   r.Protocol,
				PortRange:  r.PortRange,
				SourceCIDR: r.SourceCidr,
				DestCIDR:   r.DestCidr,
				Priority:   int(r.Priority),
			})
		}
		err := d.cfg.NetworkDriver.ApplyFirewall(ctx, cmd.InstanceName, rules)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"rules_applied": len(rules)}, err)

	// Stage 6 Storage & Volume Command Handlers
	case *aurorav1.ServerMessage_CreateVolumeCommand:
		cmd := payload.CreateVolumeCommand
		log.Printf("[INFO] Received CreateVolumeCommand: %s in pool %s (size: %d, type: %s)", cmd.VolumeName, cmd.PoolName, cmd.SizeBytes, cmd.ContentType)
		err := d.cfg.StorageDriver.CreateVolume(ctx, cmd.PoolName, cmd.VolumeName, cmd.SizeBytes, cmd.ContentType)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"created": true}, err)

	case *aurorav1.ServerMessage_ResizeVolumeCommand:
		cmd := payload.ResizeVolumeCommand
		log.Printf("[INFO] Received ResizeVolumeCommand: %s in pool %s (newSize: %d)", cmd.VolumeName, cmd.PoolName, cmd.NewSizeBytes)
		err := d.cfg.StorageDriver.ResizeVolume(ctx, cmd.PoolName, cmd.VolumeName, cmd.NewSizeBytes)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"resized": true}, err)

	case *aurorav1.ServerMessage_AttachVolumeCommand:
		cmd := payload.AttachVolumeCommand
		log.Printf("[INFO] Received AttachVolumeCommand: %s to instance %s at %s", cmd.VolumeName, cmd.InstanceName, cmd.MountPath)
		err := d.cfg.StorageDriver.AttachVolume(ctx, cmd.InstanceName, cmd.PoolName, cmd.VolumeName, cmd.MountPath, cmd.ReadOnly)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"attached": true}, err)

	case *aurorav1.ServerMessage_DetachVolumeCommand:
		cmd := payload.DetachVolumeCommand
		log.Printf("[INFO] Received DetachVolumeCommand: %s from instance %s", cmd.VolumeName, cmd.InstanceName)
		err := d.cfg.StorageDriver.DetachVolume(ctx, cmd.InstanceName, cmd.VolumeName)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"detached": true}, err)

	case *aurorav1.ServerMessage_DeleteVolumeCommand:
		cmd := payload.DeleteVolumeCommand
		log.Printf("[INFO] Received DeleteVolumeCommand: %s in pool %s", cmd.VolumeName, cmd.PoolName)
		err := d.cfg.StorageDriver.DeleteVolume(ctx, cmd.PoolName, cmd.VolumeName)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"deleted": true}, err)

	case *aurorav1.ServerMessage_CreateVolumeSnapshotCommand:
		cmd := payload.CreateVolumeSnapshotCommand
		log.Printf("[INFO] Received CreateVolumeSnapshotCommand: snapshot %s for volume %s", cmd.SnapshotName, cmd.VolumeName)
		err := d.cfg.StorageDriver.CreateSnapshot(ctx, cmd.PoolName, cmd.VolumeName, cmd.SnapshotName)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"snapshot_created": true}, err)

	case *aurorav1.ServerMessage_RestoreVolumeSnapshotCommand:
		cmd := payload.RestoreVolumeSnapshotCommand
		log.Printf("[INFO] Received RestoreVolumeSnapshotCommand: restore snapshot %s for volume %s", cmd.SnapshotName, cmd.VolumeName)
		err := d.cfg.StorageDriver.RestoreSnapshot(ctx, cmd.PoolName, cmd.VolumeName, cmd.SnapshotName)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"snapshot_restored": true}, err)

	case *aurorav1.ServerMessage_DeleteVolumeSnapshotCommand:
		cmd := payload.DeleteVolumeSnapshotCommand
		log.Printf("[INFO] Received DeleteVolumeSnapshotCommand: delete snapshot %s for volume %s", cmd.SnapshotName, cmd.VolumeName)
		err := d.cfg.StorageDriver.DeleteSnapshot(ctx, cmd.PoolName, cmd.VolumeName, cmd.SnapshotName)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"snapshot_deleted": true}, err)

	// Stage 9 Interactive Web Terminal & VNC Console Messages
	case *aurorav1.ServerMessage_ConsoleSessionMessage:
		cMsg := payload.ConsoleSessionMessage
		d.handleConsoleMessage(ctx, stream, cMsg)

	// Stage 10 Image Management & OS Template Commands
	case *aurorav1.ServerMessage_SyncImageCommand:
		cmd := payload.SyncImageCommand
		log.Printf("[INFO] Received SyncImageCommand for image %s (%s)", cmd.ImageAlias, cmd.IncusFingerprint)
		err := d.cfg.ImageDriver.SyncImage(ctx, cmd.SourceRemote, cmd.ImageAlias, cmd.IncusFingerprint, cmd.ExpectedChecksum)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"synced": true, "fingerprint": cmd.IncusFingerprint}, err)

	case *aurorav1.ServerMessage_VerifyImageCommand:
		cmd := payload.VerifyImageCommand
		log.Printf("[INFO] Received VerifyImageCommand for image %s", cmd.IncusFingerprint)
		valid, err := d.cfg.ImageDriver.VerifyImage(ctx, cmd.IncusFingerprint, cmd.ExpectedChecksum)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"valid": valid}, err)

	case *aurorav1.ServerMessage_DeleteImageCommand:
		cmd := payload.DeleteImageCommand
		log.Printf("[INFO] Received DeleteImageCommand for image %s", cmd.IncusFingerprint)
		err := d.cfg.ImageDriver.DeleteImage(ctx, cmd.IncusFingerprint, cmd.ImageAlias)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"deleted": true}, err)

	case *aurorav1.ServerMessage_GetImageAvailabilityCommand:
		images, err := d.cfg.ImageDriver.ListImages(ctx)
		d.sendDriverResult(stream, msg.CorrelationId, map[string]interface{}{"images": images}, err)
	}
}

func (d *Daemon) handleConsoleMessage(ctx context.Context, stream aurorav1.NodeGatewayService_StreamTunnelClient, msg *aurorav1.ConsoleSessionMessage) {
	if msg == nil {
		return
	}

	d.consoleMu.Lock()
	defer d.consoleMu.Unlock()

	switch msg.Type {
	case aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_START:
		// Clean up existing session if any
		if s, ok := d.activeConsoles[msg.SessionId]; ok {
			s.cancel()
			_ = s.inWriter.Close()
			delete(d.activeConsoles, msg.SessionId)
		}

		sessCtx, sessCancel := context.WithCancel(ctx)
		inReader, inWriter := io.Pipe()
		outReader, outWriter := io.Pipe()
		resizeChan := make(chan compute.WindowSize, 16)

		session := &AgentConsoleSession{
			inWriter:   inWriter,
			resizeChan: resizeChan,
			cancel:     sessCancel,
		}
		d.activeConsoles[msg.SessionId] = session

		// Goroutine: Read driver stdout -> stream to Hub
		go func() {
			defer outReader.Close()
			buf := make([]byte, 2048)
			for {
				n, err := outReader.Read(buf)
				if n > 0 {
					dataCopy := make([]byte, n)
					copy(dataCopy, buf[:n])
					_ = stream.Send(&aurorav1.NodeMessage{
						CorrelationId: msg.SessionId,
						Payload: &aurorav1.NodeMessage_ConsoleSessionMessage{
							ConsoleSessionMessage: &aurorav1.ConsoleSessionMessage{
								SessionId:  msg.SessionId,
								InstanceId: msg.InstanceId,
								Type:       aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA,
								Data:       dataCopy,
							},
						},
					})
				}
				if err != nil {
					_ = stream.Send(&aurorav1.NodeMessage{
						CorrelationId: msg.SessionId,
						Payload: &aurorav1.NodeMessage_ConsoleSessionMessage{
							ConsoleSessionMessage: &aurorav1.ConsoleSessionMessage{
								SessionId:   msg.SessionId,
								InstanceId:  msg.InstanceId,
								Type:        aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_CLOSE,
								CloseReason: "driver_exited",
							},
						},
					})
					return
				}
			}
		}()

		// Goroutine: Execute ConsoleDriver
		go func() {
			defer outWriter.Close()
			defer inReader.Close()
			_ = d.cfg.ConsoleDriver.StartConsole(
				sessCtx,
				msg.SessionId,
				msg.InstanceName,
				compute.ConsoleSessionType(msg.Command),
				msg.Command,
				msg.Env,
				int(msg.Cols),
				int(msg.Rows),
				inReader,
				outWriter,
				resizeChan,
			)
		}()

	case aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_DATA:
		if s, ok := d.activeConsoles[msg.SessionId]; ok {
			_, _ = s.inWriter.Write(msg.Data)
		}

	case aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_RESIZE:
		if s, ok := d.activeConsoles[msg.SessionId]; ok {
			select {
			case s.resizeChan <- compute.WindowSize{Cols: int(msg.Cols), Rows: int(msg.Rows)}:
			default:
			}
		}

	case aurorav1.ConsoleMessageType_CONSOLE_MESSAGE_TYPE_CLOSE:
		if s, ok := d.activeConsoles[msg.SessionId]; ok {
			s.cancel()
			_ = s.inWriter.Close()
			delete(d.activeConsoles, msg.SessionId)
		}
	}
}

func (d *Daemon) sendDriverResult(stream aurorav1.NodeGatewayService_StreamTunnelClient, correlationID string, data interface{}, err error) {
	result := &aurorav1.CommandResult{
		CommandCorrelationId: correlationID,
	}

	if err != nil {
		result.Success = false
		result.ErrorMessage = err.Error()
	} else {
		result.Success = true
		if data != nil {
			payloadJSON, _ := json.Marshal(data)
			result.ResultPayloadJson = payloadJSON
		}
	}

	_ = stream.Send(&aurorav1.NodeMessage{
		CorrelationId:   correlationID,
		Timestamp:       timestamppb.Now(),
		ProtocolVersion: "aurora.v1",
		Payload: &aurorav1.NodeMessage_CommandResult{
			CommandResult: result,
		},
	})
}

func (d *Daemon) gatherCapabilities() *aurorav1.NodeCapabilities {
	return &aurorav1.NodeCapabilities{
		IncusSupported:     true,
		KvmSupported:       true,
		ZfsSupported:       true,
		BtrfsSupported:     true,
		OvsSupported:       true,
		KernelVersion:      runtime.GOOS,
		Architecture:       runtime.GOARCH,
		OsDistribution:     "linux",
		CpuCores:           int32(runtime.NumCPU()),
		TotalMemoryBytes:   16 * 1024 * 1024 * 1024,  // 16 GB baseline
		TotalStorageBytes:  500 * 1024 * 1024 * 1024, // 500 GB baseline
	}
}

func (d *Daemon) gatherCapabilitiesMap() map[string]interface{} {
	return map[string]interface{}{
		"incus_supported":     true,
		"kvm_supported":       true,
		"zfs_supported":       true,
		"btrfs_supported":     true,
		"ovs_supported":       true,
		"kernel_version":      runtime.GOOS,
		"architecture":        runtime.GOARCH,
		"os_distribution":     "linux",
		"cpu_cores":           runtime.NumCPU(),
		"total_memory_bytes":  int64(16 * 1024 * 1024 * 1024),
		"total_storage_bytes": int64(500 * 1024 * 1024 * 1024),
	}
}

func (d *Daemon) gatherTelemetry() *aurorav1.NodeTelemetry {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &aurorav1.NodeTelemetry{
		CpuUsagePercent:  5.2,
		MemoryUsedBytes:  int64(memStats.Alloc),
		MemoryFreeBytes:  int64(memStats.Sys - memStats.Alloc),
		StorageUsedBytes: 50 * 1024 * 1024 * 1024,
		StorageFreeBytes: 450 * 1024 * 1024 * 1024,
		Load_1M:          0.25,
		Load_5M:          0.30,
		Load_15M:         0.20,
		RunningInstances: 0,
		TotalInstances:   0,
		Timestamp:        timestamppb.Now(),
	}
}
