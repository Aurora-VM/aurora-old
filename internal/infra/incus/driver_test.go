package incus

import (
	"context"
	"testing"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulatedDriver_Lifecycle_FullFlow(t *testing.T) {
	ctx := context.Background()
	drv := NewSimulatedDriver()

	// 1. Create container instance
	spec := &compute.InstanceSpec{
		Name:             "test-web-01",
		Type:             compute.TypeContainer,
		CPUCores:         2,
		MemoryBytes:      2 * 1024 * 1024 * 1024,
		StorageBytes:     20 * 1024 * 1024 * 1024,
		Image:            "images:ubuntu/24.04",
		StartAfterCreate: true,
	}

	info, err := drv.CreateInstance(ctx, spec)
	require.NoError(t, err)
	assert.Equal(t, "test-web-01", info.Name)
	assert.Equal(t, compute.StatusRunning, info.Status)
	assert.Equal(t, compute.TypeContainer, info.Type)
	assert.NotEmpty(t, info.IPv4Address)

	// 2. Duplicate creation -> ErrInstanceAlreadyExists
	_, err = drv.CreateInstance(ctx, spec)
	assert.ErrorIs(t, err, compute.ErrInstanceAlreadyExists)

	// 3. Stop instance
	err = drv.StopInstance(ctx, "test-web-01", false)
	require.NoError(t, err)

	info, err = drv.GetInstance(ctx, "test-web-01")
	require.NoError(t, err)
	assert.Equal(t, compute.StatusStopped, info.Status)

	// 4. Double stop -> ErrInstanceStopped
	err = drv.StopInstance(ctx, "test-web-01", false)
	assert.ErrorIs(t, err, compute.ErrInstanceStopped)

	// 5. Restart instance
	err = drv.RestartInstance(ctx, "test-web-01", false)
	require.NoError(t, err)

	info, err = drv.GetInstance(ctx, "test-web-01")
	require.NoError(t, err)
	assert.Equal(t, compute.StatusRunning, info.Status)

	// 6. Get live metrics
	metrics, err := drv.GetMetrics(ctx, "test-web-01")
	require.NoError(t, err)
	assert.Greater(t, metrics.CPUUsagePercent, 0.0)
	assert.Greater(t, metrics.MemoryUsageBytes, int64(0))

	// 7. Update spec
	err = drv.UpdateSpec(ctx, "test-web-01", 4, 4*1024*1024*1024, 40*1024*1024*1024)
	require.NoError(t, err)

	// 8. Delete instance
	err = drv.DeleteInstance(ctx, "test-web-01", true)
	require.NoError(t, err)

	_, err = drv.GetInstance(ctx, "test-web-01")
	assert.ErrorIs(t, err, compute.ErrInstanceNotFound)
}
