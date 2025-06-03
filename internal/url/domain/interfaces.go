package domain

import "context"

// EventPublisher interface for publishing domain events
type EventPublisher interface {
	PublishURLCreated(ctx context.Context, url *URL) error
	PublishURLUpdated(ctx context.Context, url *URL, updatedFields []string) error
	PublishURLClicked(ctx context.Context, event *ClickEvent) error
	Close() error
}
