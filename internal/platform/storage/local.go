package storage

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrDenied = errors.New("storage access denied")
var ErrUnavailable = errors.New("storage operation failed")
var workspacePattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ObjectKey(workspace, logical string) (string, error) {
	if !workspacePattern.MatchString(workspace) || logical == "" || len(logical) > 1024 {
		return "", ErrDenied
	}
	h := sha256.Sum256([]byte(logical))
	return workspace + "/" + hex.EncodeToString(h[:]), nil
}
func validKey(k string) bool {
	parts := strings.Split(k, "/")
	if len(parts) != 2 || !workspacePattern.MatchString(parts[0]) || len(parts[1]) != 64 {
		return false
	}
	_, e := hex.DecodeString(parts[1])
	return e == nil
}

// Scoped is the only boundary domains/API callers should receive. The workspace
// argument must come from membership-authorized request context, never an object.
type Scoped struct{ Provider StorageProvider }

func (s Scoped) check(workspace, key string) error {
	if !validKey(key) || !strings.HasPrefix(key, workspace+"/") {
		return ErrDenied
	}
	return nil
}
func (s Scoped) Put(ctx context.Context, workspace, key string, r io.Reader, m Metadata) error {
	if e := s.check(workspace, key); e != nil {
		return e
	}
	return s.Provider.Put(ctx, key, r, m)
}
func (s Scoped) Open(ctx context.Context, workspace, key string) (io.ReadCloser, error) {
	if e := s.check(workspace, key); e != nil {
		return nil, e
	}
	return s.Provider.Get(ctx, key)
}
func (s Scoped) Delete(ctx context.Context, workspace, key string) error {
	if e := s.check(workspace, key); e != nil {
		return e
	}
	return s.Provider.Delete(ctx, key)
}
func (s Scoped) SignedURL(ctx context.Context, workspace, key string, ttl time.Duration) (string, error) {
	if e := s.check(workspace, key); e != nil {
		return "", e
	}
	return s.Provider.SignedURL(ctx, key, ttl)
}
func (s Scoped) Metadata(ctx context.Context, workspace, key string) (Metadata, error) {
	if e := s.check(workspace, key); e != nil {
		return Metadata{}, e
	}
	return s.Provider.Metadata(ctx, key)
}

// S3Compatible is a private-bucket transport contract. An adapter must honor the
// same object/metadata semantics; no credentials/public URLs are in this contract.
type S3Compatible interface {
	StorageProvider
	PrivateBucket() bool
}

func NewS3Scoped(p S3Compatible) (Scoped, error) {
	if p == nil || !p.PrivateBucket() {
		return Scoped{}, ErrDenied
	}
	return Scoped{p}, nil
}

type Local struct {
	root     *os.Root
	mu       sync.Mutex
	MaxBytes int64
}

func NewLocal(path string, maxBytes int64) (*Local, error) {
	if maxBytes <= 0 {
		return nil, ErrDenied
	}
	if e := os.MkdirAll(path, 0700); e != nil {
		return nil, ErrUnavailable
	}
	r, e := os.OpenRoot(path)
	if e != nil {
		return nil, ErrUnavailable
	}
	return &Local{root: r, MaxBytes: maxBytes}, nil
}
func (s *Local) Close() error { return s.root.Close() }
func (s *Local) Put(ctx context.Context, key string, r io.Reader, m Metadata) error {
	if !validKey(key) || m.Size < 0 || m.Size > s.MaxBytes || len(m.ContentType) > 256 {
		return ErrDenied
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	data, e := io.ReadAll(io.LimitReader(r, s.MaxBytes+1))
	if e != nil || int64(len(data)) != m.Size || int64(len(data)) > s.MaxBytes {
		return ErrUnavailable
	}
	h := sha256.Sum256(data)
	m.ETag = hex.EncodeToString(h[:])
	b, e := json.Marshal(m)
	if e != nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = s.root.MkdirAll(strings.Split(key, "/")[0], 0700); e != nil {
		return ErrUnavailable
	}
	f, e := s.root.OpenFile(key+".pending", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return ErrUnavailable
	}
	defer s.root.Remove(key + ".pending")
	e = binary.Write(f, binary.BigEndian, uint32(len(b)))
	if e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return ErrUnavailable
	}
	if e = s.root.Rename(key+".pending", key); e != nil {
		return ErrUnavailable
	}
	slog.Info("storage operation", "operation", "put", "outcome", "success")
	return nil
}
func (s *Local) open(key string) (*os.File, Metadata, error) {
	if !validKey(key) {
		return nil, Metadata{}, ErrDenied
	}
	f, e := s.root.Open(key)
	if e != nil {
		return nil, Metadata{}, ErrUnavailable
	}
	fail := func() (*os.File, Metadata, error) { f.Close(); return nil, Metadata{}, ErrUnavailable }
	var n uint32
	if binary.Read(f, binary.BigEndian, &n) != nil || n > 4096 {
		return fail()
	}
	b := make([]byte, n)
	if _, e = io.ReadFull(f, b); e != nil {
		return fail()
	}
	var m Metadata
	if json.Unmarshal(b, &m) != nil {
		return fail()
	}
	if !m.ExpiresAt.IsZero() && !time.Now().Before(m.ExpiresAt) {
		f.Close()
		return nil, Metadata{}, ErrDenied
	}
	return f, m, nil
}
func (s *Local) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	f, _, e := s.open(key)
	slog.Info("storage operation", "operation", "get", "outcome", outcome(e))
	return f, e
}
func (s *Local) Metadata(ctx context.Context, key string) (Metadata, error) {
	if e := ctx.Err(); e != nil {
		return Metadata{}, e
	}
	f, m, e := s.open(key)
	if f != nil {
		f.Close()
	}
	return m, e
}
func (s *Local) Delete(ctx context.Context, key string) error {
	if !validKey(key) {
		return ErrDenied
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.root.Remove(key)
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	if e != nil {
		return ErrUnavailable
	}
	slog.Info("storage operation", "operation", "delete", "outcome", "success")
	return nil
}
func (s *Local) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", ErrDenied
}
func (s *Local) Cleanup(ctx context.Context) error {
	return fs.WalkDir(s.root.FS(), ".", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return ErrUnavailable
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() || !validKey(path) {
			return nil
		}
		f, _, e := s.open(path)
		if f != nil {
			f.Close()
		}
		if errors.Is(e, ErrDenied) {
			return s.Delete(ctx, path)
		}
		return e
	})
}
func outcome(e error) string {
	if e != nil {
		return "denied_or_failed"
	}
	return "success"
}
