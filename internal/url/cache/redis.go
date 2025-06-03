package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
)

const (
	urlPrefix      = "url:"
	responsePrefix = "response:"
	defaultTTL     = 24 * time.Hour
	responseTTL    = 5 * time.Minute // Shorter TTL for responses
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, shortCode string) (
	*domain.URL, error) {

	key := fmt.Sprintf("%s%s", urlPrefix, shortCode)

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("cache get error: %w", err)
	}

	var url domain.URL
	if err := json.Unmarshal([]byte(val), &url); err != nil {
		return nil, fmt.Errorf("cache unmarshal error: %w", err)
	}

	return &url, nil
}

func (c *RedisCache) Set(ctx context.Context, url *domain.URL) error {
	key := fmt.Sprintf("%s%s", urlPrefix, url.ShortCode)

	data, err := json.Marshal(url)
	if err != nil {
		return fmt.Errorf("cache marshal error: %w", err)
	}

	ttl := defaultTTL
	if url.ExpiresAt != nil {
		ttl = time.Until(*url.ExpiresAt)
		if ttl < 0 {
			ttl = 0
		}
	}

	err = c.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("cache set error: %w", err)
	}

	return nil
}

func (c *RedisCache) Delete(ctx context.Context, shortCode string) error {
	key := fmt.Sprintf("%s%s", urlPrefix, shortCode)

	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("cache delete error: %w", err)
	}

	return nil
}

func (c *RedisCache) Invalidate(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, fmt.Sprintf("%s%s*", urlPrefix, pattern),
		0).Iterator()

	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("cache scan error: %w", err)
	}

	if len(keys) > 0 {
		err := c.client.Del(ctx, keys...).Err()
		if err != nil {
			return fmt.Errorf("cache delete multiple error: %w", err)
		}
	}

	return nil
}

// Add these methods to RedisCache
func (c *RedisCache) GetResponse(ctx context.Context, key string) (*domain.URLResponse, error) {
	cacheKey := fmt.Sprintf("%s%s", responsePrefix, key)

	val, err := c.client.Get(ctx, cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("response cache get error: %w", err)
	}

	var response domain.URLResponse
	if err := json.Unmarshal([]byte(val), &response); err != nil {
		return nil, fmt.Errorf("response cache unmarshal error: %w", err)
	}

	return &response, nil
}

func (c *RedisCache) SetResponse(ctx context.Context, key string, response *domain.URLResponse, ttl time.Duration) error {
	cacheKey := fmt.Sprintf("%s%s", responsePrefix, key)

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("response cache marshal error: %w", err)
	}

	if ttl <= 0 {
		ttl = responseTTL
	}

	err = c.client.Set(ctx, cacheKey, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("response cache set error: %w", err)
	}

	return nil
}

func (c *RedisCache) DeleteResponse(ctx context.Context, key string) error {
	cacheKey := fmt.Sprintf("%s%s", responsePrefix, key)

	err := c.client.Del(ctx, cacheKey).Err()
	if err != nil {
		return fmt.Errorf("response cache delete error: %w", err)
	}

	return nil
}

// Helper function to generate cache keys
func GenerateResponseCacheKey(originalURL string, userID int64) string {
	data := fmt.Sprintf("%s:%d", originalURL, userID)
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (c *RedisCache) ClearResponseCache(ctx context.Context) error {
	// Get all response cache keys
	pattern := "response:*"
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get response cache keys: %w", err)
	}

	if len(keys) == 0 {
		return nil // No keys to delete
	}

	// Delete all response cache keys
	err = c.client.Del(ctx, keys...).Err()
	if err != nil {
		return fmt.Errorf("failed to delete response cache keys: %w", err)
	}

	return nil
}

func (c *RedisCache) ClearAllCache(ctx context.Context) error {
	// Clear entire Redis database (use with caution!)
	err := c.client.FlushDB(ctx).Err()
	if err != nil {
		return fmt.Errorf("failed to flush Redis database: %w", err)
	}
	return nil
}
