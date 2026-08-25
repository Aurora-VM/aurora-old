package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadServerConfig_Defaults(t *testing.T) {
	os.Clearenv()
	cfg := LoadServerConfig()
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 9443, cfg.GRPCPort)
	assert.True(t, cfg.AutoMigrate)
	assert.NotEmpty(t, cfg.MasterKey)
	assert.NotEmpty(t, cfg.JWTSecret)
}

func TestLoadServerConfig_EnvOverrides(t *testing.T) {
	os.Setenv("AURORA_HTTP_PORT", "9090")
	os.Setenv("AURORA_ENV", "production")
	os.Setenv("AURORA_AUTO_MIGRATE", "false")
	defer os.Clearenv()

	cfg := LoadServerConfig()
	assert.Equal(t, 9090, cfg.HTTPPort)
	assert.Equal(t, "production", cfg.Env)
	assert.False(t, cfg.AutoMigrate)
}

func TestMaskURL(t *testing.T) {
	raw := "postgres://aurora:secret_password@localhost:5432/aurora"
	masked := MaskURL(raw)
	assert.NotContains(t, masked, "secret_password")
	assert.Contains(t, masked, "postgres://aurora:")
}

func TestServerConfig_StringMasking(t *testing.T) {
	cfg := LoadServerConfig()
	str := cfg.String()
	assert.NotContains(t, str, "aurora_dev_password")
	assert.NotContains(t, str, cfg.MasterKey)
	assert.NotContains(t, str, cfg.JWTSecret)
}
