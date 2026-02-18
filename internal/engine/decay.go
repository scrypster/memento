// Package engine provides the memory enrichment and management engine.
package engine

import (
	"math"
	"time"
)

const (
	// decayHalfLifeDays is the number of days for decay_score to halve
	// without any access. At 60 days a memory sits at 0.5; at 120 days, 0.25.
	decayHalfLifeDays = 60.0

	// accessBoost is added to decay_score on each access, capped at 1.0.
	accessBoost = 0.1
)

// ComputeDecayScore returns the current decay score for a memory given:
//   - baseScore: the score when the memory was last accessed (or created)
//   - lastAccess: when the memory was last accessed (or created_at if never)
//   - accessCount: total number of times the memory has been accessed
//
// Score formula: baseScore * 2^(-daysSinceAccess / halfLife)
// Score is clamped to [0.0, 1.0].
func ComputeDecayScore(baseScore float64, lastAccess time.Time, accessCount int) float64 {
	daysSince := time.Since(lastAccess).Hours() / 24.0
	decayed := baseScore * math.Pow(2, -daysSince/decayHalfLifeDays)

	// Access count provides a small permanent boost (floor effect)
	boost := math.Min(float64(accessCount)*0.01, 0.3)
	score := decayed + boost

	return math.Min(math.Max(score, 0.0), 1.0)
}

// DecayScoreAfterAccess computes the new decay_score after an access event.
// Accessing a memory boosts its score towards 1.0.
func DecayScoreAfterAccess(currentScore float64) float64 {
	return math.Min(currentScore+accessBoost, 1.0)
}
