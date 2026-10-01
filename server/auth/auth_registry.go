package auth

import (
	"fmt"
	"sync"
	"tunnelforge/server/utils"
)

type AuthRegistry struct {
	mu          sync.RWMutex
	agentTokens map[string]string
}

var (
	instance *AuthRegistry
	once     sync.Once
)

// GetAuthRegistry returns the global singleton AuthRegistry instance.
func GetAuthRegistry() *AuthRegistry {
	once.Do(func() {
		instance = &AuthRegistry{
			agentTokens: make(map[string]string),
		}
	})
	return instance
}

// NewAuthRegistry returns the global singleton AuthRegistry instance.
func NewAuthRegistry() *AuthRegistry {
	return GetAuthRegistry()
}

func (r *AuthRegistry) Register(agentID, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	hashedToken, err := utils.HashToken(token)
	if err != nil {
		return fmt.Errorf("error hashing token: %v", err)
	}
	r.agentTokens[agentID] = hashedToken
	return nil
}

func (r *AuthRegistry) Get(agentID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hashedToken, ok := r.agentTokens[agentID]
	return hashedToken, ok
}

func (r *AuthRegistry) ValidateToken(agentID, token string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hashedToken, ok := r.agentTokens[agentID]
	if !ok {
		return false, nil
	}

	valid, err := utils.VerifyToken(token, hashedToken)
	if err != nil {
		return false, fmt.Errorf("error verifying token: %v", err)
	}
	return valid, nil
}

func (r *AuthRegistry) Delete(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.agentTokens, agentID)
}

// Global package-level functions operating on the singleton instance

// Register registers an agent ID and token in the singleton AuthRegistry.
func Register(agentID, token string) error {
	return GetAuthRegistry().Register(agentID, token)
}

// Get retrieves the hashed token for an agent ID from the singleton AuthRegistry.
func Get(agentID string) (string, bool) {
	return GetAuthRegistry().Get(agentID)
}

// ValidateToken checks if the provided token is valid for the given agent ID in the singleton AuthRegistry.
func ValidateToken(agentID, token string) (bool, error) {
	return GetAuthRegistry().ValidateToken(agentID, token)
}

// Delete removes an agent ID from the singleton AuthRegistry.
func Delete(agentID string) {
	GetAuthRegistry().Delete(agentID)
}
