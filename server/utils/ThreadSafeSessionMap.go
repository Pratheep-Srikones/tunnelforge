package utils

import (
	"sync"

	"github.com/hashicorp/yamux"
)

type ThreadSafeSessionMap struct {
	mu    sync.RWMutex
	items map[string]*yamux.Session
}

// NewThreadSafeConnMap creates a new thread-safe map
func NewThreadSafeSessionMap() *ThreadSafeSessionMap {
	return &ThreadSafeSessionMap{
		items: make(map[string]*yamux.Session),
	}
}

// Store adds or updates an item in the map
func (m *ThreadSafeSessionMap) Store(key string, value *yamux.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
}

// Load retrieves an item from the map
func (m *ThreadSafeSessionMap) Load(key string) (*yamux.Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.items[key]
	return value, ok
}

// Delete removes an item from the map
func (m *ThreadSafeSessionMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
}

// Range iterates over the map items in a thread-safe manner
func (m *ThreadSafeSessionMap) Range(f func(key string, value *yamux.Session) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.items {
		if !f(k, v) {
			break
		}
	}
}
