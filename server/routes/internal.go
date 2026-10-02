package routes

import (
	"net/http"
	"os"
	"tunnelforge/server/tunnel"

	"github.com/gin-gonic/gin"
)

const DefaultAdminToken = "tf_admin_xyz123"

func getAdminToken() string {
	if token := os.Getenv("FORGE_ADMIN_TOKEN"); token != "" {
		return token
	}
	return DefaultAdminToken
}

// adminAuthMiddleware verifies the X-Admin-Token header on admin endpoints.
func adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		expectedToken := getAdminToken()
		providedToken := c.GetHeader("X-Admin-Token")

		if providedToken == "" || providedToken != expectedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized: invalid or missing X-Admin-Token header",
			})
			return
		}
		c.Next()
	}
}

// UseInternalRoutes registers health and admin management endpoints on the provided router group.
func UseInternalRoutes(rg *gin.RouterGroup, registries ...*tunnel.AgentRegistry) {
	var reg *tunnel.AgentRegistry
	if len(registries) > 0 && registries[0] != nil {
		reg = registries[0]
	} else {
		reg = tunnel.GetAgentRegistry()
	}

	// Health check endpoint (accessible without admin token)
	rg.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "TunnelForge server is running",
		})
	})

	admin := rg.Group("/admin", adminAuthMiddleware())

	// Admin health check
	admin.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "TunnelForge admin API is healthy",
		})
	})

	// GET /admin/sessions - List all active tunnel sessions
	admin.GET("/sessions", func(c *gin.Context) {
		tunnels := reg.List()
		if tunnels == nil {
			tunnels = []*tunnel.Tunnel{}
		}

		c.JSON(http.StatusOK, gin.H{
			"sessions": tunnels,
			"count":    len(tunnels),
		})
	})

	// GET /admin/sessions/:id - Get session details by subdomain, session ID, or agent ID
	admin.GET("/sessions/:id", func(c *gin.Context) {
		id := c.Param("id")

		t, ok := findTunnel(reg, id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "session not found",
			})
			return
		}

		c.JSON(http.StatusOK, t)
	})

	// DELETE /admin/sessions/:id - Force-disconnect an active tunnel session
	admin.DELETE("/sessions/:id", func(c *gin.Context) {
		id := c.Param("id")

		t, ok := findTunnel(reg, id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "session not found",
			})
			return
		}

		// Force-close the yamux multiplexing session
		if t.Session != nil {
			_ = t.Session.Close()
		}

		// Remove from the registry
		reg.Remove(t.Subdomain)

		c.JSON(http.StatusOK, gin.H{
			"message":   "session disconnected",
			"subdomain": t.Subdomain,
			"agent_id":  t.AgentID,
			"id":        t.ID,
		})
	})
}

// findTunnel looks up a tunnel by subdomain first, then by Tunnel ID or Agent ID.
func findTunnel(reg *tunnel.AgentRegistry, id string) (*tunnel.Tunnel, bool) {
	if t, ok := reg.Get(id); ok {
		return t, true
	}

	for _, item := range reg.List() {
		if item.ID == id || item.AgentID == id {
			return item, true
		}
	}
	return nil, false
}
