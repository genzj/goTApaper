package logging

import "sync"

// DefaultRingSize is the number of entries kept in memory when no size is
// configured.
const DefaultRingSize = 2000

// Ring is a fixed-capacity circular buffer of log entries with a fan-out to
// live subscribers.
//
// Capacity bounds memory use directly: there is no age-based eviction, an
// entry is dropped only when a newer one needs its slot. It is safe for
// concurrent use, because logrus hooks fire on whichever goroutine logged and
// the API reads from its own.
type Ring struct {
	mu      sync.RWMutex
	entries []Entry
	// next is the index the following write goes to; once the buffer has
	// wrapped it is also the oldest entry.
	next int
	// count is how many slots are filled, saturating at cap(entries). Kept
	// separately from next so a partially filled buffer does not report empty
	// trailing slots as entries.
	count int
	seq   uint64

	subscribers map[int]chan Entry
	nextSubID   int
}

// NewRing builds a ring holding up to size entries. A size below one falls
// back to DefaultRingSize, so a misconfigured setting cannot silently disable
// logging.
func NewRing(size int) *Ring {
	if size < 1 {
		size = DefaultRingSize
	}
	return &Ring{
		entries:     make([]Entry, size),
		subscribers: map[int]chan Entry{},
	}
}

// Cap reports the configured capacity.
func (r *Ring) Cap() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Len reports how many entries are currently buffered.
func (r *Ring) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// Add stores an entry, assigns it a sequence number and publishes it to
// subscribers. It returns the stored entry, so a caller that also needs to
// write the record elsewhere uses the same sequence number.
func (r *Ring) Add(e Entry) Entry {
	r.mu.Lock()

	r.seq++
	e.Seq = r.seq
	r.entries[r.next] = e
	r.next = (r.next + 1) % len(r.entries)
	if r.count < len(r.entries) {
		r.count++
	}

	// Snapshot the subscriber channels so the send below happens outside the
	// lock. Holding it across a send would let a slow reader block every
	// goroutine that logs.
	targets := make([]chan Entry, 0, len(r.subscribers))
	for _, ch := range r.subscribers {
		targets = append(targets, ch)
	}
	r.mu.Unlock()

	for _, ch := range targets {
		select {
		case ch <- e:
		default:
			// The subscriber is not keeping up. Dropping is deliberate: a live
			// tail that stalls must not slow down the application whose logs
			// it is watching, and the reader can re-sync from Snapshot.
		}
	}

	return e
}

// Snapshot returns the buffered entries matching f, oldest first.
func (r *Ring) Snapshot(f Filter) []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Entry, 0, r.count)
	// Walk from the oldest filled slot. Once wrapped that is next; before
	// wrapping the buffer starts at zero.
	start := 0
	if r.count == len(r.entries) {
		start = r.next
	}
	for i := range r.count {
		e := r.entries[(start+i)%len(r.entries)]
		if f.Matches(e) {
			out = append(out, e)
		}
	}

	// Limit keeps the most recent entries, which is what a log viewer wants
	// when it asks for "the last 100 lines".
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out
}

// Subscribe returns a channel of entries added from now on, and a function
// that stops the subscription and closes the channel.
//
// The channel is buffered: a subscriber that falls behind by more than the
// buffer misses entries rather than blocking the logger. Callers that need a
// gap-free view read Snapshot first and then follow, using Entry.Seq to tell
// whether anything was missed.
func (r *Ring) Subscribe(buffer int) (<-chan Entry, func()) {
	if buffer < 1 {
		buffer = 64
	}

	r.mu.Lock()
	id := r.nextSubID
	r.nextSubID++
	ch := make(chan Entry, buffer)
	r.subscribers[id] = ch
	r.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.subscribers, id)
			r.mu.Unlock()
			// Closed after removal from the map, so Add can never send on a
			// closed channel. Once guards against a double close from a
			// caller that cancels twice.
			close(ch)
		})
	}
	return ch, cancel
}

// Subscribers reports the number of live subscriptions, for tests and for the
// debug log.
func (r *Ring) Subscribers() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscribers)
}
