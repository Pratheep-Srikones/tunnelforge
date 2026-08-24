package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

func GenerateAgentUUID() string {
	return uuid.New().String()
}

func GenerateAgentToken() string {
	token, err := generateToken(32) // 32 bytes gives 256 bits of entropy
	if err != nil {
		fmt.Println("Error generating agent token: " + err.Error())
		fmt.Println("Falling back to UUID generation")
		return uuid.New().String()
	}
	return token
}

func generateToken(length int) (string, error) {
	// 32 bytes gives 256 bits of entropy
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	// RawURLEncoding avoids padding '=' characters
	return base64.RawURLEncoding.EncodeToString(b), nil
}
