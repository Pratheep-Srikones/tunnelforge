package tunnel

import "testing"

func TestRegisterAndGet(t *testing.T) {
	registry := NewAgentRegistry()

	tunnel := &Tunnel{
		ID:        "1",
		AgentID:   "agent_123",
		Subdomain: "test_app",
	}

	if err := registry.Register(tunnel); err != nil {
		t.Fatalf("expected registration to succeed, got: %v", err)
	}

	got, ok := registry.Get("test_app")
	if !ok {
		t.Fatal("expected tunnel to exist")
	}

	if got != tunnel {
		t.Fatal("expected same tunnel")
	}

	if got.AgentID != "agent_123" {
		t.Fatalf("expected agent_id 'agent_123', got '%s'", got.AgentID)
	}
}

func TestDuplicateSubdomain(t *testing.T) {
	registry := NewAgentRegistry()

	first := &Tunnel{
		ID:        "1",
		AgentID:   "agent_1",
		Subdomain: "test_app",
	}

	second := &Tunnel{
		ID:        "2",
		AgentID:   "agent_2",
		Subdomain: "test_app",
	}

	if err := registry.Register(first); err != nil {
		t.Fatalf("first registration should succeed, got: %v", err)
	}

	if err := registry.Register(second); err == nil {
		t.Fatal("second registration should fail for duplicate subdomain")
	}
}

func TestRemoveIfSameDoesNotRemoveReplacement(t *testing.T) {
	registry := NewAgentRegistry()

	first := &Tunnel{
		ID:        "1",
		AgentID:   "agent_1",
		Subdomain: "test_app",
	}

	second := &Tunnel{
		ID:        "2",
		AgentID:   "agent_1",
		Subdomain: "test_app",
	}

	if err := registry.Register(first); err != nil {
		t.Fatalf("failed to register first: %v", err)
	}

	// Simulate disconnect before reconnect
	registry.Remove("test_app")

	if err := registry.Register(second); err != nil {
		t.Fatalf("failed to register second: %v", err)
	}

	// Old tunnel attempts cleanup using RemoveIfSame
	removed := registry.RemoveIfSame("test_app", first.ID)
	if removed {
		t.Fatal("RemoveIfSame should not remove replacement tunnel")
	}

	got, ok := registry.Get("test_app")
	if !ok {
		t.Fatal("second tunnel should still exist")
	}

	if got != second {
		t.Fatal("old tunnel removed the replacement")
	}

	// Cleanup with correct ID succeeds
	if !registry.RemoveIfSame("test_app", second.ID) {
		t.Fatal("RemoveIfSame should remove matching tunnel")
	}

	if _, ok := registry.Get("test_app"); ok {
		t.Fatal("tunnel should be removed")
	}
}
