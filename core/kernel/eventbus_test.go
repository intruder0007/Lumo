package kernel

import (
	"errors"
	"testing"
)

func TestEventBus_PublishWithoutRegisterEmitterFails(t *testing.T) {
	b := NewEventBus()
	err := b.Publish(Event{Domain: "source-control", Kind: "branch-changed"})
	var want *UnregisteredEmitterError
	if !errors.As(err, &want) {
		t.Fatalf("Publish before RegisterEmitter = %v, want *UnregisteredEmitterError", err)
	}
}

func TestEventBus_PublishDisallowedKindFails(t *testing.T) {
	b := NewEventBus()
	b.RegisterEmitter("source-control", []string{"branch-changed"})

	err := b.Publish(Event{Domain: "source-control", Kind: "commit-created"})
	var want *DisallowedEventKindError
	if !errors.As(err, &want) {
		t.Fatalf("Publish of undeclared kind = %v, want *DisallowedEventKindError", err)
	}
}

func TestEventBus_SubscriberReceivesMatchingKind(t *testing.T) {
	b := NewEventBus()
	b.RegisterEmitter("source-control", []string{"branch-changed", "commit-created"})

	var received []Event
	b.Subscribe("source-control", []string{"branch-changed"}, func(e Event) {
		received = append(received, e)
	})

	if err := b.Publish(Event{Domain: "source-control", Kind: "branch-changed"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := b.Publish(Event{Domain: "source-control", Kind: "commit-created"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if len(received) != 1 || received[0].Kind != "branch-changed" {
		t.Fatalf("received = %#v, want exactly one branch-changed event", received)
	}
}

func TestEventBus_SubscriberOnlyReceivesItsOwnDomain(t *testing.T) {
	b := NewEventBus()
	b.RegisterEmitter("source-control", []string{"branch-changed"})
	b.RegisterEmitter("security-center", []string{"gate-state-changed"})

	var scReceived, secReceived int
	b.Subscribe("source-control", []string{"branch-changed"}, func(Event) { scReceived++ })
	b.Subscribe("security-center", []string{"gate-state-changed"}, func(Event) { secReceived++ })

	_ = b.Publish(Event{Domain: "source-control", Kind: "branch-changed"})

	if scReceived != 1 {
		t.Fatalf("source-control subscriber received %d events, want 1", scReceived)
	}
	if secReceived != 0 {
		t.Fatalf("security-center subscriber received %d events, want 0", secReceived)
	}
}

func TestEventBus_PanickingHandlerDoesNotBlockOthers(t *testing.T) {
	b := NewEventBus()
	b.RegisterEmitter("automation", []string{"job-completed"})

	var secondCalled bool
	b.Subscribe("automation", []string{"job-completed"}, func(Event) {
		panic("simulated handler failure")
	})
	b.Subscribe("automation", []string{"job-completed"}, func(Event) {
		secondCalled = true
	})

	if err := b.Publish(Event{Domain: "automation", Kind: "job-completed"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !secondCalled {
		t.Fatal("second subscriber was not called after the first panicked")
	}
}

func TestEventBus_RegisterEmitterReplacesPreviousAllowList(t *testing.T) {
	b := NewEventBus()
	b.RegisterEmitter("quality", []string{"quality-gate-changed", "old-kind"})
	b.RegisterEmitter("quality", []string{"quality-gate-changed"})

	if err := b.Publish(Event{Domain: "quality", Kind: "old-kind"}); err == nil {
		t.Fatal("Publish of a kind dropped by re-registration should fail")
	}
}
