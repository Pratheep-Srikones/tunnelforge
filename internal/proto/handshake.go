package proto

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