package config

import (
	"time"

	"github.com/hashicorp/yamux"
)

func YamuxConfig() *yamux.Config {
	config := yamux.DefaultConfig()
	config.EnableKeepAlive = true
	config.KeepAliveInterval = 5 * time.Second
	return config
}