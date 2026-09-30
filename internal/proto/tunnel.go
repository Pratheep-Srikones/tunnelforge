package proto

import "fmt"

type TunnelRegisterRequest struct {
	Type       string   `json:"type"`
	Subdomains []string `json:"subdomains"`
}

type TunnelRegisterResponse struct {
	Type    string                     `json:"type"`
	OK      bool                       `json:"ok"`
	Results map[string]SubdomainResult `json:"results,omitempty"`
	Message string                     `json:"message,omitempty"`
}

type SubdomainResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func (r TunnelRegisterRequest) Validate() error {
	if r.Type != "tunnel_register" {
		return fmt.Errorf("invalid message type: %q", r.Type)
	}

	if len(r.Subdomains) == 0 {
		return fmt.Errorf("at least one subdomain is required")
	}

	seen := make(map[string]struct{}, len(r.Subdomains))
	for _, sub := range r.Subdomains {
		if sub == "" {
			return fmt.Errorf("subdomain must not be empty")
		}

		if err := ValidateSubdomain(sub); err != nil {
			return fmt.Errorf("subdomain %q: %w", sub, err)
		}

		if _, dup := seen[sub]; dup {
			return fmt.Errorf("duplicate subdomain: %q", sub)
		}
		seen[sub] = struct{}{}
	}

	return nil
}
