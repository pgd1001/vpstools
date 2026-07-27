package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const connectTimeout = 5 * time.Second

type JetStreamPublisher struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	config Config
}

func NewJetStreamPublisher(ctx context.Context, config Config) (*JetStreamPublisher, error) {
	if err := validateJetStreamConfig(config); err != nil {
		return nil, err
	}
	nc, js, err := connectJetStream(config)
	if err != nil {
		return nil, err
	}
	if err := ensureStream(ctx, js, config); err != nil {
		nc.Close()
		return nil, err
	}
	return &JetStreamPublisher{nc: nc, js: js, config: config}, nil
}

func (p *JetStreamPublisher) Publish(ctx context.Context, notification Notification) error {
	payload, err := notification.Marshal()
	if err != nil {
		return err
	}
	_, err = p.js.Publish(ctx, p.config.Subject, payload, jetstream.WithMsgID(notification.MessageID()))
	return err
}

func (p *JetStreamPublisher) Close() error {
	if p == nil || p.nc == nil {
		return nil
	}
	p.nc.Close()
	return nil
}

type JetStreamConsumer struct {
	nc       *nats.Conn
	consumer jetstream.Consumer
	config   Config
}

func NewJetStreamConsumer(ctx context.Context, config Config) (*JetStreamConsumer, error) {
	if err := validateJetStreamConfig(config); err != nil {
		return nil, err
	}
	nc, js, err := connectJetStream(config)
	if err != nil {
		return nil, err
	}
	if err := ensureStream(ctx, js, config); err != nil {
		nc.Close()
		return nil, err
	}
	consumer, err := js.CreateOrUpdateConsumer(ctx, config.Stream, jetstream.ConsumerConfig{
		Durable:       config.Durable,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       config.AckWait,
		MaxDeliver:    config.MaxDeliver,
		FilterSubject: config.Subject,
		MaxAckPending: 1,
		Description:   "VPS Tools database-authoritative job notifications",
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream durable pull consumer: %w", err)
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("inspect JetStream durable pull consumer: %w", err)
	}
	if info.Config.Durable != config.Durable || info.Config.DeliverSubject != "" || info.Config.AckPolicy != jetstream.AckExplicitPolicy || info.Config.AckWait != config.AckWait || info.Config.MaxDeliver != config.MaxDeliver || info.Config.MaxAckPending != 1 || info.Config.FilterSubject != config.Subject {
		nc.Close()
		return nil, errors.New("JetStream consumer configuration is not explicit-ack, bounded, or subject-scoped as required")
	}
	return &JetStreamConsumer{nc: nc, consumer: consumer, config: config}, nil
}

func (c *JetStreamConsumer) Next(ctx context.Context) (Delivery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wait := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait {
		wait = time.Until(deadline)
	}
	if wait <= 0 {
		return nil, ctx.Err()
	}
	msg, err := c.consumer.Next(jetstream.FetchMaxWait(wait))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) || errors.Is(err, jetstream.ErrNoMessages) {
			return nil, ErrNoMessage
		}
		return nil, err
	}
	notification, err := ParseNotification(msg.Data())
	if err != nil {
		// Malformed notifications are poison messages. Terminating them avoids
		// wasting the finite redelivery budget on data that cannot be claimed.
		_ = msg.Term()
		return nil, fmt.Errorf("invalid JetStream dispatch notification: %w", err)
	}
	return &jetStreamDelivery{msg: msg, notification: notification}, nil
}

func (c *JetStreamConsumer) Close() error {
	if c == nil || c.nc == nil {
		return nil
	}
	c.nc.Close()
	return nil
}

type jetStreamDelivery struct {
	msg          jetstream.Msg
	notification Notification
}

func (d *jetStreamDelivery) Notification() Notification {
	return d.notification
}

func (d *jetStreamDelivery) Ack(ctx context.Context) error {
	return d.msg.DoubleAck(ctx)
}

func (d *jetStreamDelivery) Nak(_ context.Context, delay time.Duration) error {
	if delay <= 0 {
		return d.msg.Nak()
	}
	return d.msg.NakWithDelay(delay)
}

func connectJetStream(config Config) (*nats.Conn, jetstream.JetStream, error) {
	nc, err := nats.Connect(config.URL, nats.Name("svrtools-dispatch"), nats.Timeout(connectTimeout))
	if err != nil {
		return nil, nil, fmt.Errorf("connect to NATS: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("initialise NATS JetStream: %w", err)
	}
	return nc, js, nil
}

func ensureStream(ctx context.Context, js jetstream.JetStream, config Config) error {
	stream, err := js.Stream(ctx, config.Stream)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		_, err = js.CreateStream(ctx, jetstream.StreamConfig{
			Name:        config.Stream,
			Subjects:    []string{config.Subject},
			Retention:   jetstream.WorkQueuePolicy,
			Storage:     jetstream.FileStorage,
			MaxMsgs:     100000,
			MaxAge:      7 * 24 * time.Hour,
			Duplicates:  config.DuplicateWindow,
			Description: "VPS Tools database-authoritative job notifications",
		})
		if err != nil {
			return fmt.Errorf("create JetStream stream: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect JetStream stream: %w", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return fmt.Errorf("inspect JetStream stream configuration: %w", err)
	}
	if info.Config.Retention != jetstream.WorkQueuePolicy || info.Config.Storage != jetstream.FileStorage || info.Config.Duplicates != config.DuplicateWindow || !contains(info.Config.Subjects, config.Subject) {
		return fmt.Errorf("JetStream stream %q exists with incompatible retention, storage, or subject configuration", config.Stream)
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func validateJetStreamConfig(config Config) error {
	if strings.TrimSpace(config.URL) == "" {
		return errors.New("NATS_URL is required for JetStream dispatch")
	}
	if strings.TrimSpace(config.Stream) == "" || strings.TrimSpace(config.Subject) == "" || strings.TrimSpace(config.Durable) == "" {
		return errors.New("JetStream stream, subject, and durable consumer are required")
	}
	if config.MaxDeliver < 1 || config.MaxDeliver > 20 {
		return errors.New("JetStream MaxDeliver must be between 1 and 20")
	}
	if config.AckWait <= 0 || config.DuplicateWindow <= 0 {
		return errors.New("JetStream AckWait and DuplicateWindow must be positive")
	}
	return nil
}
