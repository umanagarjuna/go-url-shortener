package repository

import (
	"context"
	"database/sql"
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

func (r *PostgresRepository) GetByShortCode(ctx context.Context,
	shortCode string) (*domain.URL, error) {

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

func (r *PostgresRepository) IncrementClickCount(ctx context.Context,
	shortCode string) error {

	query := `
        UPDATE urls 
        SET click_count = click_count + 1 
        WHERE short_code = $1`

	_, err := r.db.ExecContext(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment click count: %w", err)
	}

	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, shortCode string,
	userID *int64) error {

	query := `
        UPDATE urls 
        SET is_active = false 
        WHERE short_code = $1`

	args := []interface{}{shortCode}

	if userID != nil {
		query += " AND user_id = $2"
		args = append(args, *userID)
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("URL not found or unauthorized")
	}

	return nil
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, userID int64,
	limit, offset int) ([]*domain.URL, error) {

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

// Migration SQL
const CreateTableSQL = `
CREATE TABLE IF NOT EXISTS urls (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    user_id BIGINT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    click_count BIGINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB,
    
    INDEX idx_short_code (short_code),
    INDEX idx_user_id (user_id),
    INDEX idx_created_at (created_at)
);

-- Partitioning by creation date for better performance
CREATE TABLE urls_2025_01 PARTITION OF urls
FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
`
