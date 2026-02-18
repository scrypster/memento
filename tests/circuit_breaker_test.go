package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/scrypster/memento/internal/llm"
)

// TestCircuitBreakerClosed verifies that the circuit breaker allows requests
// to pass through when in the closed state (normal operation).
func TestCircuitBreakerClosed(t *testing.T) {
	cb := llm.NewCircuitBreaker()
	ctx := context.Background()

	// Successful operation should work
	successFunc := func() (interface{}, error) {
		return "success", nil
	}

	result, err := cb.Execute(ctx, successFunc)
	if err != nil {
		t.Fatalf("Expected successful execution in closed state, got error: %v", err)
	}

	if result != "success" {
		t.Fatalf("Expected result 'success', got: %v", result)
	}

	// Circuit should still be closed
	state := cb.State()
	if state != "closed" {
		t.Fatalf("Expected circuit to be closed, got: %s", state)
	}
}

// TestCircuitBreakerOpen verifies that after 3 consecutive failures,
// the circuit breaker transitions to the open state and rejects requests.
func TestCircuitBreakerOpen(t *testing.T) {
	cb := llm.NewCircuitBreaker()
	ctx := context.Background()

	// Function that always fails
	failFunc := func() (interface{}, error) {
		return nil, errors.New("operation failed")
	}

	// Execute 3 times to trigger circuit breaker (maxFailures = 3)
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(ctx, failFunc)
		if err == nil {
			t.Fatalf("Expected error on attempt %d", i+1)
		}
	}

	// Circuit should now be open
	state := cb.State()
	if state != "open" {
		t.Fatalf("Expected circuit to be open after 3 failures, got: %s", state)
	}

	// Further requests should be rejected immediately
	_, err := cb.Execute(ctx, failFunc)
	if err == nil {
		t.Fatal("Expected circuit breaker to reject request in open state")
	}
	if !errors.Is(err, llm.ErrCircuitOpen) {
		t.Fatalf("Expected ErrCircuitOpen, got: %v", err)
	}
}

// TestCircuitBreakerHalfOpen verifies that after the timeout period,
// the circuit breaker transitions to half-open state and allows test requests.
func TestCircuitBreakerHalfOpen(t *testing.T) {
	// Create circuit breaker with short timeout for testing
	cb := llm.NewCircuitBreakerWithConfig(llm.CircuitBreakerConfig{
		MaxFailures:         3,
		Timeout:             100 * time.Millisecond, // Short timeout for testing
		HalfOpenMaxSuccesses: 2,
	})
	ctx := context.Background()

	// Trigger circuit breaker to open
	failFunc := func() (interface{}, error) {
		return nil, errors.New("operation failed")
	}

	for i := 0; i < 3; i++ {
		cb.Execute(ctx, failFunc)
	}

	// Verify it's open
	if state := cb.State(); state != "open" {
		t.Fatalf("Expected circuit to be open, got: %s", state)
	}

	// Wait for timeout to allow transition to half-open using polling
	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	halfOpenReached := false
	for !halfOpenReached {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for circuit to transition to half-open")
		case <-ticker.C:
			if cb.State() == "half-open" {
				halfOpenReached = true
			}
		}
	}

	// Next request should transition to half-open
	successFunc := func() (interface{}, error) {
		return "success", nil
	}

	// First success in half-open state
	_, err := cb.Execute(ctx, successFunc)
	if err != nil {
		t.Fatalf("Expected successful execution in half-open state, got: %v", err)
	}

	// After HalfOpenMaxSuccesses (2) successful requests, circuit should close
	_, err = cb.Execute(ctx, successFunc)
	if err != nil {
		t.Fatalf("Expected successful execution, got: %v", err)
	}

	// Circuit should now be closed again
	state := cb.State()
	if state != "closed" {
		t.Fatalf("Expected circuit to be closed after successful recovery, got: %s", state)
	}
}

// TestCircuitBreakerTimeout verifies that the health check function
// respects the timeout and fails appropriately.
func TestCircuitBreakerTimeout(t *testing.T) {
	cb := llm.NewCircuitBreaker()

	// Create a health check that takes longer than the 2s timeout
	slowHealthCheck := func() error {
		time.Sleep(3 * time.Second)
		return nil
	}

	// Create a context with 2s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cb.HealthCheck(ctx, slowHealthCheck)
	if err == nil {
		t.Fatal("Expected health check to timeout")
	}

	// Should be a context deadline exceeded error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Expected context.DeadlineExceeded error, got: %v", err)
	}
}

// TestCircuitBreakerHealthCheckSuccess verifies that a successful
// health check completes without error.
func TestCircuitBreakerHealthCheckSuccess(t *testing.T) {
	cb := llm.NewCircuitBreaker()

	// Quick health check that succeeds
	healthCheck := func() error {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cb.HealthCheck(ctx, healthCheck)
	if err != nil {
		t.Fatalf("Expected successful health check, got: %v", err)
	}
}

// TestCircuitBreakerHealthCheckFailure verifies that a failed
// health check returns the error.
func TestCircuitBreakerHealthCheckFailure(t *testing.T) {
	cb := llm.NewCircuitBreaker()

	// Health check that fails
	expectedErr := errors.New("health check failed")
	healthCheck := func() error {
		return expectedErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := cb.HealthCheck(ctx, healthCheck)
	if err == nil {
		t.Fatal("Expected health check to fail")
	}
	if err != expectedErr {
		t.Fatalf("Expected error %v, got: %v", expectedErr, err)
	}
}

// TestCircuitBreakerContextCancellation verifies that context cancellation
// is properly handled during execution.
func TestCircuitBreakerContextCancellation(t *testing.T) {
	cb := llm.NewCircuitBreaker()

	// Create a context and cancel it immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Function that checks context
	checkFunc := func() (interface{}, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return "success", nil
		}
	}

	_, err := cb.Execute(ctx, checkFunc)
	if err == nil {
		t.Fatal("Expected error due to cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Expected context.Canceled error, got: %v", err)
	}
}

// TestCircuitBreakerMetrics verifies that circuit breaker metrics
// are tracked correctly.
func TestCircuitBreakerMetrics(t *testing.T) {
	cb := llm.NewCircuitBreaker()
	ctx := context.Background()

	successFunc := func() (interface{}, error) {
		return "success", nil
	}

	failFunc := func() (interface{}, error) {
		return nil, errors.New("failure")
	}

	// Execute some successful and failed requests
	cb.Execute(ctx, successFunc)
	cb.Execute(ctx, successFunc)
	cb.Execute(ctx, failFunc)

	metrics := cb.Metrics()

	if metrics.TotalRequests != 3 {
		t.Fatalf("Expected 3 total requests, got: %d", metrics.TotalRequests)
	}
	if metrics.TotalSuccesses != 2 {
		t.Fatalf("Expected 2 successful requests, got: %d", metrics.TotalSuccesses)
	}
	if metrics.TotalFailures != 1 {
		t.Fatalf("Expected 1 failed request, got: %d", metrics.TotalFailures)
	}
}
