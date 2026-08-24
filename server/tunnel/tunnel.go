package tunnel

import (
	"time"

	"github.com/hashicorp/yamux"
)

type Tunnel struct {
	ID        string
	Subdomain string
	Session   *yamux.Session
	CreatedAt time.Time
}
