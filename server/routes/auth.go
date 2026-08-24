package routes

import (
	"net/http"
	"tunnelforge/internal/proto"
	"tunnelforge/server/utils"

	"github.com/gin-gonic/gin"
)

const SecretEnrollmentKey = "tf_enroll_xyz123"

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

	if req.EnrollmentKey != SecretEnrollmentKey {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid enrollment key"})
		return
	}

	agentID := utils.GenerateAgentUUID()
	token := utils.GenerateAgentToken()

	var res proto.RegisterResponse
	res.AgentID = agentID
	res.Token = token

	c.JSON(http.StatusOK, res)
}
