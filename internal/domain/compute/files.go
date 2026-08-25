package compute

import (
	"errors"
	"path"
	"strings"
	"time"
)

var (
	ErrInvalidPath     = errors.New("invalid file path: must be an absolute path and cannot traverse outside guest root")
	ErrFileNotFound    = errors.New("file or directory not found in guest filesystem")
	ErrFileTooLarge    = errors.New("file exceeds maximum upload limit (100MB)")
	ErrBackupNotFound  = errors.New("instance backup not found")
	ErrSnapshotNotFound = errors.New("instance snapshot not found")
)

// GuestFileInfo represents metadata for a file or directory inside a guest instance.
type GuestFileInfo struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	IsDir    bool      `json:"isDir"`
	SizeBytes int64    `json:"sizeBytes"`
	Mode     string    `json:"mode"`
	ModTime  time.Time `json:"modTime"`
}

// CleanGuestPath sanitizes and validates an absolute guest filesystem path.
func CleanGuestPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/", nil
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if strings.Contains(cleaned, "..") {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

// InstanceBackup represents a backup archive of an instance.
type InstanceBackup struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instanceId"`
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"sizeBytes"`
	Status     string    `json:"status"` // "ready", "creating", "restoring", "failed"
	CreatedAt  time.Time `json:"createdAt"`
}

// InstanceSnapshot represents a point-in-time snapshot of an instance state and root disk.
type InstanceSnapshot struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instanceId"`
	Name       string    `json:"name"`
	Stateful   bool      `json:"stateful"`
	SizeBytes  int64     `json:"sizeBytes"`
	CreatedAt  time.Time `json:"createdAt"`
}
