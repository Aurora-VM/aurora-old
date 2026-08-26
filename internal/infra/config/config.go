package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ServerConfig holds the runtime configuration for the Aurora Control Plane.
type ServerConfig struct {
	Env         string `json:"env"`
	HTTPPort    int    `json:"httpPort"`
	GRPCPort    int    `json:"grpcPort"`
	DatabaseURL string `json:"-"` // Omit from JSON serialization
	RedisURL    string `json:"-"` // Omit from JSON serialization
	MasterKey   string `json:"-"` // Omit from JSON serialization
	JWTSecret   string `json:"-"` // Omit from JSON serialization
	LogLevel      string `json:"logLevel"`
	AutoMigrate   bool   `json:"autoMigrate"`
	MigrationsDir string `json:"migrationsDir"`
}

// String provides a safe, secret-masked string representation of ServerConfig.
func (c *ServerConfig) String() string {
	return fmt.Sprintf("ServerConfig{Env: %s, HTTPPort: %d, GRPCPort: %d, DatabaseURL: %s, RedisURL: %s, LogLevel: %s, AutoMigrate: %t, MigrationsDir: %s}",
		c.Env, c.HTTPPort, c.GRPCPort, MaskURL(c.DatabaseURL), MaskURL(c.RedisURL), c.LogLevel, c.AutoMigrate, c.MigrationsDir)
}

// AgentConfig holds runtime configuration for the Aurora Node Agent.
type AgentConfig struct {
	Env             string `json:"env"`
	NodeName        string `json:"nodeName"`
	HubURL          string `json:"hubUrl"`
	EnrollmentToken string `json:"-"` // Omit from JSON serialization
	IncusSocket     string `json:"incusSocket"`
	LogLevel        string `json:"logLevel"`
}

// String provides a safe, secret-masked string representation of AgentConfig.
func (c *AgentConfig) String() string {
	maskedToken := ""
	if c.EnrollmentToken != "" {
		maskedToken = "***MASKED***"
	}
	return fmt.Sprintf("AgentConfig{Env: %s, NodeName: %s, HubURL: %s, EnrollmentToken: %s, IncusSocket: %s, LogLevel: %s}",
		c.Env, c.NodeName, c.HubURL, maskedToken, c.IncusSocket, c.LogLevel)
}

// MaskURL masks sensitive credentials in standard connection URIs.
func MaskURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "***INVALID_URL***"
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}
	return u.String()
}

// LoadServerConfig loads configuration from environment variables with safe defaults.
func LoadServerConfig() *ServerConfig {
	return &ServerConfig{
		Env:         getEnv("AURORA_ENV", "development"),
		HTTPPort:    getEnvAsInt("AURORA_HTTP_PORT", 8080),
		GRPCPort:    getEnvAsInt("AURORA_GRPC_PORT", 9443), // Use 9443 to avoid port conflicts with Incus's default 8443
		DatabaseURL: getEnv("AURORA_DATABASE_URL", "postgres://aurora:aurora_dev_password@localhost:5432/aurora?sslmode=disable"),
		RedisURL:    getEnv("AURORA_REDIS_URL", "redis://localhost:6379/0"),
		MasterKey:   getEnv("AURORA_MASTER_KEY", "aurora-dev-master-key-32-bytes-long!"),
		JWTSecret:   getEnv("AURORA_JWT_SECRET", "aurora-dev-jwt-secret-key-64-bytes-long-for-hmac-sha256-signing!"),
		LogLevel:      getEnv("AURORA_LOG_LEVEL", "info"),
		AutoMigrate:   getEnvAsBool("AURORA_AUTO_MIGRATE", true),
		MigrationsDir: getEnv("AURORA_MIGRATIONS_DIR", "/etc/aurora/migrations"),
	}
}

// LoadAgentConfig loads agent configuration from environment variables with safe defaults.
func LoadAgentConfig() *AgentConfig {
	return &AgentConfig{
		Env:             getEnv("AURORA_ENV", "development"),
		NodeName:        getEnv("AURORA_NODE_NAME", "localhost-node"),
		HubURL:          getEnv("AURORA_HUB_URL", "https://localhost:9443"),
		EnrollmentToken: getEnv("AURORA_ENROLLMENT_TOKEN", ""),
		IncusSocket:     getEnv("AURORA_INCUS_SOCKET", "/var/lib/incus/unix.socket"),
		LogLevel:        getEnv("AURORA_LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := getEnv(key, "")
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	valStr := getEnv(key, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}
	return defaultVal
}
