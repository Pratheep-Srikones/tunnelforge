package tunnel

import "sync"

type AgentRegistry struct {
	mu      sync.RWMutex
	tunnels map[string]*Tunnel
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		tunnels: make(map[string]*Tunnel),
	}
}

func (r *AgentRegistry) Register(t *Tunnel) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tunnels[t.Subdomain]; exists {
		return false
	}
	r.tunnels[t.Subdomain] = t
	return true
}

func (r *AgentRegistry) Get(subdomain string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tunnels[subdomain]
	return t, ok
}

func (r *AgentRegistry) Delete(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tunnels, subdomain)
}

func (r *AgentRegistry) Remove(t *Tunnel) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.tunnels[t.Subdomain]
	if !ok {
		return false
	}

	if current != t {
		return false
	}

	delete(r.tunnels, t.Subdomain)
	return true
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
	return len(r.tunnels)
}
