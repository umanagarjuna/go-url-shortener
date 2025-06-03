package service

import (
	"context"
	"fmt"
	"github.com/umanagarjuna/go-url-shortener/internal/url/metrics"
	"strings"
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
	repo      repository.Repository // FIXED: Use interface instead of concrete type
	cache     *cache.RedisCache
	generator shortcode.Generator
	validator validator.URLValidator
	publisher *events.EventPublisher
	logger    *zap.Logger
	metrics   metrics.Metrics
	baseURL   string
}

type Config struct {
	BaseURL string
}

func NewURLService(
	repo repository.Repository,
	cache *cache.RedisCache,
	generator shortcode.Generator,
	validator validator.URLValidator,
	publisher *events.EventPublisher,
	logger *zap.Logger,
	metrics metrics.Metrics, // NEW
	config Config,
) *URLService {
	return &URLService{
		repo:      repo,
		cache:     cache,
		generator: generator,
		validator: validator,
		publisher: publisher,
		logger:    logger,
		metrics:   metrics, // NEW
		baseURL:   config.BaseURL,
	}
}

func (s *URLService) CreateURL(ctx context.Context, req *domain.CreateURLRequest) (*domain.URLResponse, error) {
	start := time.Now()
	defer func() {
		s.metrics.RecordDuration("url_create_duration", time.Since(start))
	}()

	s.metrics.IncrementCounter("url_create_requests_total")
	// 1. Check response cache first
	cacheKey := cache.GenerateResponseCacheKey(req.URL, req.UserID)
	if cachedResponse, err := s.cache.GetResponse(ctx, cacheKey); err == nil && cachedResponse != nil {
		s.logger.Debug("Returning cached response",
			zap.String("url", req.URL),
			zap.Int64("user_id", req.UserID),
			zap.String("short_code", cachedResponse.ShortCode))
		return cachedResponse, nil
	}

	// 2. Validate URL
	if err := s.validator.Validate(req.URL); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	// 3. Check if URL is safe
	safe, err := s.validator.IsSafe(req.URL)
	if err != nil {
		s.logger.Error("Failed to check URL safety",
			zap.Error(err), zap.String("url", req.URL))
	}
	if !safe {
		return nil, fmt.Errorf("URL is not safe")
	}

	// 4. Check for existing URL with detailed logging
	s.logger.Info("Checking for existing URL for user",
		zap.String("url", req.URL),
		zap.Int64("user_id", req.UserID))

	existingURL, err := s.repo.GetByOriginalURLAndUser(ctx, req.URL, req.UserID)
	if err != nil {
		s.metrics.IncrementCounter("url_create_errors_total")
		s.logger.Error("Failed to check existing URL",
			zap.Error(err),
			zap.String("url", req.URL),
			zap.Int64("user_id", req.UserID))
		return nil, fmt.Errorf("cannot verify existing URLs: %w", err)
	}

	var response *domain.URLResponse

	if existingURL != nil {
		s.metrics.IncrementCounter("url_duplicates_prevented_total")
		s.logger.Info("Found existing URL for user - returning same short code",
			zap.String("original_url", req.URL),
			zap.String("existing_short_code", existingURL.ShortCode),
			zap.Int64("user_id", req.UserID),
			zap.Int64("existing_id", existingURL.ID),
			zap.Time("created_at", existingURL.CreatedAt),
			zap.Any("expires_at", existingURL.ExpiresAt))

		response = s.buildURLResponse(existingURL)
	} else {
		// 5. No existing URL - create new one
		s.metrics.IncrementCounter("url_create_new_total")
		s.logger.Info("No existing URL found for user - creating new one",
			zap.String("url", req.URL),
			zap.Int64("user_id", req.UserID))

		response, err = s.createNewURLWithRetry(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	// 6. Cache the response for future requests
	if err := s.cache.SetResponse(ctx, cacheKey, response, 5*time.Minute); err != nil {
		s.logger.Warn("Failed to cache response",
			zap.Error(err),
			zap.String("cache_key", cacheKey))
	}

	return response, nil
}

func (s *URLService) createNewURLWithRetry(ctx context.Context, req *domain.CreateURLRequest) (*domain.URLResponse, error) {
	maxRetries := 5

	for attempt := 1; attempt <= maxRetries; attempt++ {
		url, err := s.attemptCreateURL(ctx, req)
		if err != nil {
			// Handle duplicate short code error
			if isDuplicateShortCodeError(err) {
				s.logger.Warn("Duplicate short code detected, retrying",
					zap.Int("attempt", attempt),
					zap.Int("max_retries", maxRetries),
					zap.Error(err))

				if attempt == maxRetries {
					// Last attempt failed - try to find existing URL as fallback
					return s.handleDuplicateErrorFallback(ctx, req)
				}
				continue // Retry with new short code
			}

			// Other errors (validation, database, etc.)
			return nil, fmt.Errorf("failed to create URL on attempt %d: %w", attempt, err)
		}

		// Success!
		s.logger.Info("Successfully created new URL",
			zap.String("short_code", url.ShortCode),
			zap.String("original_url", req.URL),
			zap.Int64("user_id", req.UserID),
			zap.Int("attempt", attempt))

		return url, nil
	}

	return nil, fmt.Errorf("unexpected error: should not reach here")
}

func (s *URLService) attemptCreateURL(ctx context.Context, req *domain.CreateURLRequest) (*domain.URLResponse, error) {
	// Generate unique short code
	shortCode, err := s.generateUniqueShortCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate short code: %w", err)
	}

	// Create URL entity
	url := &domain.URL{
		ShortCode:   shortCode,
		OriginalURL: req.URL,
		UserID:      req.UserID, // FIXED: Direct assignment (not pointer)
		IsActive:    true,
		ClickCount:  0,
	}

	// Set expiration if provided
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		url.ExpiresAt = &expiresAt
	}

	// Set metadata if provided
	if req.Metadata != nil && len(req.Metadata) > 0 {
		url.Metadata = make(domain.JSONB)
		for key, value := range req.Metadata {
			url.Metadata[key] = value
		}
	}

	// Save to database
	if err := s.repo.Create(ctx, url); err != nil {
		return nil, err
	}

	// Cache and publish events (non-blocking)
	if err := s.cache.Set(ctx, url); err != nil {
		s.logger.Warn("Failed to cache URL", zap.Error(err))
	}

	if err := s.publisher.PublishURLCreated(ctx, url); err != nil {
		s.logger.Error("Failed to publish URL created event", zap.Error(err))
	}

	return s.buildURLResponse(url), nil
}

func (s *URLService) handleDuplicateErrorFallback(ctx context.Context, req *domain.CreateURLRequest) (*domain.URLResponse, error) {
	s.logger.Warn("All retry attempts failed, checking for existing URL as fallback",
		zap.String("url", req.URL),
		zap.Int64("user_id", req.UserID))

	// Try to find existing URL one more time
	existingURL, err := s.repo.GetByOriginalURLAndUser(ctx, req.URL, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to create URL after retries and failed to find existing URL: %w", err)
	}

	if existingURL != nil {
		s.logger.Info("Found existing URL during fallback",
			zap.String("short_code", existingURL.ShortCode))
		return s.buildURLResponse(existingURL), nil
	}

	return nil, fmt.Errorf("failed to create URL after %d retry attempts", 5)
}

func (s *URLService) generateUniqueShortCode(ctx context.Context) (string, error) {
	maxRetries := 10

	for i := 0; i < maxRetries; i++ {
		shortCode, err := s.generator.Generate()
		if err != nil {
			return "", fmt.Errorf("failed to generate short code: %w", err)
		}

		// Check if short code already exists
		existing, err := s.repo.GetByShortCode(ctx, shortCode)
		if err != nil {
			s.logger.Error("Failed to check short code uniqueness",
				zap.Error(err), zap.String("short_code", shortCode))
			return "", fmt.Errorf("failed to check short code uniqueness: %w", err)
		}

		if existing == nil {
			return shortCode, nil // Short code is unique
		}

		s.logger.Debug("Generated duplicate short code, retrying",
			zap.String("short_code", shortCode),
			zap.Int("attempt", i+1))
	}

	return "", fmt.Errorf("failed to generate unique short code after %d attempts", maxRetries)
}

func (s *URLService) buildURLResponse(url *domain.URL) *domain.URLResponse {
	return &domain.URLResponse{
		ShortCode:   url.ShortCode,
		ShortURL:    fmt.Sprintf("%s/%s", s.baseURL, url.ShortCode),
		OriginalURL: url.OriginalURL,
		CreatedAt:   url.CreatedAt,
		ExpiresAt:   url.ExpiresAt,
		ClickCount:  url.ClickCount,
	}
}

// Helper functions
func isDuplicateShortCodeError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "duplicate key value violates unique constraint") &&
		strings.Contains(errMsg, "urls_short_code_key")
}

// Additional helper for user-specific duplicate URL check (if needed)
func isDuplicateUserURLError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "duplicate key value violates unique constraint") &&
		(strings.Contains(errMsg, "urls_user_url_key") ||
			strings.Contains(errMsg, "idx_urls_user_url"))
}

func (s *URLService) GetURL(ctx context.Context, shortCode string) (*domain.URLResponse, error) {
	// Try cache first
	url, err := s.cache.Get(ctx, shortCode)
	if err != nil {
		s.logger.Warn("Failed to get URL from cache",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	// If not in cache, get from database
	if url == nil {
		url, err = s.repo.GetByShortCode(ctx, shortCode)
		if err != nil {
			return nil, fmt.Errorf("failed to get URL from repository: %w", err)
		}
		if url == nil {
			return nil, nil // URL not found
		}

		// Cache for future requests
		if err := s.cache.Set(ctx, url); err != nil {
			s.logger.Warn("Failed to cache URL",
				zap.Error(err), zap.String("short_code", shortCode))
		}
	}

	// Check if URL is active
	if !url.IsActive {
		return nil, nil
	}

	// Check if URL has expired
	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		return nil, nil
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

// FIXED: Remove userID parameter to match interface
func (s *URLService) DeleteURL(ctx context.Context, shortCode string) error {
	// Delete from database
	if err := s.repo.Delete(ctx, shortCode); err != nil {
		return err
	}

	// Remove from cache
	if err := s.cache.Delete(ctx, shortCode); err != nil {
		s.logger.Warn("Failed to delete from cache",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	return nil
}

// FIXED: Use correct repository method name
func (s *URLService) GetUserURLs(ctx context.Context, userID int64,
	limit, offset int) ([]*domain.URLResponse, error) {

	urls, err := s.repo.GetUserURLs(ctx, userID, limit, offset) // FIXED: GetUserURLs instead of GetByUserID
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

func (s *URLService) GetURLAndIncrementClick(ctx context.Context, shortCode, userAgent, clientIP, referrer string) (*domain.URL, error) {
	// Try to get from cache first
	url, err := s.cache.Get(ctx, shortCode)
	if err != nil {
		s.logger.Warn("Failed to get URL from cache",
			zap.Error(err), zap.String("short_code", shortCode))
	}

	// If not in cache, get from database
	if url == nil {
		url, err = s.repo.GetByShortCode(ctx, shortCode)
		if err != nil {
			return nil, fmt.Errorf("failed to get URL from database: %w", err)
		}
		if url == nil {
			return nil, nil // URL not found
		}

		// Cache the URL for future requests
		if err := s.cache.Set(ctx, url); err != nil {
			s.logger.Warn("Failed to cache URL",
				zap.Error(err), zap.String("short_code", shortCode))
		}
	}

	// Check if URL is active
	if !url.IsActive {
		s.logger.Info("URL is not active", zap.String("short_code", shortCode))
		return nil, nil
	}

	// Check if URL has expired
	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		s.logger.Info("URL has expired",
			zap.String("short_code", shortCode),
			zap.Time("expires_at", *url.ExpiresAt))
		return nil, nil
	}

	// Increment click count and publish event (async to not slow down redirect)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Increment click count in database
		if err := s.repo.IncrementClickCount(ctx, shortCode); err != nil {
			s.logger.Error("Failed to increment click count",
				zap.Error(err), zap.String("short_code", shortCode))
		} else {
			s.logger.Debug("Successfully incremented click count",
				zap.String("short_code", shortCode))
		}

		// Publish click event to Kafka
		clickEvent := &domain.ClickEvent{
			ShortCode: shortCode,
			UserAgent: userAgent,
			IPAddress: clientIP,
			Referrer:  referrer,
			Timestamp: time.Now(),
		}

		if err := s.publisher.PublishURLClicked(ctx, clickEvent); err != nil {
			s.logger.Error("Failed to publish URL clicked event",
				zap.Error(err), zap.String("short_code", shortCode))
		} else {
			s.logger.Debug("Successfully published URL clicked event",
				zap.String("short_code", shortCode))
		}

		// Update cache with incremented count (for consistency)
		updatedURL := *url
		updatedURL.ClickCount++
		if err := s.cache.Set(ctx, &updatedURL); err != nil {
			s.logger.Warn("Failed to update cache with new click count",
				zap.Error(err), zap.String("short_code", shortCode))
		} else {
			s.logger.Debug("Updated cache with new click count",
				zap.String("short_code", shortCode),
				zap.Int64("new_count", updatedURL.ClickCount))
		}
	}()

	return url, nil
}
