// member-api 는 회원 개인정보를 필드 단위로 암호화해 저장하는 데모 서비스다.
//
//	member-api serve     HTTP 서버. DEK 는 Pod 에 주입된 Vault Agent(proxy, 127.0.0.1:8200)를 거쳐 token 없이 받는다
//	member-api init-dek  DEK 적재 Job 용(ArgoCD Sync hook). ServiceAccount token 으로 Kubernetes auth 에 직접 로그인해 DEK v1 을 적재한다
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/kubernetes"

	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/envelope"
	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "serve":
		err = serve(ctx)
	case "init-dek":
		err = initDEK(ctx)
	default:
		err = fmt.Errorf("unknown command %q (serve | init-dek)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context) error {
	port := getenv("PORT", "8080")
	mode := getenv("LOAD_MODE", "async")

	// VAULT_ADDR 는 Agent 의 리스너(http://127.0.0.1:8200)다. Agent 가 auto-auth token 을 붙여 주므로 token 을 두지 않는다
	client, err := vaultapi.NewClient(vaultapi.DefaultConfig())
	if err != nil {
		return err
	}
	client.ClearToken()

	s := server.New(getenv("DATA_PATH", "/data/members.json"), os.Getenv("API_TOKEN"))
	load := func(ctx context.Context) (*envelope.Keyring, error) {
		return envelope.LoadKeyring(ctx, client, envelope.DefaultConfig)
	}
	retry := server.Retry{Initial: time.Second, Max: 10 * time.Second}

	switch mode {
	case "sync":
		// keyring 을 받을 때까지 HTTP 서버를 띄우지 않는다. 그동안 livenessProbe 는 연결 거부로 실패한다
		slog.Info("loading keyring before starting HTTP server", "load_mode", mode)
		if err := s.LoadSync(ctx, load, retry); err != nil {
			return err
		}
	case "async":
		s.LoadAsync(ctx, load, retry)
	default:
		return fmt.Errorf("LOAD_MODE must be sync or async, got %q", mode)
	}

	srv := &http.Server{Addr: ":" + port, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("listening", "addr", srv.Addr, "load_mode", mode)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func initDEK(ctx context.Context) error {
	client, err := vaultapi.NewClient(vaultapi.DefaultConfig())
	if err != nil {
		return err
	}

	// 기본 경로(/var/run/secrets/kubernetes.io/serviceaccount/token)의 ServiceAccount token 으로 로그인한다
	auth, err := kubernetes.NewKubernetesAuth(getenv("VAULT_ROLE", "member-api-migration"))
	if err != nil {
		return err
	}
	secret, err := client.Auth().Login(ctx, auth)
	if err != nil {
		return fmt.Errorf("kubernetes auth login: %w", err)
	}
	if secret == nil || secret.Auth == nil {
		return errors.New("kubernetes auth login returned no token")
	}

	created, err := envelope.InitDEK(ctx, client, envelope.DefaultConfig)
	if err != nil {
		return err
	}
	if created {
		fmt.Println("DEK v1 created")
	} else {
		fmt.Println("DEK already exists, skipped")
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
