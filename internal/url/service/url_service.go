package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/umanagarjuna/go-url-shortener/internal/url/cache"
	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
	"github.com/umanagarjuna/go-url-shortener/internal/url/events"
	"github.com/umanagarjuna/go-url-shortener/internal/url/repository"
	"github.com/umanagarjuna/go-url-shortener/pkg/shortcode"
	"github.com/umanagarjuna/go-url-shortener/pkg/validator"
)

type URLService struct {
	repo      *repository.PostgresRepository
	cache     *cache.RedisCache
	generator shortcode.Generator
	validator validator.URLValidator
	publisher *events.EventPublisher
	logger    *zap.Logger
	baseURL   string
}

type Config struct {
	BaseURL string
}

func NewURLService(
	repo *repository.PostgresRepository,
	cache *cache.RedisCache,
	generator shortcode.Generator,
	validator validator.URLValidator,
	publisher *events.EventPublisher,
	logger *zap.Logger,
	config Config,
) *URLService {
	return &URLService{
		repo:      repo,
		cache:     cache,
		generator: generator,
		validator: validator,
		publisher: publisher,
		logger:    logger,
		baseURL:   config.BaseURL,
	}
}

func (s *URLService) CreateURL(ctx context.Context,
	req *domain.CreateURLRequest) (*domain.URLResponse, error) {

	// Validate URL
	if err := s.validator.Validate(req.URL); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// Check if URL is safe
	safe, err := s.validator.IsSafe(req.URL)
	if err != nil {
		s.logger.Error("Failed to check URL safety",
			zap.Error(err), zap.String("url", req.URL))
	}
	if !safe {
		return nil, fmt.Errorf("URL is not safe")
	}

	// Generate short code
	shortCode, err := s.generateUniqueShortCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate short code: %w", err)
	}

	// Create URL entity
	url := &domain.URL{
		ShortCode:   shortCode,
		OriginalURL: req.URL,
		UserID:      req.UserID,
		IsActive:    true,
	}

	if req.ExpiresIn != nil {
		expiresAt := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		url.ExpiresAt = &expiresAt
	}

	if req.Metadata != nil && len(req.Metadata) > 0 {
		convertedMetadata := make(domain.JSONB, len(req.Metadata))
		// Iterate over the original map[string]string and assign values
		// to the new map[string]interface{}
		for key, value := range req.Metadata {
			convertedMetadata[key] = value // A string value can be assigned to an interface{} type
		}
		url.Metadata = convertedMetadata
	}

	// Save to database
	if err := s.repo.Create(ctx, url); err != nil {
		return nil, fmt.Errorf("failed to save URL: %w", err)
	}

	// Cache the URL
	if err := s.cache.Set(ctx, url); err != nil {
		s.logger.Warn("Failed to cache URL",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	// Publish event
	if err := s.publisher.PublishURLCreated(ctx, url); err != nil {
		s.logger.Error("Failed to publish URL created event",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	return &domain.URLResponse{
		ShortCode:   url.ShortCode,
		ShortURL:    fmt.Sprintf("%s/%s", s.baseURL, url.ShortCode),
		OriginalURL: url.OriginalURL,
		CreatedAt:   url.CreatedAt,
		ExpiresAt:   url.ExpiresAt,
		ClickCount:  url.ClickCount,
	}, nil
}

func (s *URLService) GetURL(ctx context.Context, shortCode string) (
	*domain.URLResponse, error) {

	// Try cache first
	url, err := s.cache.Get(ctx, shortCode)
	if err != nil {
		s.logger.Warn("Cache get failed",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	// If not in cache, get from database
	if url == nil {
		url, err = s.repo.GetByShortCode(ctx, shortCode)
		if err != nil {
			return nil, fmt.Errorf("failed to get URL: %w", err)
		}
		if url == nil {
			return nil, nil
		}

		// Update cache
		if err := s.cache.Set(ctx, url); err != nil {
			s.logger.Warn("Failed to update cache",
				zap.Error(err), zap.String("short_code", shortCode))
		}
	}

	return &domain.URLResponse{
		ShortCode:   url.ShortCode,
		ShortURL:    fmt.Sprintf("%s/%s", s.baseURL, url.ShortCode),
		OriginalURL: url.OriginalURL,
		CreatedAt:   url.CreatedAt,
		ExpiresAt:   url.ExpiresAt,
		ClickCount:  url.ClickCount,
	}, nil
}

func (s *URLService) RedirectURL(ctx context.Context, shortCode string,
	clickEvent *domain.ClickEvent) (string, error) {

	// Get URL
	urlResp, err := s.GetURL(ctx, shortCode)
	if err != nil {
		return "", err
	}
	if urlResp == nil {
		return "", fmt.Errorf("URL not found")
	}

	// Increment click count asynchronously
	go func() {
		ctx := context.Background()
		if err := s.repo.IncrementClickCount(ctx, shortCode); err != nil {
			s.logger.Error("Failed to increment click count",
				zap.Error(err), zap.String("short_code", shortCode))
		}

		// Publish click event
		clickEvent.ShortCode = shortCode
		clickEvent.Timestamp = time.Now()
		if err := s.publisher.PublishURLClicked(ctx, clickEvent); err != nil {
			s.logger.Error("Failed to publish click event",
				zap.Error(err), zap.String("short_code", shortCode))
		}
	}()

	return urlResp.OriginalURL, nil
}

func (s *URLService) DeleteURL(ctx context.Context, shortCode string,
	userID *int64) error {

	// Delete from database
	if err := s.repo.Delete(ctx, shortCode, userID); err != nil {
		return err
	}

	// Remove from cache
	if err := s.cache.Delete(ctx, shortCode); err != nil {
		s.logger.Warn("Failed to delete from cache",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	return nil
}

func (s *URLService) GetUserURLs(ctx context.Context, userID int64,
	limit, offset int) ([]*domain.URLResponse, error) {

	urls, err := s.repo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]*domain.URLResponse, len(urls))
	for i, url := range urls {
		responses[i] = &domain.URLResponse{
			ShortCode:   url.ShortCode,
			ShortURL:    fmt.Sprintf("%s/%s", s.baseURL, url.ShortCode),
			OriginalURL: url.OriginalURL,
			CreatedAt:   url.CreatedAt,
			ExpiresAt:   url.ExpiresAt,
			ClickCount:  url.ClickCount,
		}
	}

	return responses, nil
}

func (s *URLService) generateUniqueShortCode(ctx context.Context) (
	string, error) {

	for i := 0; i < 10; i++ {
		code, err := s.generator.Generate()
		if err != nil {
			return "", err
		}

		// Check if code already exists
		existing, err := s.repo.GetByShortCode(ctx, code)
		if err != nil {
			return "", err
		}

		if existing == nil {
			return code, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique short code")
}
