// internal/adapter/manager.go
package adapter

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrAdapterNotFound = errors.New("adapter not found")
	ErrNoHealthy       = errors.New("no healthy adapter available")
	ErrRateLimited     = errors.New("adapter rate limited")
)

// AdapterInfo describes a registered adapter's status.
type AdapterInfo struct {
	Name         string
	Healthy      bool
	Primary      bool
	Capabilities AdapterCaps
}

// rateLimiter tracks per-adapter call rate.
type rateLimiter struct {
	maxPerSec  int
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

func newRateLimiter(maxPerSec int) *rateLimiter {
	return &rateLimiter{
		maxPerSec:  maxPerSec,
		tokens:     maxPerSec,
		lastRefill: time.Now(),
	}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	if elapsed >= time.Second {
		r.tokens = r.maxPerSec
		r.lastRefill = now
	}
	if r.tokens <= 0 {
		return false
	}
	r.tokens--
	return true
}

// entry holds a registered adapter and its metadata.
type entry struct {
	adapter TradingAdapter
	healthy bool
	limiter *rateLimiter
}

// AdapterManager is a thread-safe registry of TradingAdapters with health
// tracking, primary/fallback selection, and per-adapter rate limiting.
type AdapterManager struct {
	mu          sync.RWMutex
	adapters    map[string]*entry
	primaryName string
	order       []string // insertion order for deterministic fallback
	rateLimit   int      // requests per second per adapter
}

// NewAdapterManager creates a manager. ratePerSec sets the per-adapter rate
// limit (requests/second). Use 0 for unlimited.
func NewAdapterManager(ratePerSec int) *AdapterManager {
	return &AdapterManager{
		adapters:  make(map[string]*entry),
		rateLimit: ratePerSec,
	}
}

// Register adds an adapter. The first registered adapter becomes the primary.
func (m *AdapterManager) Register(name string, a TradingAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lim *rateLimiter
	if m.rateLimit > 0 {
		lim = newRateLimiter(m.rateLimit)
	}

	// If already registered, replace but keep order position.
	if _, exists := m.adapters[name]; !exists {
		m.order = append(m.order, name)
	}

	m.adapters[name] = &entry{
		adapter: a,
		healthy: a.IsConnected(),
		limiter: lim,
	}

	if m.primaryName == "" {
		m.primaryName = name
	}
}

// Unregister removes an adapter by name.
func (m *AdapterManager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.adapters, name)

	for i, n := range m.order {
		if n == name {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}

	if m.primaryName == name {
		m.primaryName = ""
		if len(m.order) > 0 {
			m.primaryName = m.order[0]
		}
	}
}

// Get returns an adapter by name. Returns ErrRateLimited if the adapter's
// rate limiter rejects the call.
func (m *AdapterManager) Get(name string) (TradingAdapter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.adapters[name]
	if !ok {
		return nil, ErrAdapterNotFound
	}
	if e.limiter != nil && !e.limiter.Allow() {
		return nil, ErrRateLimited
	}
	return e.adapter, nil
}

// SetPrimary designates an adapter as the primary.
func (m *AdapterManager) SetPrimary(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.adapters[name]; ok {
		m.primaryName = name
	}
}

// GetPrimary returns the primary adapter if healthy, otherwise the first
// healthy fallback in registration order.
func (m *AdapterManager) GetPrimary() (TradingAdapter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try the designated primary first.
	if e, ok := m.adapters[m.primaryName]; ok && e.healthy {
		return e.adapter, nil
	}

	// Fallback: first healthy adapter in insertion order.
	for _, name := range m.order {
		if e := m.adapters[name]; e.healthy {
			return e.adapter, nil
		}
	}

	return nil, ErrNoHealthy
}

// List returns info about every registered adapter.
func (m *AdapterManager) List() []AdapterInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]AdapterInfo, 0, len(m.order))
	for _, name := range m.order {
		e := m.adapters[name]
		out = append(out, AdapterInfo{
			Name:         name,
			Healthy:      e.healthy,
			Primary:      name == m.primaryName,
			Capabilities: e.adapter.Capabilities(),
		})
	}
	return out
}

// HealthCheck pings every registered adapter and updates health status.
func (m *AdapterManager) HealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.adapters {
		e.healthy = e.adapter.IsConnected()
	}
}

// SetHealthy allows manually setting an adapter's health status (useful for
// tests and external health probes).
func (m *AdapterManager) SetHealthy(name string, healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.adapters[name]; ok {
		e.healthy = healthy
	}
}

// Len returns the number of registered adapters.
func (m *AdapterManager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.adapters)
}
