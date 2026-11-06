//go:build darwin
// +build darwin

package setter

import (
	"sync"
	"time"
)

// slotReuseGrace is how long a slot must have been off screen before it is
// handed to the system again.
//
// When a wallpaper stops being displayed, WallpaperAgent keeps its rendered
// runtime alive for about five seconds before removing it (observed on
// macOS 26: "Wallpaper removed from runtime" five seconds after the switch
// away, every time). Setting that path again inside the window logs
// `stopPendingRemovalTimerAndReactivate` and `No content change ... already
// resolved`: the agent puts the old render back and never reads the file, so
// the desktop shows whatever the slot held before -- not the picture just
// copied into it. Re-applying two history entries a couple of seconds apart
// hit exactly that.
//
// A second of margin over the observed timer, because the agent switches
// asynchronously and the vacate time recorded here is when osascript
// returned, not when the agent acted on it.
const slotReuseGrace = 6 * time.Second

// slotReuseGuard remembers when each slot stopped being displayed, so a slot
// is never reused while the agent may still hold its old render.
type slotReuseGuard struct {
	mu      sync.Mutex
	vacated map[string]time.Time
	grace   time.Duration

	// last is the slot this process set most recently. The system's own
	// answer to "what is displayed" lags a set by a moment, so right after a
	// set it can still name the slot just replaced; trusting it would pick
	// the slot already on screen, which the agent does not re-render.
	last string

	// Injected so the test can run without sleeping.
	now   func() time.Time
	sleep func(time.Duration)
}

func newSlotReuseGuard(grace time.Duration) *slotReuseGuard {
	return &slotReuseGuard{
		vacated: map[string]time.Time{},
		grace:   grace,
		now:     time.Now,
		sleep:   time.Sleep,
	}
}

// waitFor returns how long slot must still wait before it can be set,
// zero when it is safe now. It does not sleep, so the caller can log first.
func (g *slotReuseGuard) waitFor(slot string) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	at, ok := g.vacated[slot]
	if !ok {
		return 0
	}
	if remaining := g.grace - g.now().Sub(at); remaining > 0 {
		return remaining
	}
	return 0
}

// current is the slot this process set last, or "" before the first set.
func (g *slotReuseGuard) current() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last
}

// recordSet notes that slot is now displayed, and that every other slot --
// whether or not the system still reports it -- stopped being displayed now.
func (g *slotReuseGuard) recordSet(slot string, others []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	for _, other := range others {
		if other != "" && other != slot {
			g.vacated[other] = now
		}
	}
	delete(g.vacated, slot)
	g.last = slot
}

// slotGuard is the process-wide guard. A restart loses it, which is harmless:
// restarting takes longer than the grace.
var slotGuard = newSlotReuseGuard(slotReuseGrace)
