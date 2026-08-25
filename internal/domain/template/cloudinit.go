package template

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxCloudInitSizeBytes = 64 * 1024 // 64 KB limit

var (
	validUsernameRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	sshKeyPrefixRegex  = regexp.MustCompile(`^(ssh-(rsa|dss|ed25519)|ecdsa-sha2-[a-z0-9-]+)\s+[A-Za-z0-9+/]+={0,3}(\s+.*)?$`)
)

// CloudInitUser defines a guest operating system user created via cloud-init.
type CloudInitUser struct {
	Name              string   `json:"name"`
	GECOS             string   `json:"gecos,omitempty"`
	Groups            string   `json:"groups,omitempty"`
	Sudo              string   `json:"sudo,omitempty"`
	Shell             string   `json:"shell,omitempty"`
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
	LockPasswd        bool     `json:"lockPasswd"`
}

// CloudInitFile defines a file injected into the guest filesystem on first boot.
type CloudInitFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Permissions string `json:"permissions,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// CloudInitConfig defines safe, guest-scoped initialization parameters.
type CloudInitConfig struct {
	Hostname       string                 `json:"hostname,omitempty"`
	Users          []CloudInitUser        `json:"users,omitempty"`
	Packages       []string               `json:"packages,omitempty"`
	WriteFiles     []CloudInitFile        `json:"writeFiles,omitempty"`
	RunCmd         []string               `json:"runcmd,omitempty"`
	Timezone       string                 `json:"timezone,omitempty"`
	Locale         string                 `json:"locale,omitempty"`
	NetworkConfig  map[string]interface{} `json:"networkConfig,omitempty"`
	CustomUserData string                 `json:"customUserData,omitempty"`
}

// Validate checks the safety, geometry, and formatting of cloud-init configuration.
func (c *CloudInitConfig) Validate() error {
	if c == nil {
		return nil
	}

	rendered, err := c.RenderUserData()
	if err != nil {
		return err
	}

	if len(rendered) > MaxCloudInitSizeBytes {
		return ErrCloudInitOversized
	}

	for _, u := range c.Users {
		if !validUsernameRegex.MatchString(u.Name) {
			return fmt.Errorf("%w: invalid username '%s'", ErrInvalidCloudInit, u.Name)
		}
		for _, key := range u.SSHAuthorizedKeys {
			trimmed := strings.TrimSpace(key)
			if trimmed != "" && !sshKeyPrefixRegex.MatchString(trimmed) {
				return fmt.Errorf("%w: invalid SSH public key format for user '%s'", ErrInvalidCloudInit, u.Name)
			}
		}
	}

	for _, f := range c.WriteFiles {
		if !filepath.IsAbs(f.Path) || strings.Contains(f.Path, "..") {
			return fmt.Errorf("%w: file path must be absolute and cannot contain '..' ('%s')", ErrInvalidCloudInit, f.Path)
		}
	}

	return nil
}

// RenderUserData generates standard `#cloud-config` YAML data.
func (c *CloudInitConfig) RenderUserData() (string, error) {
	if c == nil {
		return "", nil
	}

	if strings.TrimSpace(c.CustomUserData) != "" {
		custom := strings.TrimSpace(c.CustomUserData)
		if !strings.HasPrefix(custom, "#cloud-config") {
			custom = "#cloud-config\n" + custom
		}
		return custom, nil
	}

	var buf bytes.Buffer
	buf.WriteString("#cloud-config\n")

	if c.Hostname != "" {
		buf.WriteString(fmt.Sprintf("hostname: %s\n", c.Hostname))
		buf.WriteString("manage_etc_hosts: true\n")
	}

	if c.Timezone != "" {
		buf.WriteString(fmt.Sprintf("timezone: %s\n", c.Timezone))
	}

	if c.Locale != "" {
		buf.WriteString(fmt.Sprintf("locale: %s\n", c.Locale))
	}

	if len(c.Users) > 0 {
		buf.WriteString("users:\n")
		for _, u := range c.Users {
			buf.WriteString(fmt.Sprintf("  - name: %s\n", u.Name))
			if u.GECOS != "" {
				buf.WriteString(fmt.Sprintf("    gecos: %s\n", u.GECOS))
			}
			if u.Groups != "" {
				buf.WriteString(fmt.Sprintf("    groups: %s\n", u.Groups))
			} else {
				buf.WriteString("    groups: sudo, users\n")
			}
			if u.Sudo != "" {
				buf.WriteString(fmt.Sprintf("    sudo: %s\n", u.Sudo))
			} else {
				buf.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
			}
			if u.Shell != "" {
				buf.WriteString(fmt.Sprintf("    shell: %s\n", u.Shell))
			} else {
				buf.WriteString("    shell: /bin/bash\n")
			}
			if u.LockPasswd {
				buf.WriteString("    lock_passwd: true\n")
			}
			if len(u.SSHAuthorizedKeys) > 0 {
				buf.WriteString("    ssh_authorized_keys:\n")
				for _, k := range u.SSHAuthorizedKeys {
					buf.WriteString(fmt.Sprintf("      - %s\n", strings.TrimSpace(k)))
				}
			}
		}
	}

	if len(c.Packages) > 0 {
		buf.WriteString("packages:\n")
		for _, p := range c.Packages {
			buf.WriteString(fmt.Sprintf("  - %s\n", strings.TrimSpace(p)))
		}
	}

	if len(c.WriteFiles) > 0 {
		buf.WriteString("write_files:\n")
		for _, f := range c.WriteFiles {
			buf.WriteString(fmt.Sprintf("  - path: %s\n", f.Path))
			if f.Permissions != "" {
				buf.WriteString(fmt.Sprintf("    permissions: '%s'\n", f.Permissions))
			} else {
				buf.WriteString("    permissions: '0644'\n")
			}
			if f.Owner != "" {
				buf.WriteString(fmt.Sprintf("    owner: %s\n", f.Owner))
			}
			buf.WriteString("    content: |\n")
			lines := strings.Split(f.Content, "\n")
			for _, line := range lines {
				buf.WriteString(fmt.Sprintf("      %s\n", line))
			}
		}
	}

	if len(c.RunCmd) > 0 {
		buf.WriteString("runcmd:\n")
		for _, cmd := range c.RunCmd {
			buf.WriteString(fmt.Sprintf("  - %s\n", cmd))
		}
	}

	return buf.String(), nil
}

// SanitizeForAudit returns non-sensitive metadata safe for logging in the audit trail.
func (c *CloudInitConfig) SanitizeForAudit() map[string]interface{} {
	if c == nil {
		return map[string]interface{}{}
	}

	userNames := make([]string, 0, len(c.Users))
	for _, u := range c.Users {
		userNames = append(userNames, u.Name)
	}

	filePaths := make([]string, 0, len(c.WriteFiles))
	for _, f := range c.WriteFiles {
		filePaths = append(filePaths, f.Path)
	}

	return map[string]interface{}{
		"hostname":   c.Hostname,
		"userCount":  len(c.Users),
		"users":      userNames,
		"packageCnt": len(c.Packages),
		"fileCount":  len(c.WriteFiles),
		"filePaths":  filePaths,
		"runcmdCnt":  len(c.RunCmd),
	}
}
