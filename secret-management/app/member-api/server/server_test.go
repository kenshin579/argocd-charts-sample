package server

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/envelope"
)

func newKeyring(t *testing.T) *envelope.Keyring {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	k, err := envelope.NewKeyring(1, map[uint8][]byte{1: key})
	require.NoError(t, err)
	return k
}

func okLoader(k *envelope.Keyring) Loader {
	return func(context.Context) (*envelope.Keyring, error) { return k, nil }
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newLoadedServer(t *testing.T) (*Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "members.json")
	s := New(path, "demo-api-token-v1")
	require.NoError(t, s.LoadSync(context.Background(), okLoader(newKeyring(t)), Retry{Initial: time.Millisecond, Max: time.Millisecond}))
	return s, path
}

func TestServer_HealthzAlwaysOK(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "members.json"), "")
	rec := do(t, s.Handler(), http.MethodGet, "/healthz", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestServer_ReadyzBeforeLoad(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "members.json"), "")
	assert.False(t, s.Ready())
	assert.Equal(t, http.StatusServiceUnavailable, do(t, s.Handler(), http.MethodGet, "/readyz", "").Code)
}

func TestServer_MembersBeforeReady(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "members.json"), "")
	h := s.Handler()

	post := do(t, h, http.MethodPost, "/members", `{"id":"alice","email":"alice@example.com","phone":"010-1234-5678"}`)
	assert.Equal(t, http.StatusServiceUnavailable, post.Code)
	assert.JSONEq(t, `{"error":"keyring not loaded"}`, post.Body.String())

	get := do(t, h, http.MethodGet, "/members/alice", "")
	assert.Equal(t, http.StatusServiceUnavailable, get.Code)
	assert.JSONEq(t, `{"error":"keyring not loaded"}`, get.Body.String())
}

func TestServer_LoadAsyncRetriesUntilSuccess(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "members.json"), "")
	k := newKeyring(t)
	var calls atomic.Int32
	load := func(context.Context) (*envelope.Keyring, error) {
		// Agent 가 auto-auth 를 끝내기 전이나 Vault 가 sealed 일 때처럼 처음 두 번은 실패한다
		if calls.Add(1) <= 2 {
			return nil, errors.New("Code: 403. Errors: permission denied")
		}
		return k, nil
	}

	s.LoadAsync(context.Background(), load, Retry{Initial: time.Millisecond, Max: 5 * time.Millisecond})

	assert.Eventually(t, func() bool {
		return do(t, s.Handler(), http.MethodGet, "/readyz", "").Code == http.StatusOK
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, int32(3), calls.Load())
}

func TestServer_LoadSyncStopsOnContextCancel(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "members.json"), "")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	failing := func(context.Context) (*envelope.Keyring, error) { return nil, errors.New("vault is sealed") }

	err := s.LoadSync(ctx, failing, Retry{Initial: 5 * time.Millisecond, Max: 10 * time.Millisecond})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, s.Ready())
}

func TestServer_MembersRoundTrip(t *testing.T) {
	s, path := newLoadedServer(t)
	h := s.Handler()

	post := do(t, h, http.MethodPost, "/members", `{"id":"alice","email":"alice@example.com","phone":"010-1234-5678"}`)
	assert.Equal(t, http.StatusCreated, post.Code)

	get := do(t, h, http.MethodGet, "/members/alice", "")
	assert.Equal(t, http.StatusOK, get.Code)
	assert.JSONEq(t, `{"id":"alice","email":"alice@example.com","phone":"010-1234-5678"}`, get.Body.String())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "alice@example.com")
}

func TestServer_PostInvalidJSON(t *testing.T) {
	s, _ := newLoadedServer(t)
	assert.Equal(t, http.StatusBadRequest, do(t, s.Handler(), http.MethodPost, "/members", `{"id":`).Code)
	assert.Equal(t, http.StatusBadRequest, do(t, s.Handler(), http.MethodPost, "/members", `{"email":"a@b.c"}`).Code)
}

func TestServer_GetUnknownMember(t *testing.T) {
	s, _ := newLoadedServer(t)
	assert.Equal(t, http.StatusNotFound, do(t, s.Handler(), http.MethodGet, "/members/bob", "").Code)
}

func TestServer_ConfigMasksToken(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "members.json"), "demo-api-token-v1")
	rec := do(t, s.Handler(), http.MethodGet, "/config", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"api_token":"de****","length":17}`, rec.Body.String())
}
