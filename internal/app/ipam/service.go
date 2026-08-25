package ipam

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainIPAM "github.com/aurora-vm/aurora/internal/domain/ipam"
)

// Service provides IPAM application workflows.
type Service struct {
	poolRepo   domainIPAM.IPPoolRepository
	allocRepo  domainIPAM.IPAllocationRepository
	authorizer identity.Authorizer
	auditRepo  audit.Repository
}

// NewService constructs an IPAM application service.
func NewService(
	poolRepo domainIPAM.IPPoolRepository,
	allocRepo domainIPAM.IPAllocationRepository,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		poolRepo:   poolRepo,
		allocRepo:  allocRepo,
		authorizer: authorizer,
		auditRepo:  auditRepo,
	}
}

type CreatePoolRequest struct {
	Name       string   `json:"name"`
	LocationID string   `json:"locationId"`
	CIDR       string   `json:"cidr"`
	Gateway    string   `json:"gateway"`
	DNSServers []string `json:"dnsServers"`
	VLANID     *int     `json:"vlanId,omitempty"`
	IsPrivate  bool     `json:"isPrivate"`
}

func (s *Service) CreatePool(ctx context.Context, sub *identity.Subject, req CreatePoolRequest) (*domainIPAM.IPPool, error) {
	if err := s.authorizer.Authorize(ctx, sub, "ipam:manage", nil); err != nil {
		return nil, err
	}

	ip, ipNet, err := net.ParseCIDR(req.CIDR)
	if err != nil {
		return nil, domainIPAM.ErrInvalidCIDR
	}

	ipVer := 4
	if ip.To4() == nil {
		ipVer = 6
	}

	// Validate Gateway is inside CIDR
	gwIP := net.ParseIP(req.Gateway)
	if gwIP == nil || !ipNet.Contains(gwIP) {
		return nil, fmt.Errorf("gateway %s is not within cidr %s", req.Gateway, req.CIDR)
	}

	if len(req.DNSServers) == 0 {
		req.DNSServers = []string{"1.1.1.1", "8.8.8.8"}
	}

	pool := &domainIPAM.IPPool{
		Name:       req.Name,
		LocationID: req.LocationID,
		IPVersion:  ipVer,
		CIDR:       ipNet.String(),
		Gateway:    req.Gateway,
		DNSServers: req.DNSServers,
		VLANID:     req.VLANID,
		IsPrivate:  req.IsPrivate,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.poolRepo.Create(ctx, pool); err != nil {
		return nil, err
	}

	actorID := sub.UserID
	resID := pool.ID
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "ipam:pool:create",
		ActorID:      &actorID,
		ResourceType: "ip_pool",
		ResourceID:   &resID,
		CreatedAt:    time.Now().UTC(),
	})

	return pool, nil
}

func (s *Service) ListPools(ctx context.Context, sub *identity.Subject, locationID string) ([]*domainIPAM.IPPool, error) {
	if err := s.authorizer.Authorize(ctx, sub, "ipam:read", nil); err != nil {
		return nil, err
	}
	return s.poolRepo.List(ctx, locationID)
}

func (s *Service) GetPool(ctx context.Context, sub *identity.Subject, poolID string) (*domainIPAM.IPPool, *domainIPAM.PoolUtilization, error) {
	if err := s.authorizer.Authorize(ctx, sub, "ipam:read", nil); err != nil {
		return nil, nil, err
	}

	pool, err := s.poolRepo.GetByID(ctx, poolID)
	if err != nil {
		return nil, nil, err
	}

	utilization, err := s.GetPoolUtilization(ctx, pool)
	if err != nil {
		return nil, nil, err
	}

	return pool, utilization, nil
}

func (s *Service) AllocateIP(ctx context.Context, sub *identity.Subject, poolID string, instanceID *string, ifaceName string, isReserved bool, notes string) (*domainIPAM.IPAllocation, error) {
	if err := s.authorizer.Authorize(ctx, sub, "ipam:manage", nil); err != nil {
		return nil, err
	}

	pool, err := s.poolRepo.GetByID(ctx, poolID)
	if err != nil {
		return nil, err
	}

	allocations, err := s.allocRepo.ListByPoolID(ctx, poolID)
	if err != nil {
		return nil, err
	}

	allocatedMap := make(map[string]bool)
	for _, a := range allocations {
		allocatedMap[a.IPAddress] = true
	}
	// Gateway is reserved
	allocatedMap[pool.Gateway] = true

	nextIP, err := s.findNextAvailableIP(pool.CIDR, allocatedMap)
	if err != nil {
		return nil, err
	}

	if ifaceName == "" {
		ifaceName = "eth0"
	}

	alloc := &domainIPAM.IPAllocation{
		PoolID:        poolID,
		InstanceID:    instanceID,
		IPAddress:     nextIP,
		InterfaceName: ifaceName,
		IsReserved:    isReserved,
		Notes:         notes,
		AllocatedAt:   time.Now().UTC(),
	}

	if err := s.allocRepo.Create(ctx, alloc); err != nil {
		return nil, err
	}

	actorID := sub.UserID
	resID := alloc.ID
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "ipam:ip:allocate",
		ActorID:      &actorID,
		ResourceType: "ip_allocation",
		ResourceID:   &resID,
		CreatedAt:    time.Now().UTC(),
	})

	return alloc, nil
}

func (s *Service) ReleaseIP(ctx context.Context, sub *identity.Subject, allocationID string) error {
	if err := s.authorizer.Authorize(ctx, sub, "ipam:manage", nil); err != nil {
		return err
	}

	alloc, err := s.allocRepo.GetByID(ctx, allocationID)
	if err != nil {
		return err
	}

	if err := s.allocRepo.Release(ctx, allocationID); err != nil {
		return err
	}

	actorID := sub.UserID
	resID := alloc.ID
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "ipam:ip:release",
		ActorID:      &actorID,
		ResourceType: "ip_allocation",
		ResourceID:   &resID,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

func (s *Service) ListAllocations(ctx context.Context, sub *identity.Subject, poolID string) ([]*domainIPAM.IPAllocation, error) {
	if err := s.authorizer.Authorize(ctx, sub, "ipam:read", nil); err != nil {
		return nil, err
	}
	return s.allocRepo.ListByPoolID(ctx, poolID)
}

func (s *Service) GetPoolUtilization(ctx context.Context, pool *domainIPAM.IPPool) (*domainIPAM.PoolUtilization, error) {
	allocations, err := s.allocRepo.ListByPoolID(ctx, pool.ID)
	if err != nil {
		return nil, err
	}

	_, ipNet, err := net.ParseCIDR(pool.CIDR)
	if err != nil {
		return nil, err
	}

	total := s.calculateTotalHostIPs(ipNet)
	allocated := int64(0)
	reserved := int64(0)

	for _, a := range allocations {
		if a.IsReserved {
			reserved++
		} else {
			allocated++
		}
	}

	free := total - (allocated + reserved)
	if free < 0 {
		free = 0
	}

	usagePct := 0.0
	if total > 0 {
		usagePct = (float64(allocated+reserved) / float64(total)) * 100.0
	}

	return &domainIPAM.PoolUtilization{
		PoolID:          pool.ID,
		CIDR:            pool.CIDR,
		TotalIPs:        total,
		AllocatedIPs:    allocated,
		ReservedIPs:     reserved,
		FreeIPs:         free,
		UsagePercentage: math.Round(usagePct*100) / 100,
	}, nil
}

func (s *Service) findNextAvailableIP(cidr string, allocated map[string]bool) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", domainIPAM.ErrInvalidCIDR
	}

	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return "", domainIPAM.ErrIPPoolExhausted
	}

	maskSize, _ := ipNet.Mask.Size()
	startInt := binary.BigEndian.Uint32(ip4)
	totalHosts := uint32(1 << (32 - maskSize))

	// Skip network address (i = 0) and broadcast address (i = totalHosts - 1)
	for i := uint32(1); i < totalHosts-1; i++ {
		currentInt := startInt + i
		currentIP := make(net.IP, 4)
		binary.BigEndian.PutUint32(currentIP, currentInt)
		ipStr := currentIP.String()

		if !allocated[ipStr] {
			return ipStr, nil
		}
	}

	return "", domainIPAM.ErrIPPoolExhausted
}

func (s *Service) calculateTotalHostIPs(ipNet *net.IPNet) int64 {
	maskSize, _ := ipNet.Mask.Size()
	if ipNet.IP.To4() != nil {
		if maskSize >= 31 {
			return 2
		}
		return int64(1<<(32-maskSize)) - 2
	}
	return 65536
}
