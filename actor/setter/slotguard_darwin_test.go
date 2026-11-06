//go:build darwin
// +build darwin

package setter

import (
	"testing"
	"time"
)

func newTestGuard() (*slotReuseGuard, *time.Time) {
	clock := time.Date(2026, 11, 5, 23, 53, 51, 0, time.UTC)
	g := newSlotReuseGuard(6 * time.Second)
	g.now = func() time.Time { return clock }
	g.sleep = func(d time.Duration) { clock = clock.Add(d) }
	return g, &clock
}

// TestSlotGuardWaitsOutRapidReapply replays the clicks that failed: two
// applies a couple of seconds apart, so the second goes back to the slot the
// agent was still holding.
func TestSlotGuardWaitsOutRapidReapply(t *testing.T) {
	const a, b = "/app/wallpaper_osx_0.jpeg", "/app/wallpaper_osx_1.jpeg"
	slots := []string{a, b}
	g, clock := newTestGuard()

	if w := g.waitFor(a); w != 0 {
		t.Fatalf("an unused slot must not wait, got %s", w)
	}
	g.recordSet(a, slots)
	*clock = clock.Add(2 * time.Second)

	g.recordSet(b, slots) // a leaves the screen now
	*clock = clock.Add(2500 * time.Millisecond)

	if w := g.waitFor(a); w != 3500*time.Millisecond {
		t.Errorf("reusing a 2.5s after it was vacated: want 3.5s wait, got %s", w)
	}
	if w := g.waitFor(b); w != 0 {
		t.Errorf("the slot on screen was never vacated, want no wait, got %s", w)
	}

	g.sleep(g.waitFor(a))
	if w := g.waitFor(a); w != 0 {
		t.Errorf("after waiting out the grace, want no wait, got %s", w)
	}
}

// TestSlotGuardTrustsItsOwnLastSet pins that the slot chosen next is decided
// by what was set, not by a system readout that still names the old slot.
func TestSlotGuardTrustsItsOwnLastSet(t *testing.T) {
	const a, b = "/app/wallpaper_osx_0.jpeg", "/app/wallpaper_osx_1.jpeg"
	g, _ := newTestGuard()

	if got := g.current(); got != "" {
		t.Fatalf("before any set, want no current slot, got %q", got)
	}
	g.recordSet(b, []string{a, b})
	if got := g.current(); got != b {
		t.Errorf("want current %q, got %q", b, got)
	}
	if got := nextSlot(g.current(), "/app", ".jpeg"); got != "wallpaper_osx_0" {
		t.Errorf("with %q on screen, want the other slot, got %q", b, got)
	}
}

// TestSlotGuardClearsVacatedOnSet pins that setting a slot clears its own
// vacate mark, so the slot on screen is never made to wait for itself.
func TestSlotGuardClearsVacatedOnSet(t *testing.T) {
	const a, b = "/app/wallpaper_osx_0.jpeg", "/app/wallpaper_osx_1.jpeg"
	g, _ := newTestGuard()

	g.recordSet(b, []string{a, b})
	g.recordSet(a, []string{a, b})
	if _, ok := g.vacated[a]; ok {
		t.Errorf("the slot just set must not be marked vacated")
	}
}
