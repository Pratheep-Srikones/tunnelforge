package proto

import (
	"fmt"
	"regexp"
)

type HandshakeRequest struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	AgentID   string `json:"agent_id"`
	Subdomain string `json:"subdomain,omitempty"`
}

type HandshakeResponse struct {
	Type    string `json:"type"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

var subdomainPattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`,
)

func (r HandshakeRequest) Validate() error {
	if r.Type != "handshake" {
		return fmt.Errorf(
			"invalid message type: %q",
			r.Type,
		)
	}

	if r.Token == "" {
		return fmt.Errorf(
			"token is required",
		)
	}

	if r.AgentID == "" {
		return fmt.Errorf(
			"agent_id is required",
		)
	}

	if r.Subdomain != "" {
		if err := ValidateSubdomain(r.Subdomain); err != nil {
			return fmt.Errorf(
				"subdomain validation: %w",
				err,
			)
		}
	}

	return nil
}

func ValidateSubdomain(subdomain string) error {
	if !subdomainPattern.MatchString(subdomain) {
		return fmt.Errorf(
			"invalid subdomain: %q",
			subdomain,
		)
	}

	return nil
}
