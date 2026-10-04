package storage

import (
	"context"
	"io"
	"time"
)

type Metadata struct {
	ExpiresAt         time.Time
	Size              int64
	ContentType, ETag string
}
type StorageProvider interface {
	Put(context.Context, string, io.Reader, Metadata) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
	Metadata(context.Context, string) (Metadata, error)
	SignedURL(context.Context, string, time.Duration) (string, error)
}
