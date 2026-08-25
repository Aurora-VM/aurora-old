package compute

import "errors"

// Standard domain errors for Compute & Incus Virtualization management.
var (
	ErrInstanceNotFound         = errors.New("instance not found")
	ErrInstanceAlreadyExists     = errors.New("instance with this name already exists")
	ErrInstanceRunning          = errors.New("instance is already running")
	ErrInstanceStopped          = errors.New("instance is already stopped")
	ErrUnsupportedInstanceType  = errors.New("unsupported instance type: must be container or virtual-machine")
	ErrInvalidSpec              = errors.New("invalid instance specification")
	ErrHypervisorUnavailable    = errors.New("hypervisor node driver is unavailable")
	ErrInstanceOperationFailed  = errors.New("instance operation failed on hypervisor")
	ErrQuotaExceeded            = errors.New("compute quota exceeded")
	ErrInvalidPowerAction       = errors.New("invalid power action")
)
