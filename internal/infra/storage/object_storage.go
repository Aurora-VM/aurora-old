package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	ErrObjectNotFound  = errors.New("object storage item not found")
	ErrCorruptedObject = errors.New("object checksum validation mismatch: artifact is corrupted")
)

// ObjectStorage abstracts blob and backup artifact storage (compatible with S3/R2/local).
type ObjectStorage interface {
	Put(ctx context.Context, key string, data []byte, metadata map[string]string) error
	Get(ctx context.Context, key string) ([]byte, map[string]string, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// MemoryObjectStorage provides in-memory thread-safe storage for tests and single-node setups.
type MemoryObjectStorage struct {
	mu      sync.RWMutex
	data    map[string][]byte
	meta    map[string]map[string]string
	hashes  map[string]string
}

func NewMemoryObjectStorage() *MemoryObjectStorage {
	return &MemoryObjectStorage{
		data:   make(map[string][]byte),
		meta:   make(map[string]map[string]string),
		hashes: make(map[string]string),
	}
}

func (s *MemoryObjectStorage) Put(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hasher := sha256.New()
	hasher.Write(data)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	cp := make([]byte, len(data))
	copy(cp, data)

	metaCp := make(map[string]string)
	for k, v := range metadata {
		metaCp[k] = v
	}

	s.data[key] = cp
	s.meta[key] = metaCp
	s.hashes[key] = checksum
	return nil
}

func (s *MemoryObjectStorage) Get(ctx context.Context, key string) ([]byte, map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.data[key]
	if !exists {
		return nil, nil, ErrObjectNotFound
	}

	// Verify SHA-256 on read
	hasher := sha256.New()
	hasher.Write(data)
	computedChecksum := hex.EncodeToString(hasher.Sum(nil))

	expectedChecksum := s.hashes[key]
	if expectedChecksum != "" && computedChecksum != expectedChecksum {
		return nil, nil, ErrCorruptedObject
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, s.meta[key], nil
}

func (s *MemoryObjectStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	delete(s.meta, key)
	delete(s.hashes, key)
	return nil
}

func (s *MemoryObjectStorage) Exists(ctx context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.data[key]
	return exists, nil
}

// CorruptArtifactForTesting artificially corrupts a stored object to verify integrity checks.
func (s *MemoryObjectStorage) CorruptArtifactForTesting(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if d, ok := s.data[key]; ok && len(d) > 0 {
		d[0] ^= 0xFF // Flip bits
	}
}

// EncryptedStorageWrapper adds AES-256-GCM encryption at rest to any ObjectStorage driver.
type EncryptedStorageWrapper struct {
	underlying ObjectStorage
	key        []byte
}

func NewEncryptedStorageWrapper(underlying ObjectStorage, key []byte) (*EncryptedStorageWrapper, error) {
	if len(key) != 32 {
		// If key length is not 32 bytes, derive a 32-byte key via SHA-256
		hasher := sha256.Sum256(key)
		key = hasher[:]
	}
	return &EncryptedStorageWrapper{
		underlying: underlying,
		key:        key,
	}, nil
}

func (e *EncryptedStorageWrapper) Put(ctx context.Context, key string, plaintext []byte, metadata map[string]string) error {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return fmt.Errorf("failed to initialize AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to initialize GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate random nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return e.underlying.Put(ctx, key, ciphertext, metadata)
}

func (e *EncryptedStorageWrapper) Get(ctx context.Context, key string) ([]byte, map[string]string, error) {
	ciphertext, meta, err := e.underlying.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, nil, ErrCorruptedObject
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to decrypt artifact: %v", ErrCorruptedObject, err)
	}

	return plaintext, meta, nil
}

func (e *EncryptedStorageWrapper) Delete(ctx context.Context, key string) error {
	return e.underlying.Delete(ctx, key)
}

func (e *EncryptedStorageWrapper) Exists(ctx context.Context, key string) (bool, error) {
	return e.underlying.Exists(ctx, key)
}
