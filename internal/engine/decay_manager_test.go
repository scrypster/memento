package engine_test

import (
	"testing"
	"time"

	"github.com/scrypster/memento/internal/engine"
	"github.com/scrypster/memento/pkg/types"
)

// TestCalculateDecayReturnsValidRange verifies that CalculateDecay returns
// a value within [0.0, 1.0].
func TestCalculateDecayReturnsValidRange(t *testing.T) {
	dm := engine.NewDecayManager()

	cases := []struct {
		name           string
		hoursSinceAccess float64
	}{
		{"just_accessed", 0},
		{"one_day_ago", 24},
		{"one_week_ago", 168},
		{"one_month_ago", 720},
		{"one_year_ago", 8760},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			ref := now.Add(-time.Duration(tc.hoursSinceAccess * float64(time.Hour)))
			mem := &types.Memory{
				CreatedAt: ref,
				Importance: 0.5,
			}

			score := dm.CalculateDecay(mem, now)

			if score < 0.0 || score > 1.0 {
				t.Errorf("CalculateDecay(%s): score %f is outside [0.0, 1.0]", tc.name, score)
			}
		})
	}
}

// TestCalculateDecayDecaysOverTime verifies that older memories have lower scores.
func TestCalculateDecayDecaysOverTime(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	recentMem := &types.Memory{
		CreatedAt:  now.Add(-1 * time.Hour),
		Importance: 0.5,
	}
	oldMem := &types.Memory{
		CreatedAt:  now.Add(-720 * time.Hour), // 30 days ago
		Importance: 0.5,
	}

	recentScore := dm.CalculateDecay(recentMem, now)
	oldScore := dm.CalculateDecay(oldMem, now)

	if recentScore <= oldScore {
		t.Errorf("expected recent memory score (%f) > old memory score (%f)", recentScore, oldScore)
	}
}

// TestCalculateDecayPrefersLastAccessedAt verifies that when LastAccessedAt is set,
// it is used instead of CreatedAt for decay calculation.
func TestCalculateDecayPrefersLastAccessedAt(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	recent := now.Add(-1 * time.Hour)
	mem := &types.Memory{
		CreatedAt:      now.Add(-720 * time.Hour), // created 30 days ago
		LastAccessedAt: &recent,                    // but accessed 1 hour ago
		Importance:     0.5,
	}

	score := dm.CalculateDecay(mem, now)

	// Score should be high because LastAccessedAt is recent
	if score < 0.9 {
		t.Errorf("expected score > 0.9 when recently accessed, got %f", score)
	}
}

// TestApplyDecayWritesDecayScore verifies that ApplyDecay sets mem.DecayScore.
func TestApplyDecayWritesDecayScore(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	mem := &types.Memory{
		CreatedAt:  now.Add(-24 * time.Hour),
		Importance: 0.6,
	}

	dm.ApplyDecay(mem, now)

	if mem.DecayScore == 0.0 {
		t.Error("expected DecayScore to be set by ApplyDecay, got 0.0")
	}
}

// TestApplyDecayScoreInNormalizedRange verifies that DecayScore is always in [0.0, 1.0].
func TestApplyDecayScoreInNormalizedRange(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	cases := []struct {
		name       string
		hoursAgo   float64
		importance float64
	}{
		{"high_importance_recent", 0, 1.0},
		{"high_importance_old", 8760, 1.0},
		{"low_importance_recent", 0, 0.0},
		{"low_importance_old", 8760, 0.0},
		{"mid_importance_one_week", 168, 0.5},
		{"mid_importance_one_month", 720, 0.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := now.Add(-time.Duration(tc.hoursAgo * float64(time.Hour)))
			mem := &types.Memory{
				CreatedAt:  ref,
				Importance: tc.importance,
			}

			dm.ApplyDecay(mem, now)

			if mem.DecayScore < 0.0 || mem.DecayScore > 1.0 {
				t.Errorf("ApplyDecay(%s): DecayScore %f is outside [0.0, 1.0]", tc.name, mem.DecayScore)
			}
		})
	}
}

// TestApplyDecayScoreNotExceedOnePointZero verifies the [0.0, 2.0] raw formula is
// divided by 2.0 so the result never exceeds 1.0.
func TestApplyDecayScoreNotExceedOnePointZero(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	// Maximum possible raw score: importance=1.0 + decayFactor=1.0 (at t=0) = 2.0
	// After dividing by 2.0 the result must be exactly 1.0.
	mem := &types.Memory{
		CreatedAt:  now, // accessed right now => decay factor = 1.0
		Importance: 1.0,
	}

	dm.ApplyDecay(mem, now)

	if mem.DecayScore > 1.0 {
		t.Errorf("DecayScore must not exceed 1.0, got %f", mem.DecayScore)
	}

	// Score should be very close to 1.0 (both components at their max)
	if mem.DecayScore < 0.99 {
		t.Errorf("expected DecayScore close to 1.0 for fresh memory with max importance, got %f", mem.DecayScore)
	}
}

// TestApplyDecayUpdatesDecayUpdatedAt verifies that ApplyDecay sets DecayUpdatedAt.
func TestApplyDecayUpdatesDecayUpdatedAt(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	mem := &types.Memory{
		CreatedAt:  now.Add(-48 * time.Hour),
		Importance: 0.5,
	}

	dm.ApplyDecay(mem, now)

	if mem.DecayUpdatedAt == nil {
		t.Error("expected DecayUpdatedAt to be set by ApplyDecay")
	}
}

// TestApplyDecaySkipsIfUnchanged verifies that ApplyDecay does not update
// DecayUpdatedAt if the score has not changed meaningfully.
func TestApplyDecaySkipsIfUnchanged(t *testing.T) {
	dm := engine.NewDecayManager()
	now := time.Now()

	past := now.Add(-1 * time.Minute) // very recent sentinel time
	mem := &types.Memory{
		CreatedAt:      now.Add(-24 * time.Hour),
		Importance:     0.5,
		DecayUpdatedAt: &past,
	}

	// Calculate what the score should be and pre-set it to simulate a prior run.
	dm.ApplyDecay(mem, now)
	firstScore := mem.DecayScore
	firstUpdatedAt := mem.DecayUpdatedAt

	// Call ApplyDecay again with the same now — score should not change meaningfully.
	dm.ApplyDecay(mem, now)

	if mem.DecayScore != firstScore {
		t.Errorf("expected score unchanged on second call, got %f vs %f", mem.DecayScore, firstScore)
	}
	if mem.DecayUpdatedAt != firstUpdatedAt {
		t.Error("expected DecayUpdatedAt pointer to be unchanged when score did not change")
	}
}
