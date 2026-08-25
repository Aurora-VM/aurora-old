package ipam

import "errors"

// Standard domain errors for IPAM.
var (
	ErrIPPoolNotFound       = errors.New("ip pool not found")
	ErrIPPoolAlreadyExists  = errors.New("ip pool with this cidr already exists")
	ErrIPPoolExhausted      = errors.New("no available ip addresses in pool")
	ErrIPAlreadyAllocated   = errors.New("ip address is already allocated")
	ErrIPAllocationNotFound = errors.New("ip allocation not found")
	ErrInvalidCIDR          = errors.New("invalid cidr format")
	ErrSubnetOverlap        = errors.New("subnet overlaps with an existing ip pool")
	ErrNetworkUnreachable   = errors.New("node or location does not have reachability to this ip pool")
)
