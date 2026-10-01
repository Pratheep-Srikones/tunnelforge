package capture

import (
	"fmt"
	"sync"
)

const DefaultMaxRequests = 200

type RingBuffer struct {
	mu   sync.RWMutex
	size int
	data map[string][]*RequestEntry
}

func NewRingBuffer(max int) *RingBuffer {
	if max <= 0 {
		max = DefaultMaxRequests
	}

	return &RingBuffer{
		data: make(map[string][]*RequestEntry),
		size: max,
	}
}

func (r *RingBuffer) Push(subdomain string, re *RequestEntry) error {
	if subdomain == "" {
		return fmt.Errorf("subdomain is empty")
	}

	if re == nil {
		return fmt.Errorf("request entry is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.data == nil {
		return fmt.Errorf("ring buffer not initialized")
	}

	entries, ok := r.data[subdomain]
	if !ok {
		entries = make([]*RequestEntry, 0, r.size)
	}

	if len(entries) >= r.size {
		// Shift left to evict oldest entry at index 0
		copy(entries, entries[1:])
		entries[len(entries)-1] = re
	} else {
		entries = append(entries, re)
	}

	r.data[subdomain] = entries
	return nil
}

func (r *RingBuffer) List(subdomain string, limit int) ([]*RequestEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.data == nil {
		return nil, fmt.Errorf("ring buffer not initialized")
	}

	entries, ok := r.data[subdomain]
	if !ok || len(entries) == 0 {
		return []*RequestEntry{}, nil
	}

	start := 0
	if limit > 0 && limit < len(entries) {
		// Take the most recent 'limit' entries (from tail)
		start = len(entries) - limit
	}

	selected := entries[start:]
	out := make([]*RequestEntry, len(selected))
	copy(out, selected)

	return out, nil
}

func (r *RingBuffer) Get(subdomain string, id string) (*RequestEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.data == nil {
		return nil, fmt.Errorf("ring buffer not initialized")
	}

	entries, ok := r.data[subdomain]
	if !ok {
		return nil, fmt.Errorf("request not found")
	}

	for _, re := range entries {
		if re.ID == id {
			return re, nil
		}
	}

	return nil, fmt.Errorf("request not found")
}

func (r *RingBuffer) Remove(subdomain string, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.data == nil {
		return fmt.Errorf("ring buffer not initialized")
	}

	entries, ok := r.data[subdomain]
	if !ok {
		return fmt.Errorf("request not found")
	}

	for i, re := range entries {
		if re.ID == id {
			copy(entries[i:], entries[i+1:])
			entries[len(entries)-1] = nil // Avoid memory leak
			r.data[subdomain] = entries[:len(entries)-1]
			return nil
		}
	}

	return fmt.Errorf("request not found")
}

func (r *RingBuffer) Clear(subdomain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.data == nil {
		return fmt.Errorf("ring buffer not initialized")
	}

	delete(r.data, subdomain)
	return nil
}
