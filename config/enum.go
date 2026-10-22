package config

// Enumerated configuration vocabularies.
//
// These live here, next to the strategy constants in strategy.go, because they
// are the authoritative definition of what a value is allowed to be. The
// database stores values only; it never carries a copy of the valid set. Adding
// a value is a code change, not a schema migration.

// Watermark positions, used by the watermark renderer to place a text block.
const (
	PositionTopLeft      = "top-left"
	PositionTopCenter    = "top-center"
	PositionTopRight     = "top-right"
	PositionMiddleLeft   = "middle-left"
	PositionMiddleCenter = "middle-center"
	PositionMiddleRight  = "middle-right"
	PositionBottomLeft   = "bottom-left"
	PositionBottomCenter = "bottom-center"
	PositionBottomRight  = "bottom-right"
)

// PositionValues enumerates every valid watermark position.
var PositionValues = []string{
	PositionTopLeft, PositionTopCenter, PositionTopRight,
	PositionMiddleLeft, PositionMiddleCenter, PositionMiddleRight,
	PositionBottomLeft, PositionBottomCenter, PositionBottomRight,
}

// Watermark line alignments.
const (
	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"
)

// AlignmentValues enumerates every valid watermark alignment.
var AlignmentValues = []string{AlignLeft, AlignCenter, AlignRight}

// Crop modes.
const (
	// CropYes always crops the picture to the reference ratio
	CropYes = "yes"
	// CropNo leaves the picture as downloaded
	CropNo = "no"
	// CropWinOnly crops on Windows only
	CropWinOnly = "win-only"
)

// CropValues enumerates every valid crop mode.
var CropValues = []string{CropYes, CropNo, CropWinOnly}

// StrategyValues enumerates every download strategy declared in strategy.go.
// Individual channels support only a subset and declare their own via their
// option schema; this is the union.
var StrategyValues = []string{LargestNoLogo, Largest, ByWidth, BySize}

// Membership sets, so a lookup is a map hit rather than a scan. The slices
// above stay the source of truth, because order matters when the accepted
// values are shown to a user.
var (
	positionSet  = newSet(PositionValues)
	alignmentSet = newSet(AlignmentValues)
	cropSet      = newSet(CropValues)
	strategySet  = newSet(StrategyValues)
)

func newSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

// ValidPosition reports whether s is a known watermark position.
func ValidPosition(s string) bool { _, ok := positionSet[s]; return ok }

// ValidAlignment reports whether s is a known watermark alignment.
func ValidAlignment(s string) bool { _, ok := alignmentSet[s]; return ok }

// ValidCrop reports whether s is a known crop mode.
func ValidCrop(s string) bool { _, ok := cropSet[s]; return ok }

// ValidStrategy reports whether s is a known download strategy.
func ValidStrategy(s string) bool { _, ok := strategySet[s]; return ok }
