package member

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/envelope"
)

var alice = Member{ID: "alice", Email: "alice@example.com", Phone: "010-1234-5678"}

func newKeyring(t *testing.T) *envelope.Keyring {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	k, err := envelope.NewKeyring(1, map[uint8][]byte{1: key})
	require.NoError(t, err)
	return k
}

func TestRepository_PutGet_RoundTrip(t *testing.T) {
	repo := NewRepository(filepath.Join(t.TempDir(), "data", "members.json"), newKeyring(t))
	require.NoError(t, repo.Put(alice))

	got, err := repo.Get("alice")
	require.NoError(t, err)
	assert.Equal(t, alice, got)
}

func TestRepository_FileHasNoPlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members.json")
	require.NoError(t, NewRepository(path, newKeyring(t)).Put(alice))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), alice.Email)
	assert.NotContains(t, string(raw), alice.Phone)
}

func TestRepository_GetMissingFile(t *testing.T) {
	repo := NewRepository(filepath.Join(t.TempDir(), "members.json"), newKeyring(t))
	_, err := repo.Get("alice")
	assert.ErrorIs(t, err, ErrNotFound)
}
