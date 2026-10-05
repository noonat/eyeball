package server

import "sync"

// hub tells waiting requests that something in the store changed.
//
// A signal carries no payload, and each subscriber holds a channel with room
// for one. Publishing is a send that never blocks: a subscriber that has not
// drained its last signal already knows something changed, and loses nothing
// by missing the second, because it re-reads the store either way. A payload
// would need a buffer per subscriber and a rule for a full one, and every rule
// either drops an event or blocks the writer.
type hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

// newHub returns a hub with no subscribers.
func newHub() *hub {
	return &hub{subs: map[chan struct{}]struct{}{}}
}

// subscribe returns a channel that receives a signal after every publish, and
// a function that stops it. A caller subscribes before it reads, so a change
// landing between its read and its wait is not missed.
func (h *hub) subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// publish signals every subscriber.
func (h *hub) publish() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// count is the number of subscribers, which a test reads to prove an
// unsubscribe took effect.
func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
