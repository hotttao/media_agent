package messagebus

import (
	"context"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"sync"
	"time"
)

type TopicPublisher struct {
	mu               sync.Mutex
	rawURL, exchange string
	connection       *amqp.Connection
	channel          *amqp.Channel
}

func NewTopicPublisher(rawURL, exchange string) (*TopicPublisher, error) {
	if ValidateURL(rawURL) != nil || exchange == "" {
		return nil, errors.New("invalid RabbitMQ publisher configuration")
	}
	return &TopicPublisher{rawURL: rawURL, exchange: exchange}, nil
}
func (p *TopicPublisher) Publish(ctx context.Context, key, id string, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.connection == nil || p.connection.IsClosed() {
		conn, err := amqp.DialConfig(p.rawURL, amqp.Config{Heartbeat: 5 * time.Second, Dial: amqp.DefaultDial(3 * time.Second)})
		if err != nil {
			return errors.New("RabbitMQ publisher unavailable")
		}
		channel, err := conn.Channel()
		if err != nil {
			conn.Close()
			return err
		}
		if err := channel.ExchangeDeclare(p.exchange, "topic", true, false, false, false, nil); err != nil {
			conn.Close()
			return err
		}
		if err := channel.Confirm(false); err != nil {
			conn.Close()
			return err
		}
		p.connection, p.channel = conn, channel
	}
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, p.exchange, key, false, false, amqp.Publishing{MessageId: id, ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body})
	if err != nil {
		p.reset()
		return errors.New("RabbitMQ publish failed")
	}
	confirmed, err := confirmation.WaitContext(ctx)
	if err != nil || !confirmed {
		p.reset()
		return errors.New("RabbitMQ publish unconfirmed")
	}
	return nil
}
func (p *TopicPublisher) reset() {
	if p.connection != nil {
		_ = p.connection.Close()
	}
	p.connection = nil
	p.channel = nil
}
func (p *TopicPublisher) Close() error { p.mu.Lock(); defer p.mu.Unlock(); p.reset(); return nil }
