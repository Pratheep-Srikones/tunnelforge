package auth_test

import (
	"testing"
	"tunnelforge/server/auth"
)

func TestAuthRegistrySingleton(t *testing.T) {
	instance1 := auth.GetAuthRegistry()
	instance2 := auth.GetAuthRegistry()
	instance3 := auth.NewAuthRegistry()

	if instance1 != instance2 || instance1 != instance3 {
		t.Fatalf("Expected singleton instance equality, got %p, %p, %p", instance1, instance2, instance3)
	}
}

func TestAuthRegistryOperations(t *testing.T) {
	agentID := "agent-123"
	token := "secret-token-xyz"

	err := auth.Register(agentID, token)
	if err != nil {
		t.Fatalf("Unexpected error registering token: %v", err)
	}

	hashed, ok := auth.Get(agentID)
	if !ok || hashed == "" {
		t.Fatalf("Expected hashed token for %s, got ok=%v, hashed=%s", agentID, ok, hashed)
	}

	valid, err := auth.ValidateToken(agentID, token)
	if err != nil || !valid {
		t.Fatalf("Expected token to be valid, got valid=%v, err=%v", valid, err)
	}

	invalid, err := auth.ValidateToken(agentID, "wrong-token")
	if err != nil || invalid {
		t.Fatalf("Expected wrong token to be invalid, got valid=%v, err=%v", invalid, err)
	}

	auth.Delete(agentID)

	_, ok = auth.Get(agentID)
	if ok {
		t.Fatalf("Expected agent %s to be deleted from registry", agentID)
	}
}
