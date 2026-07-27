// Package dispatch contains the boundary between database-authoritative job
// claiming and optional delivery notifications.
//
// The JetStream implementation in this package is intentionally a bridge, not
// a second execution queue. A notification identifies a database target. A
// runner must still claim that target through the API before it can execute it.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	NotificationVersion = 1
	NotificationKind    = "execution_target_available"
)

var ErrNoMessage = errors.New("no dispatch notification available")

// Config controls the JetStream stream and durable pull consumer.
type Config struct {
	URL             string
	Stream          string
	Subject         string
	Durable         string
	MaxDeliver      int
	AckWait         time.Duration
	DuplicateWindow time.Duration
}

// Notification is deliberately metadata-only. It carries no command,
// credentials, host details, or lease. Those values remain behind the API.
type Notification struct {
	Version     int    `json:"version"`
	Kind        string `json:"kind"`
	TargetID    string `json:"target_id"`
	ExecutionID string `json:"execution_id"`
	Attempt     int    `json:"attempt"`
}

func (n Notification) Validate() error {
	if n.Version != NotificationVersion {
		return fmt.Errorf("unsupported dispatch notification version %d", n.Version)
	}
	if n.Kind != NotificationKind {
		return fmt.Errorf("unsupported dispatch notification kind %q", n.Kind)
	}
	if n.TargetID == "" || n.ExecutionID == "" {
		return errors.New("dispatch notification requires target_id and execution_id")
	}
	if n.Attempt < 0 {
		return errors.New("dispatch notification attempt must not be negative")
	}
	return nil
}

func (n Notification) MessageID() string {
	return fmt.Sprintf("%s:%d", n.TargetID, n.Attempt)
}

func (n Notification) Marshal() ([]byte, error) {
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(n)
}

func ParseNotification(payload []byte) (Notification, error) {
	var n Notification
	if err := json.Unmarshal(payload, &n); err != nil {
		return Notification{}, fmt.Errorf("decode dispatch notification: %w", err)
	}
	if err := n.Validate(); err != nil {
		return Notification{}, err
	}
	return n, nil
}

type Publisher interface {
	Publish(context.Context, Notification) error
	Close() error
}

type Delivery interface {
	Notification() Notification
	Ack(context.Context) error
	Nak(context.Context, time.Duration) error
}

type Consumer interface {
	Next(context.Context) (Delivery, error)
	Close() error
}
