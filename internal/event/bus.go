package event

import "sync"

// Subscriber is a function that handles events.
type Subscriber func(Event)

// Bus is a typed event bus using Go channels.
type Bus struct {
	mu          sync.RWMutex
	subscribers []chan Event
	closed      bool
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{}
}

// Subscribe returns a channel that receives events.
// The caller must eventually call Unsubscribe with the returned channel.
func (b *Bus) Subscribe(bufSize int) <-chan Event {
	ch := make(chan Event, bufSize)

	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()

	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (b *Bus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, sub := range b.subscribers {
		if sub == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)

			close(sub)

			return
		}
	}
}

// Publish sends an event to all subscribers.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that subscriber.
// Data is deep-copied via SnapshotData to prevent concurrent map read/write panics.
func (b *Bus) Publish(e Event) {
	// Snapshot Data before broadcasting to avoid races with goroutines
	// that may continue to modify the original maps after publishing.
	if e.Data != nil {
		e.Data = SnapshotData(e.Data)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
}

// Close closes the bus and all subscriber channels.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for _, ch := range b.subscribers {
		close(ch)
	}

	b.subscribers = nil
}
