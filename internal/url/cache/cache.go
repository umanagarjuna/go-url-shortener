package cache

import (
	"context"
	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
	"time"
)

type Cache interface {
	// URL caching
	Get(ctx context.Context, shortCode string) (*domain.URL, error)
	Set(ctx context.Context, url *domain.URL) error
	Delete(ctx context.Context, shortCode string) error
	Invalidate(ctx context.Context, pattern string) error

	// Response caching (NEW)
	GetResponse(ctx context.Context, key string) (*domain.URLResponse, error)
	SetResponse(ctx context.Context, key string, response *domain.URLResponse, ttl time.Duration) error
	DeleteResponse(ctx context.Context, key string) error
}
