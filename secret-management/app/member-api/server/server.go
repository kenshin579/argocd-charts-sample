// Package server 는 member-api 의 HTTP 서버다. 시작할 때 Vault 에서 DEK keyring 을 받아야 회원 API 를 처리할 수 있다.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/envelope"
	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/member"
)

// Loader 는 Vault 에서 keyring 을 받아 온다. Vault 가 sealed 이거나 Agent 가 아직 인증 전이면 실패한다.
type Loader func(ctx context.Context) (*envelope.Keyring, error)

// Retry 는 keyring 로드 재시도 간격이다. 실패할 때마다 두 배로 늘리고 Max 에서 멈춘다.
type Retry struct {
	Initial time.Duration
	Max     time.Duration
}

// Server 는 keyring 을 로드하기 전까지 회원 API 에 503 을 돌려준다.
type Server struct {
	dataPath string
	apiToken string
	repo     atomic.Pointer[member.Repository]
}

// New 는 dataPath 에 회원을 저장하는 Server 를 만든다. apiToken 은 K8s Secret 에서 받은 설정 값이다.
func New(dataPath, apiToken string) *Server {
	return &Server{dataPath: dataPath, apiToken: apiToken}
}

// Ready 는 keyring 을 로드했는지 알려 준다.
func (s *Server) Ready() bool {
	return s.repo.Load() != nil
}

// LoadSync 는 keyring 로드에 성공할 때까지 재시도하고, ctx 가 끝나면 ctx 의 에러를 돌려준다.
func (s *Server) LoadSync(ctx context.Context, load Loader, r Retry) error {
	wait := r.Initial
	for attempt := 1; ; attempt++ {
		k, err := load(ctx)
		if err == nil {
			s.repo.Store(member.NewRepository(s.dataPath, k))
			slog.Info("keyring loaded", "attempt", attempt, "dek_version", k.Current())
			return nil
		}
		slog.Warn("keyring load failed, will retry", "attempt", attempt, "retry_in", wait.String(), "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*2, r.Max)
	}
}

// LoadAsync 는 LoadSync 를 goroutine 으로 실행한다. HTTP 서버를 먼저 띄울 때 쓴다.
func (s *Server) LoadAsync(ctx context.Context, load Loader, r Retry) {
	go func() {
		if err := s.LoadSync(ctx, load, r); err != nil {
			slog.Error("keyring load stopped", "error", err)
		}
	}()
}

// Handler 는 HTTP 라우팅이다.
//
//	GET  /healthz       항상 200 (livenessProbe)
//	GET  /readyz        keyring 로드 전 503, 후 200 (readinessProbe)
//	POST /members       회원 저장
//	GET  /members/{id}  회원 조회
//	GET  /config        설정 시크릿(API 토큰)을 가려서 보여 준다
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !s.Ready() {
			writeError(w, http.StatusServiceUnavailable, "keyring not loaded")
			return
		}
		_, _ = w.Write([]byte("ready"))
	})
	mux.HandleFunc("POST /members", s.withRepo(s.putMember))
	mux.HandleFunc("GET /members/{id}", s.withRepo(s.getMember))
	mux.HandleFunc("GET /config", s.config)
	return mux
}

func (s *Server) withRepo(next func(http.ResponseWriter, *http.Request, *member.Repository)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo := s.repo.Load()
		if repo == nil {
			writeError(w, http.StatusServiceUnavailable, "keyring not loaded")
			return
		}
		next(w, r, repo)
	}
}

func (s *Server) putMember(w http.ResponseWriter, r *http.Request, repo *member.Repository) {
	var m member.Member
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil || m.ID == "" {
		writeError(w, http.StatusBadRequest, "body must be JSON with id, email, phone")
		return
	}
	if err := repo.Put(m); err != nil {
		slog.Error("put member failed", "id", m.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save member")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": m.ID})
}

func (s *Server) getMember(w http.ResponseWriter, r *http.Request, repo *member.Repository) {
	m, err := repo.Get(r.PathValue("id"))
	if errors.Is(err, member.ErrNotFound) {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		slog.Error("get member failed", "id", r.PathValue("id"), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read member")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) config(w http.ResponseWriter, _ *http.Request) {
	masked := "****"
	if len(s.apiToken) > 2 {
		masked = s.apiToken[:2] + "****"
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_token": masked, "length": len(s.apiToken)})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
