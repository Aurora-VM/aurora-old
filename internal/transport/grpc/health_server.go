package grpc

import (
	"context"

	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	appHealth "github.com/aurora-vm/aurora/internal/app/health"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HealthServer implements aurorav1.HealthServiceServer.
type HealthServer struct {
	aurorav1.UnimplementedHealthServiceServer
	healthService *appHealth.Service
}

// NewHealthServer constructs a new gRPC Health server.
func NewHealthServer(healthService *appHealth.Service) *HealthServer {
	return &HealthServer{healthService: healthService}
}

func (s *HealthServer) Check(ctx context.Context, req *aurorav1.HealthCheckRequest) (*aurorav1.HealthCheckResponse, error) {
	h := s.healthService.GetHealth(ctx)

	var protoStatus aurorav1.SystemStatus
	switch h.Status {
	case "healthy":
		protoStatus = aurorav1.SystemStatus_SYSTEM_STATUS_HEALTHY
	case "degraded":
		protoStatus = aurorav1.SystemStatus_SYSTEM_STATUS_DEGRADED
	default:
		protoStatus = aurorav1.SystemStatus_SYSTEM_STATUS_UNHEALTHY
	}

	compMap := make(map[string]string)
	for _, c := range h.Components {
		compMap[c.Name] = string(c.Status)
	}

	return &aurorav1.HealthCheckResponse{
		Status:          protoStatus,
		Message:         string(h.Status),
		Version:         h.Version,
		Commit:          h.Commit,
		ComponentStatus: compMap,
	}, nil
}

func (s *HealthServer) Watch(req *aurorav1.HealthCheckRequest, stream aurorav1.HealthService_WatchServer) error {
	return status.Error(codes.Unimplemented, "health watch is not implemented")
}
