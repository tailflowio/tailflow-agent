package event

import (
	"sync"
	"sync/atomic"
)

// Subscriber is a function that handles events.
type Subscriber func(Event)

type subscription struct {
	ch    chan Event
	lossy bool
}

// Bus is a typed event bus using Go channels.
type Bus struct {
	mu          sync.RWMutex
	subscribers []*subscription
	closed      bool
	seq         atomic.Uint64
	dropped     atomic.Uint64
}

func NewBus() *Bus {
	return &Bus{}
}

// Subscribe returns a channel that receives events. The subscriber is lossy:
// if the buffer is full when an event is published, that event is dropped for
// this subscriber. Use this for clients that can tolerate gaps and recover via
// other means (e.g. SSE clients can reload).
//
// The caller must eventually call Unsubscribe with the returned channel.
func (b *Bus) Subscribe(bufSize int) <-chan Event {
	return b.subscribe(bufSize, true)
}

// SubscribeBlocking returns a channel that receives events. The subscriber is
// non-lossy: Publish will block on this subscriber if its buffer is full.
// Use this for components that own data integrity (e.g. the execution store).
//
// The caller must drain the channel promptly to avoid blocking publishers.
func (b *Bus) SubscribeBlocking(bufSize int) <-chan Event {
	return b.subscribe(bufSize, false)
}

func (b *Bus) subscribe(bufSize int, lossy bool) <-chan Event {
	ch := make(chan Event, bufSize)

	b.mu.Lock()
	b.subscribers = append(b.subscribers, &subscription{ch: ch, lossy: lossy})
	b.mu.Unlock()

	return ch
}

func (b *Bus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, sub := range b.subscribers {
		if (<-chan Event)(sub.ch) == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)

			close(sub.ch)

			return
		}
	}
}

// Publish sends an event to all subscribers.
//
// Lossy subscribers (Subscribe) drop the event if their buffer is full.
// Blocking subscribers (SubscribeBlocking) make Publish wait until they accept.
// Data is deep-copied via SnapshotData to prevent concurrent map read/write panics.
func (b *Bus) Publish(e Event) {
	// Assign monotonic sequence number for stable ordering.
	e.Seq = b.seq.Add(1)

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

	for _, sub := range b.subscribers {
		if sub.lossy {
			select {
			case sub.ch <- e:
			default:
				b.dropped.Add(1)
			}

			continue
		}

		// Blocking subscriber: wait until accepted.
		sub.ch <- e
	}
}

// Dropped returns the cumulative count of events dropped for lossy subscribers.
// Useful for observability / metrics.
func (b *Bus) Dropped() uint64 {
	return b.dropped.Load()
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for _, sub := range b.subscribers {
		close(sub.ch)
	}

	b.subscribers = nil
}
