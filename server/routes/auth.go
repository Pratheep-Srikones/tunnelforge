package routes

import (
	"net/http"
	"os"
	"sync"
	"tunnelforge/internal/proto"
	"tunnelforge/server/auth"
	"tunnelforge/server/utils"

	"github.com/gin-gonic/gin"
)

const DefaultEnrollmentKey = "tf_enroll_xyz123"

var (
	keyMu               sync.RWMutex
	customEnrollmentKey string
	isUsingDefaultKey   bool = true
)

// InitEnrollmentKey configures the enrollment key in priority order:
// 1. Explicit CLI flag argument (if non-empty)
// 2. FORGE_ENROLLMENT_KEY environment variable (if non-empty)
// 3. Default key ("tf_enroll_xyz123")
func InitEnrollmentKey(flagVal string) string {
	keyMu.Lock()
	defer keyMu.Unlock()

	if flagVal != "" {
		customEnrollmentKey = flagVal
		isUsingDefaultKey = false
		return customEnrollmentKey
	}

	if envVal := os.Getenv("FORGE_ENROLLMENT_KEY"); envVal != "" {
		customEnrollmentKey = envVal
		isUsingDefaultKey = false
		return customEnrollmentKey
	}

	customEnrollmentKey = DefaultEnrollmentKey
	isUsingDefaultKey = true
	return customEnrollmentKey
}

func GetEnrollmentKey() string {
	keyMu.RLock()
	defer keyMu.RUnlock()
	if customEnrollmentKey != "" {
		return customEnrollmentKey
	}
	return DefaultEnrollmentKey
}

func IsUsingDefaultEnrollmentKey() bool {
	keyMu.RLock()
	defer keyMu.RUnlock()
	return isUsingDefaultKey
}

func UseAuthRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")

	auth.POST("/register", register)
}

func register(c *gin.Context) {
	var req proto.RegisterRequest

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request"})
		return
	}

	if req.EnrollmentKey != GetEnrollmentKey() {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid enrollment key"})
		return
	}

	agentID := utils.GenerateAgentUUID()
	token := utils.GenerateAgentToken()

	if err := auth.Register(agentID, token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to register agent"})
		return
	}

	var res proto.RegisterResponse
	res.AgentID = agentID
	res.Token = token

	c.JSON(http.StatusOK, res)
}

