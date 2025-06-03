package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

type Metrics interface {
	IncrementCounter(name string)
	IncrementCounterWithLabels(name string, labels map[string]string)
	RecordDuration(name string, duration time.Duration)
	RecordGauge(name string, value float64)
}

// Simple in-memory metrics implementation
type InMemoryMetrics struct {
	counters   map[string]*int64
	gauges     map[string]*int64 // Store as int64
	gaugeMutex sync.RWMutex      // Add mutex for gauges
}

func NewInMemoryMetrics() *InMemoryMetrics {
	return &InMemoryMetrics{
		counters: make(map[string]*int64),
		gauges:   make(map[string]*int64),
	}
}

func (m *InMemoryMetrics) IncrementCounter(name string) {
	if _, exists := m.counters[name]; !exists {
		m.counters[name] = new(int64)
	}
	atomic.AddInt64(m.counters[name], 1)
}

func (m *InMemoryMetrics) IncrementCounterWithLabels(name string, labels map[string]string) {
	// For simplicity, just use the name for now
	// In production, you'd want to include labels in the key
	m.IncrementCounter(name)
}

func (m *InMemoryMetrics) RecordDuration(name string, duration time.Duration) {
	// Convert to milliseconds
	m.RecordGauge(name+"_duration_ms", float64(duration.Nanoseconds())/1e6)
}

func (m *InMemoryMetrics) GetCounters() map[string]int64 {
	result := make(map[string]int64)
	for name, counter := range m.counters {
		result[name] = atomic.LoadInt64(counter)
	}
	return result
}

func (m *InMemoryMetrics) RecordGauge(name string, value float64) {
	m.gaugeMutex.Lock()
	defer m.gaugeMutex.Unlock()

	if _, exists := m.gauges[name]; !exists {
		m.gauges[name] = new(int64)
	}
	// Convert float64 to int64 (losing precision but simpler)
	atomic.StoreInt64(m.gauges[name], int64(value))
}

func (m *InMemoryMetrics) GetGauges() map[string]float64 {
	m.gaugeMutex.RLock()
	defer m.gaugeMutex.RUnlock()

	result := make(map[string]float64)
	for name, gauge := range m.gauges {
		result[name] = float64(atomic.LoadInt64(gauge))
	}
	return result
}
