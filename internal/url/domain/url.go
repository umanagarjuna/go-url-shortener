package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// URL represents a shortened URL entity
type URL struct {
	ID          int64      `json:"id" db:"id"`
	ShortCode   string     `json:"short_code" db:"short_code"`
	OriginalURL string     `json:"original_url" db:"original_url"`
	UserID      int64      `json:"user_id" db:"user_id"` // NOT pointer - matches schema
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at" db:"expires_at"` // Pointer - nullable in schema
	ClickCount  int64      `json:"click_count" db:"click_count"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	Metadata    JSONB      `json:"metadata" db:"metadata"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"` // NOT pointer - matches schema
	DeletedAt   time.Time  `json:"deleted_at" db:"deleted_at"`
}

// JSONB handles JSON data for PostgreSQL
type JSONB map[string]interface{}

// Value implements driver.Valuer interface for database storage
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface for database retrieval
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONB", value)
	}

	return json.Unmarshal(bytes, j)
}

// CreateURLRequest represents the request to create a new URL
type CreateURLRequest struct {
	URL       string                 `json:"url" binding:"required,url"`
	UserID    int64                  `json:"user_id" binding:"required"`
	ExpiresIn *int                   `json:"expires_in,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
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
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	Referrer  string    `json:"referrer,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
