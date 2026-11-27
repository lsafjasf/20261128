package captionsession

import "sync/atomic"

type subscriber struct {
	ch      chan Event
	stalled atomic.Bool
	closed  atomic.Bool
}

func newSubscriber() *subscriber {
	return &subscriber{ch: make(chan Event, defaultSubBuffer)}
}

// tryDeliver is called under the session lock and must never block.
func (sub *subscriber) tryDeliver(ev Event) bool {
	if sub.closed.Load() || sub.stalled.Load() {
		return false
	}
	select {
	case sub.ch <- ev:
		return true
	default:
		return false
	}
}

// deliver is used for backlog and snapshot events outside the session lock.
func (sub *subscriber) deliver(ev Event) {
	if sub.closed.Load() || sub.stalled.Load() {
		return
	}
	sub.ch <- ev
}

func (sub *subscriber) markStalled() {
	if sub.stalled.CompareAndSwap(false, true) {
		// The resync signal must survive a full buffer: make room rather
		// than silently dropping it.
		select {
		case sub.ch <- Event{Err: ErrResync}:
		default:
			select {
			case <-sub.ch:
			default:
			}
			sub.ch <- Event{Err: ErrResync}
		}
	}
}

// Subscriber is a live viewer's read handle. Events arrive in version order;
// a snapshot event materializes the full state and subsequent events are
// replayed onto it.
type Subscriber struct {
	inner   *subscriber
	session *Session
}

// Events returns the event channel. It is closed when the subscriber is
// unsubscribed.
func (sub *Subscriber) Events() <-chan Event { return sub.inner.ch }

// Stalled reports whether the subscriber overflowed its buffer. After this,
// no further ops are delivered and the client must resync via Snapshot +
// SubscribeFrom.
func (sub *Subscriber) Stalled() bool { return sub.inner.stalled.Load() }

// Close unsubscribes the viewer.
func (sub *Subscriber) Close() {
	if sub.inner.closed.CompareAndSwap(false, true) {
		sub.session.removeSubscriber(sub.inner)
		close(sub.inner.ch)
	}
}
