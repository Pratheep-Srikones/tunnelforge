package tunnel

import (
	"fmt"
	"sync"
)

type AgentRegistry struct {
	mu      sync.RWMutex
	tunnels map[string]*Tunnel
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		tunnels: make(map[string]*Tunnel),
	}
}

func (r *AgentRegistry) Register(t *Tunnel) error {
	if t == nil || t.Subdomain == "" {
		return fmt.Errorf("invalid tunnel")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tunnels[t.Subdomain]; exists {
		return fmt.Errorf("subdomain '%s' is already in use", t.Subdomain)
	}

	r.tunnels[t.Subdomain] = t
	return nil
}

func (r *AgentRegistry) Get(subdomain string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tunnels[subdomain]
	return t, ok
}

func (r *AgentRegistry) Remove(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tunnels, subdomain)
}

func (r *AgentRegistry) RemoveIfSame(subdomain, tunnelID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.tunnels[subdomain]
	if !ok {
		return false
	}

	if current.ID != tunnelID {
		return false
	}

	delete(r.tunnels, subdomain)
	return true
}

func (r *AgentRegistry) RemoveTunnel(t *Tunnel) bool {
	if t == nil {
		return false
	}
	return r.RemoveIfSame(t.Subdomain, t.ID)
}

func (r *AgentRegistry) List() []*Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Tunnel, 0, len(r.tunnels))
	for _, t := range r.tunnels {
		list = append(list, t)
	}

	return list
}

func (r *AgentRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tunnels)
}
