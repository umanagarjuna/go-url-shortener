package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"

	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
)

const (
	TopicURLCreated = "url.created"
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

func (p *EventPublisher) PublishURLClicked(ctx context.Context,
	event *domain.ClickEvent) error {

	return p.publish(TopicURLClicked, event.ShortCode, event)
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
