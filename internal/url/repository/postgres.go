package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
)

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, url *domain.URL) error {
	query := `
        INSERT INTO urls (short_code, original_url, user_id, expires_at, 
                         is_active, metadata)
        VALUES (:short_code, :original_url, :user_id, :expires_at, 
                :is_active, :metadata)
        RETURNING id, created_at`

	rows, err := r.db.NamedQueryContext(ctx, query, url)
	if err != nil {
		return fmt.Errorf("failed to insert URL: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(&url.ID, &url.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to scan returning values: %w", err)
		}
	}

	return nil
}

func (r *PostgresRepository) GetByOriginalURLAndUser(ctx context.Context,
	originalURL string, userID int64) (*domain.URL, error) {

	var url domain.URL
	query := `
        SELECT id, short_code, original_url, user_id, created_at, 
               expires_at, click_count, is_active, metadata, updated_at
        FROM urls
        WHERE original_url = $1 
          AND user_id = $2 
          AND deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT 1`

	// Remove all these debug logs for production:
	// log.Printf("DEBUG: Executing query for originalURL=%s, userID=%d", originalURL, userID)
	// log.Printf("DEBUG: Query = %s", query)

	err := r.db.GetContext(ctx, &url, query, originalURL, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			// log.Printf("DEBUG: No rows found for originalURL=%s, userID=%d", originalURL, userID)
			return nil, nil
		}
		// log.Printf("DEBUG: Database error: %v", err)
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	// log.Printf("DEBUG: Found existing URL - ID=%d, ShortCode=%s, ExpiresAt=%v", url.ID, url.ShortCode, url.ExpiresAt)

	// Check expiration in application and soft delete if expired
	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		// log.Printf("DEBUG: URL %s is expired, soft deleting", url.ShortCode)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r.SoftDelete(ctx, url.ShortCode)
			// Remove: log.Printf("DEBUG: Failed to soft delete: %v", err)
			// Remove: log.Printf("DEBUG: Successfully soft deleted %s", url.ShortCode)
		}()
		return nil, nil
	}

	// log.Printf("DEBUG: Returning existing valid URL: %s", url.ShortCode)
	return &url, nil
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, shortCode string) error {
	query := `
        UPDATE urls 
        SET deleted_at = NOW(), updated_at = NOW()
        WHERE short_code = $1 AND deleted_at IS NULL`

	_, err := r.db.ExecContext(ctx, query, shortCode)
	return err
}

// FIXED: Remove the old GetByOriginalURL method or keep it if you need it for other purposes
func (r *PostgresRepository) GetByOriginalURL(ctx context.Context, originalURL string) (*domain.URL, error) {
	var url domain.URL

	query := `
		SELECT id, short_code, original_url, user_id, created_at, 
			   expires_at, click_count, is_active, metadata
		FROM urls
		WHERE original_url = $1 AND is_active = true
		ORDER BY created_at DESC
		LIMIT 1`

	err := r.db.GetContext(ctx, &url, query, originalURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // URL not found
		}
		return nil, fmt.Errorf("failed to get URL by original URL: %w", err)
	}

	// Check if URL has expired
	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		return nil, nil // URL expired, treat as not found
	}

	return &url, nil
}

func (r *PostgresRepository) GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error) {
	var url domain.URL
	query := `
        SELECT id, short_code, original_url, user_id, created_at, 
               expires_at, click_count, is_active, metadata
        FROM urls
        WHERE short_code = $1 AND is_active = true`

	err := r.db.GetContext(ctx, &url, query, shortCode)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	// Check expiration
	if url.ExpiresAt != nil && url.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}

	return &url, nil
}

func (r *PostgresRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	query := `
		UPDATE urls 
		SET click_count = click_count + 1 
		WHERE short_code = $1 AND is_active = true`

	result, err := r.db.ExecContext(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment click count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("URL with short code %s not found or not active", shortCode)
	}

	return nil
}

func (r *PostgresRepository) Update(ctx context.Context, url *domain.URL) error {
	query := `
		UPDATE urls 
		SET user_id = $1, 
			expires_at = $2, 
			metadata = $3,
			updated_at = NOW()
		WHERE short_code = $4 AND is_active = true`

	var metadataJSON []byte
	if url.Metadata != nil && len(url.Metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(url.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := r.db.ExecContext(ctx, query,
		url.UserID,
		url.ExpiresAt,
		metadataJSON,
		url.ShortCode)

	if err != nil {
		return fmt.Errorf("failed to update URL: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("URL with short code %s not found or not active", url.ShortCode)
	}

	return nil
}

// FIXED: Update Delete method signature to match interface
func (r *PostgresRepository) Delete(ctx context.Context, shortCode string) error {
	query := `
        UPDATE urls 
        SET is_active = false 
        WHERE short_code = $1`

	result, err := r.db.ExecContext(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("URL not found")
	}

	return nil
}

// FIXED: Rename method to match interface
func (r *PostgresRepository) GetUserURLs(ctx context.Context, userID int64, limit, offset int) ([]*domain.URL, error) {
	var urls []*domain.URL
	query := `
        SELECT id, short_code, original_url, user_id, created_at, 
               expires_at, click_count, is_active, metadata
        FROM urls
        WHERE user_id = $1 AND is_active = true
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &urls, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get user URLs: %w", err)
	}

	return urls, nil
}
