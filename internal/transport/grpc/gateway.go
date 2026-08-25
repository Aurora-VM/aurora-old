package grpc

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"sync"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GatewayServer implements aurorav1.NodeEnrollmentServiceServer and aurorav1.NodeGatewayServiceServer.
type GatewayServer struct {
	aurorav1.UnimplementedNodeEnrollmentServiceServer
	aurorav1.UnimplementedNodeGatewayServiceServer
	nodeService   *appNode.Service
	consoleRouter func(*aurorav1.ConsoleSessionMessage)
}

// NewGatewayServer creates a new gRPC Gateway server.
func NewGatewayServer(nodeService *appNode.Service) *GatewayServer {
	return &GatewayServer{
		nodeService: nodeService,
	}
}

func (s *GatewayServer) SetConsoleRouter(router func(*aurorav1.ConsoleSessionMessage)) {
	s.consoleRouter = router
}

func (s *GatewayServer) SendToNode(nodeID string, msg *aurorav1.ServerMessage) error {
	sender, ok := s.nodeService.GetConnection(nodeID)
	if !ok {
		return domainNode.ErrNodeOffline
	}
	if rawSender, ok := sender.(RawServerMessageSender); ok {
		return rawSender.SendRaw(msg)
	}
	return errors.New("stream sender does not support raw message dispatch")
}

// EnrollNode exchanges an enrollment token and CSR for a signed client certificate.
func (s *GatewayServer) EnrollNode(ctx context.Context, req *aurorav1.EnrollNodeRequest) (*aurorav1.EnrollNodeResponse, error) {
	if req.EnrollmentToken == "" {
		return nil, status.Error(codes.InvalidArgument, "enrollment token is required")
	}
	if len(req.CsrPem) == 0 {
		return nil, status.Error(codes.InvalidArgument, "csr_pem is required")
	}

	capsMap := make(map[string]interface{})
	if req.Capabilities != nil {
		capsMap["cpu_cores"] = req.Capabilities.CpuCores
		capsMap["total_memory_bytes"] = req.Capabilities.TotalMemoryBytes
		capsMap["total_storage_bytes"] = req.Capabilities.TotalStorageBytes
		capsMap["incus_supported"] = req.Capabilities.IncusSupported
		capsMap["kvm_supported"] = req.Capabilities.KvmSupported
		capsMap["os_distribution"] = req.Capabilities.OsDistribution
		capsMap["architecture"] = req.Capabilities.Architecture
	}

	nodeID, certPEM, caCertPEM, interval, err := s.nodeService.EnrollNode(
		ctx, req.EnrollmentToken, req.NodeName, req.Fqdn, req.CsrPem, capsMap,
	)
	if err != nil {
		if errors.Is(err, domainNode.ErrEnrollmentTokenInvalid) {
			return nil, status.Error(codes.Unauthenticated, "invalid enrollment token")
		}
		if errors.Is(err, domainNode.ErrEnrollmentTokenExpired) {
			return nil, status.Error(codes.Unauthenticated, "enrollment token has expired")
		}
		if errors.Is(err, domainNode.ErrEnrollmentTokenUsed) {
			return nil, status.Error(codes.AlreadyExists, "enrollment token has already been consumed")
		}
		if errors.Is(err, domainNode.ErrInvalidCSR) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "enrollment failed: %v", err)
	}

	return &aurorav1.EnrollNodeResponse{
		NodeId:                   nodeID,
		CertificatePem:           certPEM,
		CaCertificatePem:         caCertPEM,
		HubGrpcEndpoint:          "127.0.0.1:8443",
		HeartbeatIntervalSeconds: int32(interval),
	}, nil
}

// StreamTunnel handles persistent bidirectional mTLS communication from node agents.
func (s *GatewayServer) StreamTunnel(stream aurorav1.NodeGatewayService_StreamTunnelServer) error {
	ctx := stream.Context()

	// 1. Extract and verify peer TLS client certificate
	clientCertPEM, err := s.extractPeerCertificate(ctx)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "mTLS peer verification failed: %v", err)
	}

	authenticatedNode, err := s.nodeService.AuthenticateNodeCertificate(ctx, clientCertPEM)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeRevoked) {
			return status.Error(codes.PermissionDenied, "node certificate has been revoked")
		}
		return status.Errorf(codes.Unauthenticated, "node authentication failed: %v", err)
	}

	nodeID := authenticatedNode.ID
	log.Printf("[INFO] Inbound mTLS stream tunnel established for node %s (%s)", nodeID, authenticatedNode.Name)

	// 2. Wrap stream in thread-safe StreamSender
	sender := &grpcStreamSender{
		stream: stream,
	}

	if err := s.nodeService.OnStreamConnected(ctx, nodeID, sender); err != nil {
		return status.Errorf(codes.Internal, "failed to register node stream: %v", err)
	}
	defer s.nodeService.OnStreamDisconnected(ctx, nodeID)

	// 3. Inbound message loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			log.Printf("[INFO] Node stream tunnel %s closed: %v", nodeID, err)
			return err
		}

		s.handleNodeMessage(ctx, stream, nodeID, msg)
	}
}

func (s *GatewayServer) handleNodeMessage(ctx context.Context, stream aurorav1.NodeGatewayService_StreamTunnelServer, nodeID string, msg *aurorav1.NodeMessage) {
	switch payload := msg.Payload.(type) {
	case *aurorav1.NodeMessage_ReadyEvent:
		log.Printf("[INFO] Node %s reported Ready (agent version: %s)", nodeID, payload.ReadyEvent.AgentVersion)

	case *aurorav1.NodeMessage_Heartbeat:
		caps := make(map[string]interface{})
		if payload.Heartbeat.Capabilities != nil {
			caps["cpu_cores"] = payload.Heartbeat.Capabilities.CpuCores
			caps["total_memory_bytes"] = payload.Heartbeat.Capabilities.TotalMemoryBytes
			caps["total_storage_bytes"] = payload.Heartbeat.Capabilities.TotalStorageBytes
		}
		_ = s.nodeService.ProcessHeartbeat(ctx, nodeID, caps)

		// Send HeartbeatAck
		_ = stream.Send(&aurorav1.ServerMessage{
			CorrelationId:   msg.CorrelationId,
			Timestamp:       timestamppb.Now(),
			ProtocolVersion: "aurora.v1",
			Payload: &aurorav1.ServerMessage_HeartbeatAck{
				HeartbeatAck: &aurorav1.HeartbeatAck{
					AcknowledgedSequence:     payload.Heartbeat.SequenceNumber,
					ServerTime:               timestamppb.Now(),
					NodeStatus:               aurorav1.NodeStatus_NODE_STATUS_ONLINE,
					HeartbeatIntervalSeconds: 10,
				},
			},
		})

	case *aurorav1.NodeMessage_CommandResult:
		res := &domainNode.CommandResult{
			CorrelationID: payload.CommandResult.CommandCorrelationId,
			Success:       payload.CommandResult.Success,
			ErrorMessage:  payload.CommandResult.ErrorMessage,
			CompletedAt:   timestamppb.Now().AsTime(),
		}
		if len(payload.CommandResult.ResultPayloadJson) > 0 {
			var payloadMap map[string]interface{}
			if err := json.Unmarshal(payload.CommandResult.ResultPayloadJson, &payloadMap); err == nil {
				res.Payload = payloadMap
			}
		}
		s.nodeService.HandleCommandResult(res)

	case *aurorav1.NodeMessage_ConsoleSessionMessage:
		if s.consoleRouter != nil {
			s.consoleRouter(payload.ConsoleSessionMessage)
		}
	}
}

// RawServerMessageSender allows sending pre-built ServerMessages over connected streams.
type RawServerMessageSender interface {
	SendRaw(srvMsg *aurorav1.ServerMessage) error
}

func (s *GatewayServer) extractPeerCertificate(ctx context.Context) ([]byte, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return nil, errors.New("no peer auth info available in context")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, errors.New("auth info is not TLS")
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return nil, errors.New("no peer certificates presented")
	}

	peerCert := tlsInfo.State.PeerCertificates[0]
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: peerCert.Raw,
	})

	return certPEM, nil
}

// grpcStreamSender adapts gRPC stream to domain.StreamSender and RawServerMessageSender.
type grpcStreamSender struct {
	mu     sync.Mutex
	stream aurorav1.NodeGatewayService_StreamTunnelServer
}

func (g *grpcStreamSender) SendRaw(srvMsg *aurorav1.ServerMessage) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stream.Send(srvMsg)
}

func (g *grpcStreamSender) Send(cmd *domainNode.Command) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	srvMsg := &aurorav1.ServerMessage{
		CorrelationId:   cmd.CorrelationID,
		Timestamp:       timestamppb.Now(),
		ProtocolVersion: "aurora.v1",
	}

	switch cmd.Type {
	case "ping":
		payloadStr := ""
		if cmd.Payload != nil && cmd.Payload["msg"] != nil {
			payloadStr = fmt.Sprintf("%v", cmd.Payload["msg"])
		}
		srvMsg.Payload = &aurorav1.ServerMessage_PingCommand{
			PingCommand: &aurorav1.PingCommand{Payload: payloadStr},
		}

	case "collect_telemetry":
		srvMsg.Payload = &aurorav1.ServerMessage_TelemetryCommand{
			TelemetryCommand: &aurorav1.CollectTelemetryCommand{Detailed: true},
		}

	case "apply_config":
		cfgJSON, _ := json.Marshal(cmd.Payload)
		srvMsg.Payload = &aurorav1.ServerMessage_ConfigCommand{
			ConfigCommand: &aurorav1.ApplyNodeConfigCommand{ConfigJson: string(cfgJSON)},
		}

	case "reboot":
		delay := int32(5)
		reason := "maintenance"
		srvMsg.Payload = &aurorav1.ServerMessage_RebootCommand{
			RebootCommand: &aurorav1.RebootNodeCommand{DelaySeconds: delay, Reason: reason},
		}

	// Stage 4 Incus Virtualization Commands
	case "create_instance":
		cfgJSON, _ := json.Marshal(cmd.Payload["config"])
		srvMsg.Payload = &aurorav1.ServerMessage_CreateInstanceCommand{
			CreateInstanceCommand: &aurorav1.CreateInstanceCommand{
				InstanceId:       fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName:     fmt.Sprintf("%v", cmd.Payload["name"]),
				InstanceType:     fmt.Sprintf("%v", cmd.Payload["type"]),
				CpuCores:         int32(toInt(cmd.Payload["cpu_cores"])),
				MemoryBytes:      toInt64(cmd.Payload["memory_bytes"]),
				StorageBytes:     toInt64(cmd.Payload["storage_bytes"]),
				Image:            fmt.Sprintf("%v", cmd.Payload["image"]),
				ConfigJson:       string(cfgJSON),
				StartAfterCreate: toBool(cmd.Payload["start_after_create"]),
			},
		}

	case "start_instance":
		srvMsg.Payload = &aurorav1.ServerMessage_StartInstanceCommand{
			StartInstanceCommand: &aurorav1.StartInstanceCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["name"]),
			},
		}

	case "stop_instance":
		srvMsg.Payload = &aurorav1.ServerMessage_StopInstanceCommand{
			StopInstanceCommand: &aurorav1.StopInstanceCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["name"]),
				Force:        toBool(cmd.Payload["force"]),
			},
		}

	case "restart_instance":
		srvMsg.Payload = &aurorav1.ServerMessage_RestartInstanceCommand{
			RestartInstanceCommand: &aurorav1.RestartInstanceCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["name"]),
				Force:        toBool(cmd.Payload["force"]),
			},
		}

	case "delete_instance":
		srvMsg.Payload = &aurorav1.ServerMessage_DeleteInstanceCommand{
			DeleteInstanceCommand: &aurorav1.DeleteInstanceCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["name"]),
				Force:        toBool(cmd.Payload["force"]),
			},
		}

	case "update_instance_spec":
		srvMsg.Payload = &aurorav1.ServerMessage_UpdateInstanceSpecCommand{
			UpdateInstanceSpecCommand: &aurorav1.UpdateInstanceSpecCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["name"]),
				CpuCores:     int32(toInt(cmd.Payload["cpu_cores"])),
				MemoryBytes:  toInt64(cmd.Payload["memory_bytes"]),
				StorageBytes: toInt64(cmd.Payload["storage_bytes"]),
			},
		}

	case "get_instance_metrics":
		srvMsg.Payload = &aurorav1.ServerMessage_GetInstanceMetricsCommand{
			GetInstanceMetricsCommand: &aurorav1.GetInstanceMetricsCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["name"]),
			},
		}

	case "configure_network":
		srvMsg.Payload = &aurorav1.ServerMessage_ConfigureNetworkCommand{
			ConfigureNetworkCommand: &aurorav1.ConfigureNetworkCommand{
				InstanceId:    fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName:  fmt.Sprintf("%v", cmd.Payload["instance_name"]),
				InterfaceName: fmt.Sprintf("%v", cmd.Payload["interface_name"]),
				Ipv4Address:   fmt.Sprintf("%v", cmd.Payload["ipv4_address"]),
				Ipv4Gateway:   fmt.Sprintf("%v", cmd.Payload["ipv4_gateway"]),
				Ipv6Address:   fmt.Sprintf("%v", cmd.Payload["ipv6_address"]),
				Ipv6Gateway:   fmt.Sprintf("%v", cmd.Payload["ipv6_gateway"]),
				MacAddress:    fmt.Sprintf("%v", cmd.Payload["mac_address"]),
				VlanId:        int32(toInt(cmd.Payload["vlan_id"])),
			},
		}

	case "apply_firewall_rules":
		srvMsg.Payload = &aurorav1.ServerMessage_ApplyFirewallRulesCommand{
			ApplyFirewallRulesCommand: &aurorav1.ApplyFirewallRulesCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["instance_name"]),
			},
		}

	case "create_volume":
		srvMsg.Payload = &aurorav1.ServerMessage_CreateVolumeCommand{
			CreateVolumeCommand: &aurorav1.CreateVolumeCommand{
				VolumeId:    fmt.Sprintf("%v", cmd.Payload["volume_id"]),
				PoolName:    fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName:  fmt.Sprintf("%v", cmd.Payload["volume_name"]),
				SizeBytes:   toInt64(cmd.Payload["size_bytes"]),
				ContentType: fmt.Sprintf("%v", cmd.Payload["content_type"]),
			},
		}

	case "resize_volume":
		srvMsg.Payload = &aurorav1.ServerMessage_ResizeVolumeCommand{
			ResizeVolumeCommand: &aurorav1.ResizeVolumeCommand{
				VolumeId:     fmt.Sprintf("%v", cmd.Payload["volume_id"]),
				PoolName:     fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName:   fmt.Sprintf("%v", cmd.Payload["volume_name"]),
				NewSizeBytes: toInt64(cmd.Payload["new_size_bytes"]),
			},
		}

	case "attach_volume":
		srvMsg.Payload = &aurorav1.ServerMessage_AttachVolumeCommand{
			AttachVolumeCommand: &aurorav1.AttachVolumeCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["instance_name"]),
				PoolName:     fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName:   fmt.Sprintf("%v", cmd.Payload["volume_name"]),
				MountPath:    fmt.Sprintf("%v", cmd.Payload["mount_path"]),
				ReadOnly:     toBool(cmd.Payload["read_only"]),
			},
		}

	case "detach_volume":
		srvMsg.Payload = &aurorav1.ServerMessage_DetachVolumeCommand{
			DetachVolumeCommand: &aurorav1.DetachVolumeCommand{
				InstanceId:   fmt.Sprintf("%v", cmd.Payload["instance_id"]),
				InstanceName: fmt.Sprintf("%v", cmd.Payload["instance_name"]),
				VolumeName:   fmt.Sprintf("%v", cmd.Payload["volume_name"]),
			},
		}

	case "delete_volume":
		srvMsg.Payload = &aurorav1.ServerMessage_DeleteVolumeCommand{
			DeleteVolumeCommand: &aurorav1.DeleteVolumeCommand{
				VolumeId:   fmt.Sprintf("%v", cmd.Payload["volume_id"]),
				PoolName:   fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName: fmt.Sprintf("%v", cmd.Payload["volume_name"]),
			},
		}

	case "create_volume_snapshot":
		srvMsg.Payload = &aurorav1.ServerMessage_CreateVolumeSnapshotCommand{
			CreateVolumeSnapshotCommand: &aurorav1.CreateVolumeSnapshotCommand{
				VolumeId:     fmt.Sprintf("%v", cmd.Payload["volume_id"]),
				PoolName:     fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName:   fmt.Sprintf("%v", cmd.Payload["volume_name"]),
				SnapshotName: fmt.Sprintf("%v", cmd.Payload["snapshot_name"]),
			},
		}

	case "restore_volume_snapshot":
		srvMsg.Payload = &aurorav1.ServerMessage_RestoreVolumeSnapshotCommand{
			RestoreVolumeSnapshotCommand: &aurorav1.RestoreVolumeSnapshotCommand{
				VolumeId:     fmt.Sprintf("%v", cmd.Payload["volume_id"]),
				PoolName:     fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName:   fmt.Sprintf("%v", cmd.Payload["volume_name"]),
				SnapshotName: fmt.Sprintf("%v", cmd.Payload["snapshot_name"]),
			},
		}

	case "delete_volume_snapshot":
		srvMsg.Payload = &aurorav1.ServerMessage_DeleteVolumeSnapshotCommand{
			DeleteVolumeSnapshotCommand: &aurorav1.DeleteVolumeSnapshotCommand{
				VolumeId:     fmt.Sprintf("%v", cmd.Payload["volume_id"]),
				PoolName:     fmt.Sprintf("%v", cmd.Payload["pool_name"]),
				VolumeName:   fmt.Sprintf("%v", cmd.Payload["volume_name"]),
				SnapshotName: fmt.Sprintf("%v", cmd.Payload["snapshot_name"]),
			},
		}

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}

	return g.stream.Send(srvMsg)
}

func toInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// Helper to construct self-signed peer cert in tests
func MockPeerContext(ctx context.Context, cert *x509.Certificate) context.Context {
	return peer.NewContext(ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: credentials.TLSInfo{}.State,
		},
	})
}
