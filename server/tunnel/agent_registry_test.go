package tunnel

import "testing"

func TestRegisterAndGet(t *testing.T) {
	registry := NewAgentRegistry()

	tunnel := &Tunnel{
		ID:        "1",
		Subdomain: "test_app",
	}

	if !registry.Register(tunnel) {
		t.Fatal("expected registration to succeed")
	}

	got, ok := registry.Get("test_app")

	if !ok {
		t.Fatal("expected tunnel to exist")
	}

	if got != tunnel {
		t.Fatal("expected same tunnel")
	}
}

func TestDuplicateSubdomain(t *testing.T) {
	registry := NewAgentRegistry()

	first := &Tunnel{
		ID:        "1",
		Subdomain: "test_app",
	}

	second := &Tunnel{
		ID:        "2",
		Subdomain: "test_app",
	}

	if !registry.Register(first) {
		t.Fatal("first registration should succeed")
	}

	if registry.Register(second) {
		t.Fatal("second registration should fail")
	}
}

func TestRemoveDoesNotRemoveReplacement(t *testing.T) {
	registry := NewAgentRegistry()

	first := &Tunnel{
		ID:        "1",
		Subdomain: "test_app",
	}

	second := &Tunnel{
		ID:        "2",
		Subdomain: "test_app",
	}

	registry.Register(first)

	// Simulate replacement by removing first
	registry.Remove(first)

	registry.Register(second)

	// Old tunnel attempts cleanup
	registry.Remove(first)

	got, ok := registry.Get("test_app")

	if !ok {
		t.Fatal("second tunnel should still exist")
	}

	if got != second {
		t.Fatal("old tunnel removed the replacement")
	}
}
