package tunnel

import "sync"

type Registry struct {
	mu sync.RWMutex
	tunnels map[string]*Tunnel
}

func NewRegistry() *Registry {
	return &Registry{
		tunnels: make(map[string]*Tunnel),
	}
}

func (r *Registry) Register(t *Tunnel) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tunnels[t.Subdomain]; exists {
		return false
	}
	r.tunnels[t.Subdomain] = t
	return true
}

func (r *Registry) Get(subdomain string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tunnels[subdomain]
	return t, ok
}

func (r *Registry) Delete(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tunnels, subdomain)
}

func (r *Registry) Remove(t *Tunnel) bool {
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

func (r *Registry) List() []*Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Tunnel, 0, len(r.tunnels))
	for _, t := range r.tunnels {
		list = append(list, t)
	}

	return list
}

func (r *Registry) Len() int {
	return len(r.tunnels)
}
