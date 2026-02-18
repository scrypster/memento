package llm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

// ErrCircuitOpen is returned when the circuit breaker is in open state
// and rejects requests to prevent cascading failures.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreakerConfig holds the configuration for the circuit breaker.
type CircuitBreakerConfig struct {
	// MaxFailures is the number of consecutive failures required to trip the circuit.
	// Default: 3
	MaxFailures uint32

	// Timeout is the duration the circuit stays open before transitioning to half-open.
	// Default: 30 seconds
	Timeout time.Duration

	// HalfOpenMaxSuccesses is the number of consecutive successes required in half-open
	// state to close the circuit again.
	// Default: 2
	HalfOpenMaxSuccesses uint32
}

// CircuitBreakerMetrics holds metrics about circuit breaker operations.
type CircuitBreakerMetrics struct {
	// TotalRequests is the total number of requests processed
	TotalRequests uint64

	// TotalSuccesses is the total number of successful requests
	TotalSuccesses uint64

	// TotalFailures is the total number of failed requests
	TotalFailures uint64

	// ConsecutiveSuccesses is the current count of consecutive successes
	ConsecutiveSuccesses uint32

	// ConsecutiveFailures is the current count of consecutive failures
	ConsecutiveFailures uint32
}

// CircuitBreaker wraps gobreaker to protect LLM calls from cascading failures.
// It implements the circuit breaker pattern with three states: closed, open, and half-open.
//
// When closed (normal operation), requests pass through normally.
// After MaxFailures consecutive failures, the circuit opens and rejects all requests.
// After Timeout duration, the circuit transitions to half-open and allows test requests.
// After HalfOpenMaxSuccesses successes in half-open state, the circuit closes again.
type CircuitBreaker struct {
	breaker *gobreaker.CircuitBreaker
	config  CircuitBreakerConfig
	mu      sync.RWMutex
	metrics CircuitBreakerMetrics
}

// NewCircuitBreaker creates a new circuit breaker with default configuration:
// - MaxFailures: 3
// - Timeout: 30 seconds
// - HalfOpenMaxSuccesses: 2
func NewCircuitBreaker() *CircuitBreaker {
	return NewCircuitBreakerWithConfig(CircuitBreakerConfig{
		MaxFailures:          3,
		Timeout:              30 * time.Second,
		HalfOpenMaxSuccesses: 2,
	})
}

// NewCircuitBreakerWithConfig creates a new circuit breaker with custom configuration.
func NewCircuitBreakerWithConfig(config CircuitBreakerConfig) *CircuitBreaker {
	cb := &CircuitBreaker{
		config: config,
	}

	settings := gobreaker.Settings{
		Name:        "LLMCircuitBreaker",
		MaxRequests: config.HalfOpenMaxSuccesses,
		Interval:    0, // Don't clear counts periodically
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= config.MaxFailures
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Optional: Add logging here if needed
		},
	}

	cb.breaker = gobreaker.NewCircuitBreaker(settings)
	return cb
}

// Execute runs the given function through the circuit breaker.
// If the circuit is open, it returns ErrCircuitOpen immediately.
// The function should return (result, error) where error indicates failure.
//
// Context is passed through for cancellation support.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		cb.recordFailure()
		return nil, ctx.Err()
	default:
	}

	result, err := cb.breaker.Execute(func() (interface{}, error) {
		// Check context again before executing
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		return fn()
	})

	// Update metrics
	if err != nil {
		cb.recordFailure()
		// Check if error is due to open circuit
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, ErrCircuitOpen
		}
	} else {
		cb.recordSuccess()
	}

	return result, err
}

// HealthCheck executes a health check function with the given context.
// The context should include a timeout (typically 2 seconds as per requirements).
//
// Example:
//   ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
//   defer cancel()
//   err := cb.HealthCheck(ctx, func() error { return ping() })
func (cb *CircuitBreaker) HealthCheck(ctx context.Context, checkFn func() error) error {
	// Run health check in a goroutine to support context cancellation
	errChan := make(chan error, 1)

	go func() {
		errChan <- checkFn()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// State returns the current state of the circuit breaker.
// Possible values: "closed", "open", "half-open"
func (cb *CircuitBreaker) State() string {
	state := cb.breaker.State()
	switch state {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Metrics returns the current metrics for the circuit breaker.
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Also get counts from gobreaker
	counts := cb.breaker.Counts()

	return CircuitBreakerMetrics{
		TotalRequests:        cb.metrics.TotalRequests,
		TotalSuccesses:       cb.metrics.TotalSuccesses,
		TotalFailures:        cb.metrics.TotalFailures,
		ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
		ConsecutiveFailures:  counts.ConsecutiveFailures,
	}
}

// recordSuccess updates metrics for a successful request.
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.TotalRequests++
	cb.metrics.TotalSuccesses++
}

// recordFailure updates metrics for a failed request.
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.TotalRequests++
	cb.metrics.TotalFailures++
}
