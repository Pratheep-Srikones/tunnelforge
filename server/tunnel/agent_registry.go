package tunnel

import (
	"fmt"
	"sync"

	"github.com/hashicorp/yamux"
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

// Register registers a tunnel with the registry.
// If the subdomain is already registered to a different agent, it returns an error.
// If the subdomain is already registered to the same agent, the existing tunnel
// is evicted and replaced (allowing seamless session reconnects/takeover), and
// any stale session is closed.
func (r *AgentRegistry) Register(t *Tunnel) error {
	if t == nil || t.Subdomain == "" {
		return fmt.Errorf("invalid tunnel")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var staleSession *yamux.Session
	if existing, exists := r.tunnels[t.Subdomain]; exists {
		if existing.AgentID != t.AgentID {
			return fmt.Errorf("subdomain '%s' is already in use", t.Subdomain)
		}
		// Same agent: evict the previous tunnel session if different
		if existing.Session != nil && existing.Session != t.Session {
			staleSession = existing.Session
		}
	}

	r.tunnels[t.Subdomain] = t

	if staleSession != nil {
		go staleSession.Close()
	}

	return nil
}

// Get gets a tunnel by subdomain
func (r *AgentRegistry) Get(subdomain string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tunnels[subdomain]
	return t, ok
}

// Remove removes a tunnel from the registry
func (r *AgentRegistry) Remove(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tunnels, subdomain)
}

// RemoveIfSame removes a tunnel from the registry if the subdomain and tunnel ID match
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
