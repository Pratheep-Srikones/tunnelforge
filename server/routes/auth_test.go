package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"tunnelforge/internal/proto"

	"github.com/gin-gonic/gin"
)

func TestEnrollmentKeyResolution(t *testing.T) {
	// Test 1: Default fallback
	os.Unsetenv("FORGE_ENROLLMENT_KEY")
	key := InitEnrollmentKey("")
	if key != DefaultEnrollmentKey {
		t.Fatalf("expected default key %s, got %s", DefaultEnrollmentKey, key)
	}
	if !IsUsingDefaultEnrollmentKey() {
		t.Fatalf("expected IsUsingDefaultEnrollmentKey to be true")
	}

	// Test 2: Env var fallback
	os.Setenv("FORGE_ENROLLMENT_KEY", "env_secret_key_123")
	defer os.Unsetenv("FORGE_ENROLLMENT_KEY")

	key = InitEnrollmentKey("")
	if key != "env_secret_key_123" {
		t.Fatalf("expected env key env_secret_key_123, got %s", key)
	}
	if IsUsingDefaultEnrollmentKey() {
		t.Fatalf("expected IsUsingDefaultEnrollmentKey to be false when env var is set")
	}

	// Test 3: CLI flag override beats env var
	key = InitEnrollmentKey("cli_override_key_999")
	if key != "cli_override_key_999" {
		t.Fatalf("expected cli override key cli_override_key_999, got %s", key)
	}
	if IsUsingDefaultEnrollmentKey() {
		t.Fatalf("expected IsUsingDefaultEnrollmentKey to be false when CLI flag is set")
	}
}

func TestRegisterAuthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/forge/internal")
	UseAuthRoutes(group)

	InitEnrollmentKey("my_custom_secret_key")

	// Test Invalid key
	invalidReq := proto.RegisterRequest{EnrollmentKey: "wrong_key"}
	body, _ := json.Marshal(invalidReq)
	req, _ := http.NewRequest(http.MethodPost, "/forge/internal/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized for invalid key, got %d", w.Code)
	}

	// Test Valid key
	validReq := proto.RegisterRequest{EnrollmentKey: "my_custom_secret_key"}
	body, _ = json.Marshal(validReq)
	req, _ = http.NewRequest(http.MethodPost, "/forge/internal/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for valid key, got %d", w.Code)
	}

	var res proto.RegisterResponse
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.AgentID == "" || res.Token == "" {
		t.Fatalf("expected agentID and token in response, got agentID=%s token=%s", res.AgentID, res.Token)
	}
}
