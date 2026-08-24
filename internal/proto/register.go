package proto

type RegisterRequest struct {
	EnrollmentKey string `json:"enrollment_key"`
}

type RegisterResponse struct {
	AgentID string `json:"agent_id"`
	Token   string `json:"token"`
}
