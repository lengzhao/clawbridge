package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LocalBackend stores files under Root; Put returns absolute paths as locators.
type LocalBackend struct {
	Root string

	mu      sync.Mutex
	byScope map[string][]string // scope -> file paths created under this backend
}

// NewLocalBackend creates a local filesystem backend.
// If root is empty or only whitespace, Root becomes filepath.Join(os.TempDir(), "clawbridge").
func NewLocalBackend(root string) (*LocalBackend, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(os.TempDir(), "clawbridge")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("media local: mkdir root: %w", err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("media local: abs root: %w", err)
	}
	return &LocalBackend{Root: abs, byScope: make(map[string][]string)}, nil
}

// Put writes data to root/scope/safeName and records the path for RemoveScope.
func (l *LocalBackend) Put(ctx context.Context, scope, name string, r io.Reader, size int64, contentType string) (string, error) {
	_ = size
	_ = contentType
	if scope == "" {
		scope = "_"
	}
	base := filepath.Join(l.Root, sanitizeScope(scope))
	safe := filepath.Base(name)
	if safe == "" || safe == "." {
		safe = "file"
	}
	if err := os.MkdirAll(base, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(base, safe)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	l.mu.Lock()
	l.byScope[scope] = append(l.byScope[scope], abs)
	l.mu.Unlock()
	return abs, nil
}

// Open opens a local file by absolute or relative path; relative paths join with Root.
func (l *LocalBackend) Open(ctx context.Context, loc string) (io.ReadCloser, error) {
	_ = ctx
	if loc == "" {
		return nil, fmt.Errorf("media local: empty locator")
	}
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		return nil, fmt.Errorf("media local: http(s) locator not supported")
	}
	path := loc
	if !filepath.IsAbs(loc) {
		path = filepath.Join(l.Root, loc)
	}
	return os.Open(path)
}

// RemoveScope deletes files recorded for scope (best-effort).
func (l *LocalBackend) RemoveScope(ctx context.Context, scope string) error {
	_ = ctx
	l.mu.Lock()
	paths := l.byScope[scope]
	delete(l.byScope, scope)
	l.mu.Unlock()
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return nil
}
