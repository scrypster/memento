package engine

import (
	"math"
	"testing"
	"time"
)

func TestComputeDecayScore_Fresh(t *testing.T) {
	score := ComputeDecayScore(1.0, time.Now(), 0)
	if score < 0.99 || score > 1.0 {
		t.Errorf("Fresh memory should have score near 1.0, got %f", score)
	}
}

func TestComputeDecayScore_Old(t *testing.T) {
	// 60 days old should be roughly half the base score
	old := time.Now().Add(-60 * 24 * time.Hour)
	score := ComputeDecayScore(1.0, old, 0)
	if score > 0.6 {
		t.Errorf("60-day-old memory should have score below 0.6, got %f", score)
	}
}

func TestComputeDecayScore_WithAccessCount(t *testing.T) {
	old := time.Now().Add(-60 * 24 * time.Hour)
	// Without access count
	scoreNoAccess := ComputeDecayScore(1.0, old, 0)
	// With access count should be boosted
	scoreWithAccess := ComputeDecayScore(1.0, old, 10)
	if scoreWithAccess <= scoreNoAccess {
		t.Errorf("Access count should boost score: no_access=%f, with_access=%f", scoreNoAccess, scoreWithAccess)
	}
}

func TestComputeDecayScore_ClampsToOne(t *testing.T) {
	// Fresh memory with high access count should clamp to 1.0
	score := ComputeDecayScore(1.0, time.Now(), 100)
	if math.Abs(score-1.0) > 0.001 {
		t.Errorf("Score should clamp to 1.0, got %f", score)
	}
}

func TestComputeDecayScore_ClampsToZero(t *testing.T) {
	// Very old memory should clamp to 0.0
	veryOld := time.Now().Add(-365 * 24 * time.Hour)
	score := ComputeDecayScore(0.1, veryOld, 0)
	if score < 0.0 || score > 0.1 {
		t.Errorf("Very old memory should have score near 0.0, got %f", score)
	}
}

func TestDecayScoreAfterAccess_Boosts(t *testing.T) {
	initial := 0.5
	boosted := DecayScoreAfterAccess(initial)
	if boosted <= initial {
		t.Errorf("Access should boost score: initial=%f, boosted=%f", initial, boosted)
	}
	if boosted > 1.0 {
		t.Errorf("Score should not exceed 1.0, got %f", boosted)
	}
}

func TestDecayScoreAfterAccess_CapsAt1(t *testing.T) {
	score := DecayScoreAfterAccess(0.99)
	if math.Abs(score-1.0) > 0.001 {
		t.Errorf("Score capped at 1.0, got %f", score)
	}
}

func TestDecayScoreAfterAccess_BoostAmount(t *testing.T) {
	initial := 0.5
	boosted := DecayScoreAfterAccess(initial)
	expected := initial + accessBoost
	if math.Abs(boosted-expected) > 0.001 {
		t.Errorf("Boost should be %f, got %f", expected, boosted)
	}
}
