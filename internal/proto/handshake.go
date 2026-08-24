package proto

import (
	"fmt"
	"regexp"
)

type HandshakeRequest struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	Subdomain string `json:"subdomain"`
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

	if r.Subdomain == "" {
		return fmt.Errorf(
			"subdomain is required",
		)
	}

	if err := ValidateSubdomain(r.Subdomain); err != nil {
		return fmt.Errorf(
			"subdomain validation: %w",
			err,
		)
	}

	if r.Token == "" {
		return fmt.Errorf(
			"token is required",
		)
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
