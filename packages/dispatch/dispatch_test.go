package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNotificationRoundTripAndStableMessageID(t *testing.T) {
	notification := Notification{
		Version: NotificationVersion, Kind: NotificationKind,
		TargetID: "target-1", ExecutionID: "execution-1", Attempt: 2,
	}
	payload, err := notification.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseNotification(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != notification {
		t.Fatalf("decoded notification differs: %#v", decoded)
	}
	if decoded.MessageID() != "target-1:2" {
		t.Fatalf("unexpected stable message ID: %q", decoded.MessageID())
	}
}

func TestNotificationValidationRejectsExecutableOrIncompletePayloads(t *testing.T) {
	tests := []Notification{
		{Version: 2, Kind: NotificationKind, TargetID: "target", ExecutionID: "execution"},
		{Version: NotificationVersion, Kind: "execute_command", TargetID: "target", ExecutionID: "execution"},
		{Version: NotificationVersion, Kind: NotificationKind, ExecutionID: "execution"},
		{Version: NotificationVersion, Kind: NotificationKind, TargetID: "target", ExecutionID: "execution", Attempt: -1},
	}
	for _, notification := range tests {
		if err := notification.Validate(); err == nil {
			t.Fatalf("expected validation error for %#v", notification)
		}
	}
}

func TestValidateJetStreamConfigBoundsRedelivery(t *testing.T) {
	valid := Config{URL: "nats://localhost:4222", Stream: "SVRTOOLS_JOBS", Subject: "svrtools.jobs.available", Durable: "runner", MaxDeliver: 5, AckWait: time.Second, DuplicateWindow: time.Minute}
	if err := validateJetStreamConfig(valid); err != nil {
		t.Fatal(err)
	}
	for _, maxDeliver := range []int{0, 21} {
		valid.MaxDeliver = maxDeliver
		if err := validateJetStreamConfig(valid); err == nil {
			t.Fatalf("expected MaxDeliver=%d to fail", maxDeliver)
		}
	}
}

func TestErrNoMessageIsDistinctFromContextCancellation(t *testing.T) {
	if errors.Is(ErrNoMessage, context.Canceled) {
		t.Fatal("no-message sentinel must not look like context cancellation")
	}
}
