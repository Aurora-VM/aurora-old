package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	HeaderPrefix = "v1:aes-gcm"
	NonceSize    = 12
)

// AESGCMProtector implements identity.SecretProtector with AES-256-GCM envelope encryption.
type AESGCMProtector struct {
	derivedKey []byte
}

// NewAESGCMProtector initializes an AES-256-GCM secret protector using HKDF key derivation.
func NewAESGCMProtector(masterKey string) (*AESGCMProtector, error) {
	if len(masterKey) < 16 {
		return nil, errors.New("master key must be at least 16 characters long")
	}

	// Derive a 32-byte (256-bit) encryption key using HKDF-SHA256
	hkdfReader := hkdf.New(sha256.New, []byte(masterKey), []byte("aurora-secrets-salt"), []byte("aurora-envelope-v1"))
	derivedKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derivedKey); err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	return &AESGCMProtector{derivedKey: derivedKey}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a versioned armored string payload.
func (p *AESGCMProtector) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.derivedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm block: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(HeaderPrefix))

	b64Nonce := base64.RawURLEncoding.EncodeToString(nonce)
	b64Cipher := base64.RawURLEncoding.EncodeToString(ciphertext)

	payload := fmt.Sprintf("%s:%s:%s", HeaderPrefix, b64Nonce, b64Cipher)
	return []byte(payload), nil
}

// Decrypt authenticates and decrypts an armored AES-256-GCM ciphertext payload.
func (p *AESGCMProtector) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	str := string(ciphertext)
	parts := strings.Split(str, ":")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "aes-gcm" {
		return nil, errors.New("malformed or unsupported secret ciphertext format")
	}

	nonce, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(nonce) != NonceSize {
		return nil, errors.New("invalid nonce in ciphertext")
	}

	rawCipher, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return nil, errors.New("invalid base64 ciphertext payload")
	}

	block, err := aes.NewCipher(p.derivedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, rawCipher, []byte(HeaderPrefix))
	if err != nil {
		return nil, errors.New("secret decryption failed: message authentication failed (tampered ciphertext)")
	}

	return plaintext, nil
}
