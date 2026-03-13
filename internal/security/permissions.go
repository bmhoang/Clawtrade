package security

import (
	"sort"
	"sync"
)

const wildcard = "*"

// Permission represents a single granted permission.
type Permission struct {
	Action   string `json:"action"`
	Exchange string `json:"exchange"`
	Symbol   string `json:"symbol"`
}

// PermissionMatrix implements a default-deny permission system for
// (action, exchange, symbol) tuples.
type PermissionMatrix struct {
	mu    sync.RWMutex
	perms map[string]struct{} // set of "action|exchange|symbol" keys
}

// NewPermissionMatrix returns an empty permission matrix (default-deny).
func NewPermissionMatrix() *PermissionMatrix {
	return &PermissionMatrix{
		perms: make(map[string]struct{}),
	}
}

func permKey(action, exchange, symbol string) string {
	return action + "|" + exchange + "|" + symbol
}

// Grant allows the given (action, exchange, symbol) combination.
func (pm *PermissionMatrix) Grant(action, exchange, symbol string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.perms[permKey(action, exchange, symbol)] = struct{}{}
}

// Revoke removes the given (action, exchange, symbol) permission.
func (pm *PermissionMatrix) Revoke(action, exchange, symbol string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.perms, permKey(action, exchange, symbol))
}

// GrantAll grants a wildcard permission for the given action on all exchanges
// and symbols.
func (pm *PermissionMatrix) GrantAll(action string) {
	pm.Grant(action, wildcard, wildcard)
}

// Check returns true if the (action, exchange, symbol) is allowed.
// It checks in order: exact match, action+exchange+*, action+*+*, *+*+*.
func (pm *PermissionMatrix) Check(action, exchange, symbol string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	candidates := []string{
		permKey(action, exchange, symbol),
		permKey(action, exchange, wildcard),
		permKey(action, wildcard, wildcard),
		permKey(wildcard, wildcard, wildcard),
	}
	for _, k := range candidates {
		if _, ok := pm.perms[k]; ok {
			return true
		}
	}
	return false
}

// ListPermissions returns all granted permissions sorted lexicographically.
func (pm *PermissionMatrix) ListPermissions() []Permission {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	keys := make([]string, 0, len(pm.perms))
	for k := range pm.perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	perms := make([]Permission, 0, len(keys))
	for _, k := range keys {
		// Parse "action|exchange|symbol"
		parts := splitPermKey(k)
		perms = append(perms, Permission{
			Action:   parts[0],
			Exchange: parts[1],
			Symbol:   parts[2],
		})
	}
	return perms
}

// splitPermKey splits a "a|b|c" key back into three parts.
func splitPermKey(key string) [3]string {
	var parts [3]string
	idx := 0
	start := 0
	for i, ch := range key {
		if ch == '|' {
			parts[idx] = key[start:i]
			idx++
			start = i + 1
		}
	}
	parts[idx] = key[start:]
	return parts
}
