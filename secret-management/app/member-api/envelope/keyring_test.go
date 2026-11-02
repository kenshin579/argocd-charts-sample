package envelope

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestKeyring_EncryptDecrypt_RoundTrip(t *testing.T) {
	k, err := NewKeyring(1, map[uint8][]byte{1: newKey(t)})
	require.NoError(t, err)

	blob, err := k.Encrypt([]byte("alice@example.com"))
	require.NoError(t, err)
	plaintext, err := k.Decrypt(blob)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", string(plaintext))
}

func TestKeyring_DecryptUnknownVersion(t *testing.T) {
	v2, err := NewKeyring(2, map[uint8][]byte{2: newKey(t)})
	require.NoError(t, err)
	blob, err := v2.Encrypt([]byte("alice@example.com"))
	require.NoError(t, err)

	v1, err := NewKeyring(1, map[uint8][]byte{1: newKey(t)})
	require.NoError(t, err)
	_, err = v1.Decrypt(blob)
	assert.ErrorIs(t, err, ErrUnknownKeyVersion)
}

func TestKeyring_DecryptTampered(t *testing.T) {
	k, err := NewKeyring(1, map[uint8][]byte{1: newKey(t)})
	require.NoError(t, err)
	blob, err := k.Encrypt([]byte("alice@example.com"))
	require.NoError(t, err)

	blob[len(blob)-1] ^= 0xff
	_, err = k.Decrypt(blob)
	assert.Error(t, err)
}

func TestNewKeyring_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		current uint8
		keys    map[uint8][]byte
	}{
		{name: "31바이트 키", current: 1, keys: map[uint8][]byte{1: make([]byte, 31)}},
		{name: "current 버전 키 없음", current: 2, keys: map[uint8][]byte{1: newKey(t)}},
		{name: "current가 0", current: 0, keys: map[uint8][]byte{0: newKey(t)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewKeyring(tt.current, tt.keys)
			assert.ErrorIs(t, err, ErrInvalidKey)
		})
	}
}
