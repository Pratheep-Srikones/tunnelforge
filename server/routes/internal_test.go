package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tunnelforge/server/tunnel"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(reg *tunnel.AgentRegistry) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	internal := r.Group("/forge/internal")
	UseInternalRoutes(internal, reg)
	return r
}

func TestInternal_Health(t *testing.T) {
	r := setupTestRouter(tunnel.NewAgentRegistry())

	req, _ := http.NewRequest(http.MethodGet, "/forge/internal/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("expected status ok, got %s", res["status"])
	}
}

func TestAdmin_AuthMiddleware(t *testing.T) {
	r := setupTestRouter(tunnel.NewAgentRegistry())

	// 1. Missing header -> 401
	req, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}

	// 2. Wrong header -> 401
	req2, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions", nil)
	req2.Header.Set("X-Admin-Token", "wrong-token")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w2.Code)
	}

	// 3. Valid header -> 200
	req3, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions", nil)
	req3.Header.Set("X-Admin-Token", DefaultAdminToken)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w3.Code)
	}
}

func TestAdmin_SessionsOperations(t *testing.T) {
	reg := tunnel.NewAgentRegistry()
	t1 := &tunnel.Tunnel{
		ID:        "t-1",
		AgentID:   "agent-alpha",
		Subdomain: "api",
		CreatedAt: time.Now(),
	}
	t2 := &tunnel.Tunnel{
		ID:        "t-2",
		AgentID:   "agent-beta",
		Subdomain: "web",
		CreatedAt: time.Now(),
	}
	_ = reg.Register(t1)
	_ = reg.Register(t2)

	r := setupTestRouter(reg)

	// 1. List sessions
	req, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions", nil)
	req.Header.Set("X-Admin-Token", DefaultAdminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var listRes struct {
		Sessions []*tunnel.Tunnel `json:"sessions"`
		Count    int              `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listRes); err != nil {
		t.Fatalf("failed to decode sessions: %v", err)
	}
	if listRes.Count != 2 {
		t.Fatalf("expected 2 sessions, got %d", listRes.Count)
	}

	// 2. Get session by subdomain
	reqSub, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions/api", nil)
	reqSub.Header.Set("X-Admin-Token", DefaultAdminToken)
	wSub := httptest.NewRecorder()
	r.ServeHTTP(wSub, reqSub)
	if wSub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wSub.Code)
	}

	var sessionRes tunnel.Tunnel
	if err := json.Unmarshal(wSub.Body.Bytes(), &sessionRes); err != nil {
		t.Fatalf("failed to decode session: %v", err)
	}
	if sessionRes.Subdomain != "api" || sessionRes.AgentID != "agent-alpha" {
		t.Errorf("unexpected session: %+v", sessionRes)
	}

	// 3. Get session by AgentID
	reqAgent, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions/agent-beta", nil)
	reqAgent.Header.Set("X-Admin-Token", DefaultAdminToken)
	wAgent := httptest.NewRecorder()
	r.ServeHTTP(wAgent, reqAgent)
	if wAgent.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wAgent.Code)
	}

	// 4. Get non-existent session
	reqNotFound, _ := http.NewRequest(http.MethodGet, "/forge/internal/admin/sessions/unknown", nil)
	reqNotFound.Header.Set("X-Admin-Token", DefaultAdminToken)
	wNotFound := httptest.NewRecorder()
	r.ServeHTTP(wNotFound, reqNotFound)
	if wNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", wNotFound.Code)
	}

	// 5. Delete session
	reqDel, _ := http.NewRequest(http.MethodDelete, "/forge/internal/admin/sessions/api", nil)
	reqDel.Header.Set("X-Admin-Token", DefaultAdminToken)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wDel.Code)
	}

	// Verify session removed from registry
	if _, exists := reg.Get("api"); exists {
		t.Error("expected api tunnel to be removed from registry")
	}

	// Verify count is now 1
	if len(reg.List()) != 1 {
		t.Errorf("expected 1 tunnel remaining, got %d", len(reg.List()))
	}
}
