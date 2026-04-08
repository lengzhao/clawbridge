package media

import (
	"context"
	"io"
)

// Backend stores and retrieves media by locator string (paths, s3://, etc.).
type Backend interface {
	Put(ctx context.Context, scope, name string, r io.Reader, size int64, contentType string) (loc string, err error)
	Open(ctx context.Context, loc string) (io.ReadCloser, error)
	RemoveScope(ctx context.Context, scope string) error
}
