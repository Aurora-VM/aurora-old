package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgon2Hasher_HashAndVerify(t *testing.T) {
	hasher := NewArgon2Hasher(nil)
	password := "CorrectHorseBatteryStaple123!"

	hash, err := hasher.Hash(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Contains(t, hash, "$argon2id$v=19$")

	// Successful verification
	match, err := hasher.Verify(password, hash)
	require.NoError(t, err)
	assert.True(t, match)

	// Wrong password failure
	match, err = hasher.Verify("WrongPassword!", hash)
	require.NoError(t, err)
	assert.False(t, match)
}

func TestArgon2Hasher_InvalidHash(t *testing.T) {
	hasher := NewArgon2Hasher(nil)

	// Malformed hash strings
	_, err := hasher.Verify("password", "invalid_hash")
	assert.Error(t, err)

	_, err = hasher.Verify("password", "$bcrypt$v=19$m=64,t=1,p=1$salt$hash")
	assert.Error(t, err)
}
