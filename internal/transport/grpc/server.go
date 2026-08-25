package grpc

import (
	aurorav1 "github.com/aurora-vm/aurora/gen/go/aurora/v1"
	appHealth "github.com/aurora-vm/aurora/internal/app/health"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	"google.golang.org/grpc"
)

// Register registers all Aurora gRPC services onto the server instance.
func Register(s *grpc.Server, healthService *appHealth.Service, nodeService *appNode.Service) *GatewayServer {
	// Register Health Service
	if healthService != nil {
		healthServer := NewHealthServer(healthService)
		aurorav1.RegisterHealthServiceServer(s, healthServer)
	}

	// Register Node Gateway & Enrollment Services
	var gatewayServer *GatewayServer
	if nodeService != nil {
		gatewayServer = NewGatewayServer(nodeService)
		aurorav1.RegisterNodeEnrollmentServiceServer(s, gatewayServer)
		aurorav1.RegisterNodeGatewayServiceServer(s, gatewayServer)
	}
	return gatewayServer
}
