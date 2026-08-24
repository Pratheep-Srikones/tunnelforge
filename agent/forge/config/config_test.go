package config_test

import (
	"testing"
	"tunnelforge/agent/forge/config"
)

func TestConfigOperations(t *testing.T) {
	if err := config.InitViperConfig(); err != nil {
		t.Fatalf("InitViperConfig failed: %v", err)
	}

	// Test Set and GetString
	if err := config.Set("agent_id", "agent-test-uuid"); err != nil {
		t.Fatalf("Set agent_id failed: %v", err)
	}
	if val := config.GetString("agent_id"); val != "agent-test-uuid" {
		t.Errorf("Expected agent_id 'agent-test-uuid', got '%s'", val)
	}

	// Test Set and GetInt
	if err := config.Set("port", 8080); err != nil {
		t.Fatalf("Set port failed: %v", err)
	}
	if val := config.GetInt("port"); val != 8080 {
		t.Errorf("Expected port 8080, got %d", val)
	}

	// Test Set and GetBool
	if err := config.Set("debug", true); err != nil {
		t.Fatalf("Set debug failed: %v", err)
	}
	if val := config.GetBool("debug"); !val {
		t.Errorf("Expected debug true, got false")
	}

	// Test Delete
	if err := config.Delete("debug"); err != nil {
		t.Fatalf("Delete debug failed: %v", err)
	}
	if val := config.GetBool("debug"); val != false {
		t.Errorf("Expected debug false after delete, got %v", val)
	}
}
