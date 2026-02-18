package llm_test

import (
	"context"
	"fmt"
	"time"

	"github.com/scrypster/memento/internal/llm"
)

// ExampleCircuitBreaker demonstrates basic usage of the circuit breaker.
func ExampleCircuitBreaker() {
	// Create a new circuit breaker with default settings
	cb := llm.NewCircuitBreaker()

	// Execute an LLM call through the circuit breaker
	result, err := cb.Execute(context.Background(), func() (interface{}, error) {
		// Your LLM API call here
		return "response from LLM", nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Result: %v\n", result)
	// Output: Result: response from LLM
}

// ExampleCircuitBreaker_customConfig demonstrates creating a circuit breaker
// with custom configuration.
func ExampleCircuitBreaker_customConfig() {
	// Create circuit breaker with custom settings
	cb := llm.NewCircuitBreakerWithConfig(llm.CircuitBreakerConfig{
		MaxFailures:          5,              // Allow 5 failures before opening
		Timeout:              60 * time.Second, // Stay open for 60 seconds
		HalfOpenMaxSuccesses: 3,              // Require 3 successes to close
	})

	// Use the circuit breaker
	_, err := cb.Execute(context.Background(), func() (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// ExampleCircuitBreaker_healthCheck demonstrates using the health check function.
func ExampleCircuitBreaker_HealthCheck() {
	cb := llm.NewCircuitBreaker()

	// Create context with 2 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run a health check
	err := cb.HealthCheck(ctx, func() error {
		// Your health check logic here (e.g., ping LLM API)
		return nil
	})

	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
		return
	}

	fmt.Println("Health check passed")
	// Output: Health check passed
}

// ExampleCircuitBreaker_State demonstrates checking the circuit breaker state.
func ExampleCircuitBreaker_State() {
	cb := llm.NewCircuitBreaker()

	// Check current state
	state := cb.State()
	fmt.Printf("Circuit breaker state: %s\n", state)
	// Output: Circuit breaker state: closed
}

// ExampleCircuitBreaker_Metrics demonstrates accessing circuit breaker metrics.
func ExampleCircuitBreaker_Metrics() {
	cb := llm.NewCircuitBreaker()
	ctx := context.Background()

	// Execute some requests
	cb.Execute(ctx, func() (interface{}, error) {
		return "success", nil
	})

	// Get metrics
	metrics := cb.Metrics()
	fmt.Printf("Total requests: %d\n", metrics.TotalRequests)
	fmt.Printf("Total successes: %d\n", metrics.TotalSuccesses)
	// Output: Total requests: 1
	// Total successes: 1
}
