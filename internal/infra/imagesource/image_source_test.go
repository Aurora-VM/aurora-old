package imagesource

import (
	"context"
	"testing"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageSource_Inspect_And_Verify(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry([]string{"images", "ubuntu"})

	// 1. Inspect valid remote image
	art, err := reg.Inspect(ctx, "images", "ubuntu/24.04")
	require.NoError(t, err)
	assert.Equal(t, "x86_64", art.Architecture)
	assert.Equal(t, compute.TypeContainer, art.InstanceType)
	assert.Equal(t, "images:ubuntu/24.04", art.ImageAlias)
	assert.NotEmpty(t, art.IncusFingerprint)

	// 2. Inspect VM image with aarch64
	artVM, err := reg.Inspect(ctx, "images", "ubuntu/24.04/cloud/arm64")
	require.NoError(t, err)
	assert.Equal(t, "aarch64", artVM.Architecture)
	assert.Equal(t, compute.TypeVirtualMachine, artVM.InstanceType)

	// 3. Inspect from untrusted remote -> error
	_, err = reg.Inspect(ctx, "untrusted-hacker-remote", "malicious-image")
	assert.Error(t, err)

	// 4. Verify valid artifact
	valid, err := reg.Verify(ctx, art)
	require.NoError(t, err)
	assert.True(t, valid)

	// 5. Verify artifact with checksum mismatch
	tampered := *art
	tampered.Checksum = "badbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbad0"
	valid, err = reg.Verify(ctx, &tampered)
	assert.False(t, valid)
	assert.ErrorIs(t, err, template.ErrFingerprintMismatch)
}
