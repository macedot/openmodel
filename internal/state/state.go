// Package state manages failure tracking and model availability
package state

import (
	"math/rand"
	"sync"
)

// State manages model failure tracking
type State struct {
	mu                sync.RWMutex
	failureCounts     map[string]int
	unavailableModels map[string]bool
	currentTimeout    int
	cycle             int
	roundRobinIndex   map[string]int // Tracks round-robin position per model
	rand              *rand.Rand     // Reusable random generator
}

// New creates a new State
func New(initialTimeout int) *State {
	return &State{
		failureCounts:     make(map[string]int),
		unavailableModels: make(map[string]bool),
		currentTimeout:    initialTimeout,
		roundRobinIndex:   make(map[string]int),
		rand:              rand.New(rand.NewSource(1)), // Seeded for reproducibility
	}
}

// RecordFailure records a failure for a model
func (s *State) RecordFailure(model string, threshold int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureCounts[model]++
	if s.failureCounts[model] >= threshold {
		s.unavailableModels[model] = true
	}
}

// IsAvailable checks if a model is available
func (s *State) IsAvailable(model string, threshold int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unavailableModels[model] {
		return false
	}
	return s.failureCounts[model] < threshold
}

// ResetModel resets a model's failure count
func (s *State) ResetModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failureCounts, model)
	delete(s.unavailableModels, model)
}

// GetProgressiveTimeout returns the current progressive timeout
func (s *State) GetProgressiveTimeout() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentTimeout
}

// IncrementTimeout doubles the timeout (up to max)
func (s *State) IncrementTimeout(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentTimeout *= 2
	if s.currentTimeout > max {
		s.currentTimeout = max
	}
	s.cycle++
}

// NextRoundRobin returns the next index for round-robin selection for a model
// total is the total number of available providers
func (s *State) NextRoundRobin(model string, total int) int {
	if total <= 1 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.roundRobinIndex[model]
	next := (current + 1) % total
	s.roundRobinIndex[model] = next
	return next
}

// GetRandomIndex returns a random index between 0 and total-1
func (s *State) GetRandomIndex(total int) int {
	if total <= 1 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rand.Intn(total)
}

// ResetRoundRobin resets the round-robin index for a model
func (s *State) ResetRoundRobin(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.roundRobinIndex, model)
}
