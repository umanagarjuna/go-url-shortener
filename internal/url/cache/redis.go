package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
)

const (
	urlPrefix  = "url:"
	defaultTTL = 24 * time.Hour
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
