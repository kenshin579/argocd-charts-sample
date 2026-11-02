// Package member 는 회원 개인정보(이메일, 전화번호)를 필드 단위로 암호화해 JSON 파일에 저장한다.
package member

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/kenshin579/argocd-charts-sample/secret-management/app/member-api/envelope"
)

// ErrNotFound 는 ID 에 해당하는 회원이 없을 때 반환된다.
var ErrNotFound = errors.New("member not found")

// Member 는 복호화된 회원 정보다.
type Member struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type document struct {
	Members []Member `json:"members"` // Email·Phone 은 base64 로 인코딩한 암호문
}

// Repository 는 Keyring 으로 암복호화하며 회원을 JSON 파일에 읽고 쓴다.
// HTTP 핸들러가 동시에 부르므로 파일 접근을 mutex 로 직렬화한다.
type Repository struct {
	mu      sync.Mutex
	path    string
	keyring *envelope.Keyring
}

// NewRepository 는 path 의 파일을 쓰는 Repository 를 만든다. 파일은 처음 Put 할 때 생긴다.
func NewRepository(path string, k *envelope.Keyring) *Repository {
	return &Repository{path: path, keyring: k}
}

// Put 은 Email·Phone 을 암호화해 저장한다. 같은 ID 가 있으면 덮어쓴다.
func (r *Repository) Put(m Member) error {
	sealed := Member{ID: m.ID}
	var err error
	if sealed.Email, err = r.seal(m.Email); err != nil {
		return err
	}
	if sealed.Phone, err = r.seal(m.Phone); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	members, err := r.load()
	if err != nil {
		return err
	}
	for i := range members {
		if members[i].ID == m.ID {
			members[i] = sealed
			return r.save(members)
		}
	}
	return r.save(append(members, sealed))
}

// Get 은 ID 로 회원을 찾아 복호화한다.
func (r *Repository) Get(id string) (Member, error) {
	r.mu.Lock()
	members, err := r.load()
	r.mu.Unlock()
	if err != nil {
		return Member{}, err
	}

	for _, m := range members {
		if m.ID != id {
			continue
		}
		email, err := r.open(m.Email)
		if err != nil {
			return Member{}, fmt.Errorf("decrypt email of %s: %w", id, err)
		}
		phone, err := r.open(m.Phone)
		if err != nil {
			return Member{}, fmt.Errorf("decrypt phone of %s: %w", id, err)
		}
		return Member{ID: id, Email: email, Phone: phone}, nil
	}
	return Member{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

func (r *Repository) seal(plaintext string) (string, error) {
	blob, err := r.keyring.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(blob), nil
}

func (r *Repository) open(encoded string) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	plaintext, err := r.keyring.Decrypt(blob)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (r *Repository) load() ([]Member, error) {
	raw, err := os.ReadFile(r.path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(raw) == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", r.path, err)
	}
	return doc.Members, nil
}

// save 는 임시 파일에 쓰고 fsync 한 뒤 rename 해서, 쓰다가 실패해도 기존 파일이 깨지지 않게 한다.
func (r *Repository) save(members []Member) error {
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(document{Members: members}, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".members-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.path)
}
