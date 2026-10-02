package tunnel

import (
	"time"

	"github.com/hashicorp/yamux"
)

type Tunnel struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	Subdomain string         `json:"subdomain"`
	Session   *yamux.Session `json:"-"`
	CreatedAt time.Time      `json:"connected_at"`
}
