package kernel

import "sync"

// Event is a change notification. Payloads carry enough to identify what
// changed (Domain, Kind) but not the new state itself — a handler that
// needs the current state calls FactStore.Get, so the store stays the
// single source of truth for "current state" and the bus stays a
// notification channel.
type Event struct {
	Domain DomainID
	Kind   string
}

// Handler reacts to an Event published by the domain it subscribed to.
type Handler func(Event)

// EventBus lets domains subscribe to other domains' change notifications
// without polling the FactStore. Every emitting domain must first declare
// the event kinds it emits via RegisterEmitter — there is no wildcard
// "publish anything" escape hatch (docs/architecture/domains/kernel.md
// §2), which is the direct mitigation for an unbounded
// everything-notifies-everything bus.
//
// A subscriber's handler is invoked synchronously and is isolated with
// recover() so a panicking handler cannot take down the bus or other
// subscribers. Isolating a *blocking* handler (so it cannot stall
// delivery to other subscribers) is deliberately out of scope for this
// skeleton — Phase A2 has no real domain generating load yet, and adding
// a worker-pool/goroutine-per-handler design before a concrete domain
// needs it would be exactly the over-design risk the kernel skeleton is
// meant to avoid. Revisit once a Phase B domain's handler actually needs
// to do slow work.
type EventBus struct {
	mu          sync.Mutex
	allowed     map[DomainID]map[string]bool
	subscribers map[DomainID][]subscription
}

type subscription struct {
	kinds   map[string]bool
	handler Handler
}

// NewEventBus returns an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		allowed:     make(map[DomainID]map[string]bool),
		subscribers: make(map[DomainID][]subscription),
	}
}

// RegisterEmitter declares the event kinds domain is allowed to publish.
// Publish rejects any kind not declared here for the publishing domain.
// Calling RegisterEmitter again for the same domain replaces its
// previous allow-list (it does not merge), so a domain's declared kinds
// always match its latest registration.
func (b *EventBus) RegisterEmitter(domain DomainID, kinds []string) {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allowed[domain] = set
}

// UnregisteredEmitterError is returned by Publish when the publishing
// domain never called RegisterEmitter.
type UnregisteredEmitterError struct{ Domain DomainID }

func (e *UnregisteredEmitterError) Error() string {
	return string(e.Domain) + ": kernel: no event kinds registered for this domain (call RegisterEmitter first)"
}

// DisallowedEventKindError is returned by Publish when the event's Kind
// was not declared for its Domain via RegisterEmitter.
type DisallowedEventKindError struct {
	Domain DomainID
	Kind   string
}

func (e *DisallowedEventKindError) Error() string {
	return string(e.Domain) + ": kernel: event kind " + e.Kind + " was not declared via RegisterEmitter"
}

// Publish delivers e to every subscriber of e.Domain that subscribed to
// e.Kind. It returns an error, without delivering the event, if
// e.Domain never registered e.Kind via RegisterEmitter.
func (b *EventBus) Publish(e Event) error {
	b.mu.Lock()
	kinds, registered := b.allowed[e.Domain]
	if !registered {
		b.mu.Unlock()
		return &UnregisteredEmitterError{Domain: e.Domain}
	}
	if !kinds[e.Kind] {
		b.mu.Unlock()
		return &DisallowedEventKindError{Domain: e.Domain, Kind: e.Kind}
	}
	subs := append([]subscription(nil), b.subscribers[e.Domain]...)
	b.mu.Unlock()

	for _, s := range subs {
		if s.kinds[e.Kind] {
			invoke(s.handler, e)
		}
	}
	return nil
}

// invoke calls handler with e, recovering from a panic so one
// misbehaving subscriber cannot take down the bus or other subscribers.
func invoke(handler Handler, e Event) {
	defer func() { recover() }()
	handler(e)
}

// Subscribe registers handler to be called for every future event
// published by domain whose Kind is in kinds.
func (b *EventBus) Subscribe(domain DomainID, kinds []string, handler Handler) {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[domain] = append(b.subscribers[domain], subscription{kinds: set, handler: handler})
}
