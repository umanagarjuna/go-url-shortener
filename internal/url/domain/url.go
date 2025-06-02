package domain

import (
	"time"
)

// URL represents a shortened URL entity
type URL struct {
	ID          int64      `json:"id" db:"id"`
	ShortCode   string     `json:"short_code" db:"short_code"`
	OriginalURL string     `json:"original_url" db:"original_url"`
	UserID      *int64     `json:"user_id,omitempty" db:"user_id"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	ClickCount  int64      `json:"click_count" db:"click_count"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	Metadata    JSONB      `json:"metadata,omitempty" db:"metadata"`
}

// JSONB handles JSON data for PostgreSQL
type JSONB map[string]interface{}

// CreateURLRequest represents the request to create a new URL
type CreateURLRequest struct {
	URL       string            `json:"url" validate:"required,url"`
	ExpiresIn *int64            `json:"expires_in,omitempty"` // seconds
	UserID    *int64            `json:"user_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// URLResponse represents the API response for URL operations
type URLResponse struct {
	ShortCode   string     `json:"short_code"`
	ShortURL    string     `json:"short_url"`
	OriginalURL string     `json:"original_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ClickCount  int64      `json:"click_count"`
}

// ClickEvent represents a URL click event for analytics
type ClickEvent struct {
	ShortCode string    `json:"short_code"`
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Referrer  string    `json:"referrer"`
}
