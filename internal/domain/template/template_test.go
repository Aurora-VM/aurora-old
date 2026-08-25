package template

import (
	"strings"
	"testing"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSTemplate_Validation(t *testing.T) {
	valid := &OSTemplate{
		Name:         "Ubuntu 24.04 LTS",
		Slug:         "ubuntu-24.04",
		Distribution: "ubuntu",
		Version:      "24.04",
	}
	require.NoError(t, valid.Validate())
	assert.Equal(t, TemplateStatusActive, valid.Status)
	assert.Equal(t, []string{"x86_64"}, valid.SupportedArchitectures)
	assert.Equal(t, []compute.InstanceType{compute.TypeContainer, compute.TypeVirtualMachine}, valid.SupportedInstanceTypes)
	assert.Equal(t, int64(5*1024*1024*1024), valid.MinDiskBytes)

	// Invalid slug
	invalidSlug := &OSTemplate{
		Name:         "Ubuntu 24.04",
		Slug:         "Ubuntu_24.04!",
		Distribution: "ubuntu",
		Version:      "24.04",
	}
	assert.ErrorIs(t, invalidSlug.Validate(), ErrInvalidTemplateSpec)

	// Missing fields
	empty := &OSTemplate{}
	assert.ErrorIs(t, empty.Validate(), ErrInvalidTemplateSpec)
}

func TestImageArtifact_Validation(t *testing.T) {
	valid := &ImageArtifact{
		TemplateID:       "tmpl-1234",
		Architecture:     "x86_64",
		InstanceType:     compute.TypeContainer,
		IncusFingerprint: strings.Repeat("a", 64),
		Checksum:         strings.Repeat("b", 64),
	}
	require.NoError(t, valid.Validate())
	assert.Equal(t, ImageStatusAvailable, valid.Status)

	// Invalid fingerprint
	badFp := &ImageArtifact{
		TemplateID:       "tmpl-1234",
		Architecture:     "x86_64",
		InstanceType:     compute.TypeContainer,
		IncusFingerprint: "not-a-hex-string",
	}
	assert.ErrorIs(t, badFp.Validate(), ErrInvalidFingerprint)

	// Invalid instance type
	badType := &ImageArtifact{
		TemplateID:   "tmpl-1234",
		Architecture: "x86_64",
		InstanceType: "bare-metal",
	}
	assert.ErrorIs(t, badType.Validate(), ErrUnsupportedInstanceType)
}

func TestCloudInitConfig_Validation_And_Rendering(t *testing.T) {
	cfg := &CloudInitConfig{
		Hostname: "aurora-guest-01",
		Timezone: "UTC",
		Locale:   "en_US.UTF-8",
		Users: []CloudInitUser{
			{
				Name:   "ubuntu",
				GECOS:  "Ubuntu Default User",
				Groups: "sudo, users",
				Sudo:   "ALL=(ALL) NOPASSWD:ALL",
				Shell:  "/bin/bash",
				SSHAuthorizedKeys: []string{
					"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGo4k7E8o9t1H+g6u8B/z/d5W1j9l2k3m4n5o6p7q8r9 admin@aurora",
				},
				LockPasswd: true,
			},
		},
		Packages: []string{"curl", "htop", "nginx"},
		WriteFiles: []CloudInitFile{
			{
				Path:        "/etc/motd",
				Content:     "Welcome to Aurora Cloud!",
				Permissions: "0644",
			},
		},
		RunCmd: []string{
			"systemctl enable nginx",
			"systemctl start nginx",
		},
	}

	require.NoError(t, cfg.Validate())

	rendered, err := cfg.RenderUserData()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rendered, "#cloud-config"))
	assert.Contains(t, rendered, "hostname: aurora-guest-01")
	assert.Contains(t, rendered, "name: ubuntu")
	assert.Contains(t, rendered, "ssh-ed25519 AAAAC3")
	assert.Contains(t, rendered, "packages:")
	assert.Contains(t, rendered, "nginx")
	assert.Contains(t, rendered, "/etc/motd")
	assert.Contains(t, rendered, "Welcome to Aurora Cloud!")

	// Test Sanitization for Audit
	auditMap := cfg.SanitizeForAudit()
	assert.Equal(t, "aurora-guest-01", auditMap["hostname"])
	assert.Equal(t, []string{"ubuntu"}, auditMap["users"])
	assert.Equal(t, []string{"/etc/motd"}, auditMap["filePaths"])

	// Test Path Traversal rejection
	badPathCfg := &CloudInitConfig{
		WriteFiles: []CloudInitFile{
			{
				Path:    "/etc/../root/.ssh/authorized_keys",
				Content: "evil",
			},
		},
	}
	assert.ErrorIs(t, badPathCfg.Validate(), ErrInvalidCloudInit)

	// Test Invalid SSH Key rejection
	badKeyCfg := &CloudInitConfig{
		Users: []CloudInitUser{
			{
				Name:              "admin",
				SSHAuthorizedKeys: []string{"malformed key without prefix"},
			},
		},
	}
	assert.ErrorIs(t, badKeyCfg.Validate(), ErrInvalidCloudInit)

	// Test Oversized Cloud-Init rejection
	hugeContent := strings.Repeat("A", 70*1024)
	hugeCfg := &CloudInitConfig{
		CustomUserData: hugeContent,
	}
	assert.ErrorIs(t, hugeCfg.Validate(), ErrCloudInitOversized)
}
