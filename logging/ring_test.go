package logging

import (
	"sync"
	"testing"
	"time"
)

func add(r *Ring, level, message string) Entry {
	return r.Add(Entry{Time: time.Now().UTC(), Level: level, Message: message})
}

func messages(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Message)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRingKeepsTheMostRecentEntries(t *testing.T) {
	r := NewRing(3)

	for _, m := range []string{"a", "b", "c", "d", "e"} {
		add(r, "info", m)
	}

	if got, want := r.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	// Oldest first, and "a" and "b" evicted rather than "d" and "e" dropped.
	if got, want := messages(r.Snapshot(Filter{})), []string{"c", "d", "e"}; !equal(got, want) {
		t.Fatalf("Snapshot() = %v, want %v", got, want)
	}
}

func TestRingSnapshotBeforeWrappingDoesNotReportEmptySlots(t *testing.T) {
	r := NewRing(10)
	add(r, "info", "a")
	add(r, "info", "b")

	if got, want := messages(r.Snapshot(Filter{})), []string{"a", "b"}; !equal(got, want) {
		t.Fatalf("Snapshot() = %v, want %v", got, want)
	}
}

func TestRingSequenceNumbersAreMonotonicAcrossWrapping(t *testing.T) {
	r := NewRing(2)

	var last uint64
	for range 5 {
		e := add(r, "info", "m")
		if e.Seq <= last {
			t.Fatalf("Seq = %d, want greater than %d", e.Seq, last)
		}
		last = e.Seq
	}
	// Eviction must not renumber what is left behind.
	if got, want := r.Snapshot(Filter{})[0].Seq, uint64(4); got != want {
		t.Fatalf("oldest surviving Seq = %d, want %d", got, want)
	}
}

func TestRingFilterByLevelKeepsMoreSevereEntries(t *testing.T) {
	r := NewRing(10)
	add(r, "debug", "d")
	add(r, "info", "i")
	add(r, "warning", "w")
	add(r, "error", "e")

	got := messages(r.Snapshot(Filter{MinLevel: "warning"}))
	if want := []string{"w", "e"}; !equal(got, want) {
		t.Fatalf("Snapshot(warning) = %v, want %v", got, want)
	}
}

func TestRingUnknownFilterLevelDoesNotHideEverything(t *testing.T) {
	r := NewRing(10)
	add(r, "info", "i")

	// A typo in a query string should not look like "there are no logs".
	if got := r.Snapshot(Filter{MinLevel: "verbose"}); len(got) != 1 {
		t.Fatalf("Snapshot(bogus level) returned %d entries, want 1", len(got))
	}
}

func TestRingFilterSinceReturnsOnlyNewerEntries(t *testing.T) {
	r := NewRing(10)
	add(r, "info", "a")
	second := add(r, "info", "b")
	add(r, "info", "c")

	got := messages(r.Snapshot(Filter{Since: second.Seq}))
	if want := []string{"c"}; !equal(got, want) {
		t.Fatalf("Snapshot(since) = %v, want %v", got, want)
	}
}

func TestRingFilterLimitKeepsTheNewest(t *testing.T) {
	r := NewRing(10)
	for _, m := range []string{"a", "b", "c", "d"} {
		add(r, "info", m)
	}

	got := messages(r.Snapshot(Filter{Limit: 2}))
	if want := []string{"c", "d"}; !equal(got, want) {
		t.Fatalf("Snapshot(limit 2) = %v, want %v", got, want)
	}
}

func TestRingSubscribeReceivesSubsequentEntries(t *testing.T) {
	r := NewRing(10)
	add(r, "info", "before")

	ch, cancel := r.Subscribe(4)
	defer cancel()

	add(r, "info", "after")

	select {
	case e := <-ch:
		if e.Message != "after" {
			t.Fatalf("received %q, want %q", e.Message, "after")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the subscribed entry")
	}
}

func TestRingCancelClosesTheChannelAndIsIdempotent(t *testing.T) {
	r := NewRing(10)
	ch, cancel := r.Subscribe(1)

	cancel()
	// A second cancel must not panic on a double close.
	cancel()

	if _, open := <-ch; open {
		t.Fatal("the channel yielded a value after cancel, want closed")
	}
	if got := r.Subscribers(); got != 0 {
		t.Fatalf("Subscribers() = %d after cancel, want 0", got)
	}
}

func TestRingSlowSubscriberDoesNotBlockTheLogger(t *testing.T) {
	r := NewRing(10)
	_, cancel := r.Subscribe(1)
	defer cancel()

	// The subscriber never reads, so its one-slot buffer fills immediately.
	// Add must drop rather than block, or every goroutine that logs would
	// stall behind a stalled log viewer.
	done := make(chan struct{})
	go func() {
		for range 100 {
			add(r, "info", "m")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Add blocked on a subscriber that is not reading")
	}
}

func TestRingIsSafeForConcurrentUse(t *testing.T) {
	r := NewRing(64)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				add(r, "info", "m")
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				r.Snapshot(Filter{Limit: 10})
			}
		}()
	}
	wg.Wait()

	if got, want := r.Len(), 64; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
}

func TestNewRingRejectsANonPositiveSize(t *testing.T) {
	// A misconfigured setting must not silently disable log capture.
	for _, size := range []int{0, -1} {
		if got := NewRing(size).Cap(); got != DefaultRingSize {
			t.Fatalf("NewRing(%d).Cap() = %d, want %d", size, got, DefaultRingSize)
		}
	}
}
