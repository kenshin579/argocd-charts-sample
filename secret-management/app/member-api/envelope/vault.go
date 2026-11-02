package envelope

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

// KV v2 문서에 DEK 를 저장하는 필드 이름이다(1편과 같다).
//
//	current_version = "1"          // 암호화에 쓰는 버전
//	wrapped_v1      = "vault:v1:…" // KEK 로 wrap 된 DEK
const (
	currentVersionField = "current_version"
	wrappedFieldPrefix  = "wrapped_v"
	dekBits             = 256
)

var (
	// ErrDEKNotInitialized 는 KV 에 DEK 문서가 아직 없을 때 반환된다.
	ErrDEKNotInitialized = errors.New("DEK not initialized")
	// ErrDEKDeleted 는 DEK 문서의 최신 버전이 삭제(soft delete)된 상태일 때 반환된다.
	ErrDEKDeleted = errors.New("DEK document is deleted")
)

// Config 는 KEK 이름과 wrap 된 DEK 를 저장할 KV v2 위치다.
type Config struct {
	KEKName string
	KVMount string
	KVPath  string
}

// DefaultConfig 는 vault/policies/*.hcl 의 경로와 맞춘 기본 위치다.
var DefaultConfig = Config{
	KEKName: "member-api-kek",
	KVMount: "secret",
	KVPath:  "member-api/dek",
}

// InitDEK 는 DEK 문서가 없을 때만 v1 을 만든다. 이미 있으면 아무것도 쓰지 않고 false 를 돌려준다.
// PreSync Job 이 sync 마다 다시 돌거나 두 Job 이 겹쳐도, check-and-set(cas=0) 때문에 문서는 한 번만 쓰인다.
func InitDEK(ctx context.Context, c *vaultapi.Client, cfg Config) (bool, error) {
	secret, err := c.KVv2(cfg.KVMount).Get(ctx, cfg.KVPath)
	switch {
	case err == nil && secret.Data == nil:
		return false, fmt.Errorf("%w: %s/%s", ErrDEKDeleted, cfg.KVMount, cfg.KVPath)
	case err == nil:
		return false, nil
	case !errors.Is(err, vaultapi.ErrSecretNotFound):
		return false, fmt.Errorf("read DEK %s/%s: %w", cfg.KVMount, cfg.KVPath, err)
	}

	resp, err := c.Logical().WriteWithContext(ctx, "transit/datakey/wrapped/"+cfg.KEKName, map[string]interface{}{
		"bits": dekBits,
	})
	if err != nil {
		return false, fmt.Errorf("generate DEK with %s: %w", cfg.KEKName, err)
	}
	if resp == nil {
		return false, errors.New("transit datakey returned no response")
	}
	ciphertext, _ := resp.Data["ciphertext"].(string)
	if ciphertext == "" {
		return false, errors.New("transit datakey returned no ciphertext")
	}

	data := map[string]interface{}{
		currentVersionField:      "1",
		wrappedFieldPrefix + "1": ciphertext,
	}
	_, err = c.KVv2(cfg.KVMount).Put(ctx, cfg.KVPath, data, vaultapi.WithCheckAndSet(0))
	if isCASConflict(err) {
		// 다른 Job 이 먼저 만들었다
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("write DEK %s/%s: %w", cfg.KVMount, cfg.KVPath, err)
	}
	return true, nil
}

// LoadKeyring 은 KV 에서 wrap 된 DEK 를 모두 읽고 Transit 으로 unwrap 해서 Keyring 을 만든다.
// 평문 DEK 는 반환된 Keyring 안(메모리)에만 있다.
func LoadKeyring(ctx context.Context, c *vaultapi.Client, cfg Config) (*Keyring, error) {
	secret, err := c.KVv2(cfg.KVMount).Get(ctx, cfg.KVPath)
	if errors.Is(err, vaultapi.ErrSecretNotFound) {
		return nil, fmt.Errorf("%w: %s/%s", ErrDEKNotInitialized, cfg.KVMount, cfg.KVPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read DEK %s/%s: %w", cfg.KVMount, cfg.KVPath, err)
	}
	if secret.Data == nil {
		return nil, fmt.Errorf("%w: %s/%s", ErrDEKDeleted, cfg.KVMount, cfg.KVPath)
	}

	current, wrapped, err := parseDEKData(secret.Data)
	if err != nil {
		return nil, err
	}

	keys := make(map[uint8][]byte, len(wrapped))
	for version, ciphertext := range wrapped {
		resp, err := c.Logical().WriteWithContext(ctx, "transit/decrypt/"+cfg.KEKName, map[string]interface{}{
			"ciphertext": ciphertext,
		})
		if err != nil {
			return nil, fmt.Errorf("unwrap DEK v%d: %w", version, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("unwrap DEK v%d: transit decrypt returned no response", version)
		}
		encoded, _ := resp.Data["plaintext"].(string)
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) == 0 {
			return nil, fmt.Errorf("unwrap DEK v%d: invalid plaintext", version)
		}
		keys[version] = key
	}
	return NewKeyring(current, keys)
}

func isCASConflict(err error) bool {
	var respErr *vaultapi.ResponseError
	if !errors.As(err, &respErr) || respErr.StatusCode != http.StatusBadRequest {
		return false
	}
	for _, e := range respErr.Errors {
		if strings.Contains(e, "check-and-set") {
			return true
		}
	}
	return false
}

func parseDEKData(data map[string]interface{}) (uint8, map[uint8]string, error) {
	raw, _ := data[currentVersionField].(string)
	current, err := parseVersion(raw)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", currentVersionField, err)
	}

	wrapped := map[uint8]string{}
	for field, value := range data {
		if !strings.HasPrefix(field, wrappedFieldPrefix) {
			continue
		}
		version, err := parseVersion(strings.TrimPrefix(field, wrappedFieldPrefix))
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", field, err)
		}
		ciphertext, _ := value.(string)
		if ciphertext == "" {
			return 0, nil, fmt.Errorf("%s is empty", field)
		}
		wrapped[version] = ciphertext
	}

	if _, ok := wrapped[current]; !ok {
		return 0, nil, fmt.Errorf("%s=%d but %s%d is missing", currentVersionField, current, wrappedFieldPrefix, current)
	}
	return current, wrapped, nil
}

func parseVersion(s string) (uint8, error) {
	v, err := strconv.ParseUint(s, 10, 8)
	if err != nil || v == 0 || strconv.FormatUint(v, 10) != s {
		return 0, fmt.Errorf("invalid DEK version %q (want 1-255)", s)
	}
	return uint8(v), nil
}
