// Package engine provides the MemoryEngine which coordinates storage and
// enrichment for the Memento memory system.
package engine

import (
	"math"
	"time"

	"github.com/scrypster/memento/pkg/types"
)

const (
	// defaultHalfLifeHours is the half-life used for exponential decay (1 week).
	defaultHalfLifeHours = 168.0

	// decayScoreThreshold is the minimum change required to write back a new score.
	decayScoreThreshold = 0.001
)

// DecayManager calculates and applies decay scores to memories.
//
// The combined decay score is computed as:
//
//	rawScore = Importance + exp(-λ * hours_since_last_access)
//
// where λ = ln(2) / half_life_hours. Because Importance ∈ [0,1] and the
// exponential factor ∈ (0,1], rawScore ∈ (0,2]. Dividing by 2.0 normalises
// the result to [0.0, 1.0] — satisfying Opus Issue #4.
type DecayManager struct {
	halfLifeHours float64
}

// NewDecayManager returns a DecayManager with the default half-life of 168 hours.
func NewDecayManager() *DecayManager {
	return &DecayManager{halfLifeHours: defaultHalfLifeHours}
}

// NewDecayManagerWithHalfLife returns a DecayManager with a custom half-life.
// halfLifeHours must be > 0; if it is not, the default (168 h) is used.
func NewDecayManagerWithHalfLife(halfLifeHours float64) *DecayManager {
	if halfLifeHours <= 0 {
		halfLifeHours = defaultHalfLifeHours
	}
	return &DecayManager{halfLifeHours: halfLifeHours}
}

// lambda returns the decay constant derived from the configured half-life.
func (d *DecayManager) lambda() float64 {
	return math.Log(2) / d.halfLifeHours
}

// refTime returns the reference timestamp used for decay calculation.
// It prefers LastAccessedAt and falls back to CreatedAt.
func refTime(mem *types.Memory) time.Time {
	if mem.LastAccessedAt != nil && !mem.LastAccessedAt.IsZero() {
		return *mem.LastAccessedAt
	}
	return mem.CreatedAt
}

// CalculateDecay returns the raw exponential decay factor for mem at the
// given instant. The returned value is in [0.0, 1.0].
func (d *DecayManager) CalculateDecay(mem *types.Memory, now time.Time) float64 {
	hours := now.Sub(refTime(mem)).Hours()
	if hours < 0 {
		hours = 0
	}
	return math.Exp(-d.lambda() * hours)
}

// ApplyDecay calculates the normalised combined decay score for mem and writes
// it back to mem.DecayScore. DecayUpdatedAt is set to now only when the score
// changes by more than decayScoreThreshold.
//
// The formula is:
//
//	DecayScore = (Importance + exp(-λ * hours_since_last_access)) / 2.0
//
// This guarantees DecayScore ∈ [0.0, 1.0].
func (d *DecayManager) ApplyDecay(mem *types.Memory, now time.Time) {
	decayFactor := d.CalculateDecay(mem, now)

	// Clamp Importance to [0, 1] to be defensive against malformed data.
	importance := mem.Importance
	if importance < 0 {
		importance = 0
	}
	if importance > 1 {
		importance = 1
	}

	// Combine and normalise to [0.0, 1.0] (Opus Issue #4: divide by 2.0).
	newScore := (importance + decayFactor) / 2.0

	// Skip writing back if the score has not changed meaningfully.
	if math.Abs(newScore-mem.DecayScore) < decayScoreThreshold {
		return
	}

	mem.DecayScore = newScore
	mem.DecayUpdatedAt = &now
}
