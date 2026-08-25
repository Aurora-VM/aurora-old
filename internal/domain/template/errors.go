package template

import "errors"

var (
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateSlugExists        = errors.New("template slug already exists")
	ErrImageArtifactNotFound     = errors.New("image artifact not found")
	ErrNoCompatibleImage         = errors.New("no compatible image artifact found for requested architecture and instance type")
	ErrInvalidFingerprint        = errors.New("invalid incus image fingerprint (must be 64 hexadecimal characters)")
	ErrFingerprintMismatch       = errors.New("image verification failed: fingerprint or checksum mismatch")
	ErrInvalidTemplateSpec       = errors.New("invalid template specification")
	ErrInvalidImageSpec          = errors.New("invalid image artifact specification")
	ErrInvalidCloudInit          = errors.New("invalid cloud-init configuration")
	ErrCloudInitOversized        = errors.New("cloud-init payload exceeds maximum allowed size (64KB)")
	ErrTemplateInUse             = errors.New("cannot delete template: active instances are referencing this template")
	ErrImageInUse                = errors.New("cannot delete image artifact: active instances are referencing this image")
	ErrUnsupportedArchitecture   = errors.New("unsupported architecture for template")
	ErrUnsupportedInstanceType   = errors.New("unsupported instance type for template")
	ErrInsufficientDisk          = errors.New("instance disk allocation is less than template minimum requirement")
	ErrInsufficientMemory        = errors.New("instance memory allocation is less than template minimum requirement")
	ErrSyncJobFailed             = errors.New("image synchronization job failed")
)
