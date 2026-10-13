// Package messagebus contains transport adapters without application models.
package messagebus

import (
	"context"
	"errors"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// TopicConsumer owns one server-named exclusive queue for a service instance.
// The caller restores its desired bindings when replacing a disconnected consumer.
type TopicConsumer struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	queue      string
	deliveries <-chan amqp.Delivery
}

func ValidateURL(rawURL string) error {
	_, err := amqp.ParseURI(rawURL)
	return err
}

func OpenTopicConsumer(ctx context.Context, rawURL, exchange string) (*TopicConsumer, error) {
	conn, err := amqp.DialConfig(rawURL, amqp.Config{
		Heartbeat: 5 * time.Second,
		Dial: func(network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 3 * time.Second}
			conn, err := dialer.DialContext(ctx, network, address)
			if err == nil {
				// Includes the AMQP/TLS handshake, not just TCP connection setup.
				err = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if err != nil {
					_ = conn.Close()
				}
			}
			return conn, err
		},
	})
	if err != nil {
		// Dial errors can contain credentials from the URL; never propagate them.
		return nil, errors.New("connect RabbitMQ failed")
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	c := &TopicConsumer{connection: conn, channel: channel, exchange: exchange}
	if err := channel.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		_ = c.Close()
		return nil, err
	}
	queue, err := channel.QueueDeclare("", false, true, true, false, amqp.Table{
		"x-max-length": int32(10000), "x-message-ttl": int32(30000), "x-queue-type": "classic",
	})
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	c.queue = queue.Name
	if err := channel.Qos(256, 0, false); err != nil {
		_ = c.Close()
		return nil, err
	}
	c.deliveries, err = channel.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *TopicConsumer) Bind(key string) error {
	return c.channel.QueueBind(c.queue, key, c.exchange, false, nil)
}

func (c *TopicConsumer) Unbind(key string) error {
	return c.channel.QueueUnbind(c.queue, key, c.exchange, nil)
}

// Receive delivers to the local broadcaster before ACK. Invalid frames are also
// ACKed by the caller's callback: these ephemeral hints never need redelivery.
func (c *TopicConsumer) Receive(ctx context.Context, deliver func([]byte)) error {
	select {
	case frame, ok := <-c.deliveries:
		if !ok {
			return errors.New("RabbitMQ consumer closed")
		}
		deliver(frame.Body)
		return frame.Ack(false)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *TopicConsumer) Close() error { return c.connection.Close() }
