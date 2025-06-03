package repository

import (
	"context"
	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
)

type Repository interface {
	Create(ctx context.Context, url *domain.URL) error
	GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error)
	GetByOriginalURLAndUser(ctx context.Context, originalURL string, userID int64) (*domain.URL, error)
	Update(ctx context.Context, url *domain.URL) error
	GetUserURLs(ctx context.Context, userID int64, limit, offset int) ([]*domain.URL, error)
	Delete(ctx context.Context, shortCode string) error
	IncrementClickCount(ctx context.Context, shortCode string) error
}
