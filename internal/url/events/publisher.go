package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"

	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
)

const (
	TopicURLCreated = "url.created"
	TopicURLUpdated = "url.updated" // Add this line
	TopicURLClicked = "url.clicked"
	TopicURLDeleted = "url.deleted"
)

type EventPublisher struct {
	producer sarama.SyncProducer
}

func NewEventPublisher(brokers []string) (*EventPublisher, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &EventPublisher{producer: producer}, nil
}

func (p *EventPublisher) PublishURLCreated(ctx context.Context,
	url *domain.URL) error {

	event := map[string]interface{}{
		"event_type": "url_created",
		"timestamp":  url.CreatedAt,
		"data": map[string]interface{}{
			"short_code":   url.ShortCode,
			"original_url": url.OriginalURL,
			"user_id":      url.UserID,
			"expires_at":   url.ExpiresAt,
		},
	}

	return p.publish(TopicURLCreated, url.ShortCode, event)
}

// Add this new method
func (p *EventPublisher) PublishURLUpdated(ctx context.Context,
	url *domain.URL, updatedFields []string) error {

	data := map[string]interface{}{
		"short_code":     url.ShortCode,
		"original_url":   url.OriginalURL,
		"updated_fields": updatedFields,
	}
	
	data["user_id"] = url.UserID

	if url.ExpiresAt != nil {
		data["expires_at"] = url.ExpiresAt
	}
	if url.Metadata != nil && len(url.Metadata) > 0 {
		// Convert JSONB to map[string]interface{}
		metadata := make(map[string]interface{})
		for k, v := range url.Metadata {
			metadata[k] = v
		}
		data["metadata"] = metadata
	}

	event := map[string]interface{}{
		"event_type": "url_updated",
		"timestamp":  time.Now(),
		"data":       data,
	}

	return p.publish(TopicURLUpdated, url.ShortCode, event)
}

func (p *EventPublisher) PublishURLClicked(ctx context.Context,
	event *domain.ClickEvent) error {

	kafkaEvent := map[string]interface{}{
		"event_type": "url_clicked",
		"timestamp":  event.Timestamp,
		"data": map[string]interface{}{
			"short_code": event.ShortCode,
			"user_agent": event.UserAgent,
			"ip_address": event.IPAddress,
			"referrer":   event.Referrer,
		},
	}

	return p.publish(TopicURLClicked, event.ShortCode, kafkaEvent)
}

func (p *EventPublisher) publish(topic, key string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	}

	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (p *EventPublisher) Close() error {
	return p.producer.Close()
}
