package envelope

import (
	"context"
	"os"
	"sync"
	"testing"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcvault "github.com/testcontainers/testcontainers-go/modules/vault"
)

const vaultImage = "hashicorp/vault:2.2.1"

// startVault 는 Transit KEK 와 policy 를 준비한 dev Vault 를 띄우고 root client 를 돌려준다.
func startVault(t *testing.T) *vaultapi.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("Docker 가 필요한 통합 테스트")
	}
	ctx := context.Background()

	ctr, err := tcvault.Run(ctx, vaultImage,
		tcvault.WithToken("root"),
		tcvault.WithInitCommand("secrets enable transit", "write -f transit/keys/member-api-kek"),
	)
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)

	addr, err := ctr.HttpHostAddress(ctx)
	require.NoError(t, err)
	cfg := vaultapi.DefaultConfig()
	cfg.Address = addr
	root, err := vaultapi.NewClient(cfg)
	require.NoError(t, err)
	root.SetToken("root")

	for _, name := range []string{"member-api", "member-api-migration"} {
		rules, err := os.ReadFile("../../../vault/policies/" + name + ".hcl")
		require.NoError(t, err)
		require.NoError(t, root.Sys().PutPolicy(name, string(rules)))
	}
	return root
}

func newTokenClient(t *testing.T, root *vaultapi.Client, policy string) *vaultapi.Client {
	t.Helper()
	secret, err := root.Auth().Token().Create(&vaultapi.TokenCreateRequest{Policies: []string{policy}, TTL: "1h"})
	require.NoError(t, err)
	c, err := root.Clone()
	require.NoError(t, err)
	c.SetToken(secret.Auth.ClientToken)
	return c
}

func TestInitDEK_Idempotent(t *testing.T) {
	ctx := context.Background()
	root := startVault(t)
	migration := newTokenClient(t, root, "member-api-migration")

	created, err := InitDEK(ctx, migration, DefaultConfig)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = InitDEK(ctx, migration, DefaultConfig)
	require.NoError(t, err)
	assert.False(t, created)
}

func TestInitDEK_ConcurrentCreatesOnce(t *testing.T) {
	ctx := context.Background()
	root := startVault(t)
	migration := newTokenClient(t, root, "member-api-migration")

	var wg sync.WaitGroup
	results := make([]bool, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = InitDEK(ctx, migration, DefaultConfig)
		}()
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.True(t, results[0] != results[1], "정확히 한 번만 created=true 여야 한다: %v", results)

	secret, err := root.KVv2("secret").Get(ctx, "member-api/dek")
	require.NoError(t, err)
	assert.Equal(t, "1", secret.Data["current_version"])
	assert.Equal(t, 1, secret.VersionMetadata.Version, "KV 문서는 한 번만 쓰여야 한다")
}

func TestLoadKeyring_NotInitialized(t *testing.T) {
	root := startVault(t)
	app := newTokenClient(t, root, "member-api")

	_, err := LoadKeyring(context.Background(), app, DefaultConfig)
	assert.ErrorIs(t, err, ErrDEKNotInitialized)
}

func TestLoadKeyring_RoundTrip(t *testing.T) {
	ctx := context.Background()
	root := startVault(t)
	migration := newTokenClient(t, root, "member-api-migration")
	app := newTokenClient(t, root, "member-api")

	_, err := InitDEK(ctx, migration, DefaultConfig)
	require.NoError(t, err)

	k1, err := LoadKeyring(ctx, app, DefaultConfig)
	require.NoError(t, err)
	blob, err := k1.Encrypt([]byte("alice@example.com"))
	require.NoError(t, err)

	// 앱이 재시작해서 다시 로드해도 같은 DEK 를 받는다
	k2, err := LoadKeyring(ctx, app, DefaultConfig)
	require.NoError(t, err)
	plaintext, err := k2.Decrypt(blob)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", string(plaintext))
}

func TestIsCASConflict_RealVaultError(t *testing.T) {
	ctx := context.Background()
	root := startVault(t)
	migration := newTokenClient(t, root, "member-api-migration")

	_, err := InitDEK(ctx, migration, DefaultConfig)
	require.NoError(t, err)

	// 이미 문서가 있는데 cas=0 으로 쓰면 Vault 가 거부한다. 이 에러를 CAS 충돌로 알아봐야 한다
	_, err = migration.KVv2("secret").Put(ctx, "member-api/dek", map[string]interface{}{"x": "y"}, vaultapi.WithCheckAndSet(0))
	require.Error(t, err)
	assert.True(t, isCASConflict(err), "got %v", err)

	assert.False(t, isCASConflict(nil))
}
