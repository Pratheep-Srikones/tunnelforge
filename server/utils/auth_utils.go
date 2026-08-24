package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
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


// Recommended OWASP parameters for Argon2id
type params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var p = params{
	memory:      64 * 1024, // 64 MB
	iterations:  3,
	parallelism: 4,
	saltLength:  16,
	keyLength:   32,
}

// HashToken generates a securely encoded Argon2id password hash string
func HashToken(token string) (string, error) {
	// 1. Generate a cryptographically secure random salt
	salt := make([]byte, p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// 2. Compute the Argon2id hash
	hash := argon2.IDKey([]byte(token), salt, p.iterations, p.memory, p.parallelism, p.keyLength)

	// 3. Format parameters, salt, and hash into a single standard string
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism, b64Salt, b64Hash)

	return encoded, nil
}

// VerifyToken securely checks an encoded hash against a plaintext password
func VerifyToken(token, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid hash format")
	}

	var memory, iterations uint32
	var parallelism uint8

	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	// Re-hash incoming password with identical parameters
	comparisonHash := argon2.IDKey([]byte(token), salt, iterations, memory, parallelism, uint32(len(decodedHash)))

	// Protect against timing attacks during comparison
	if subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1 {
		return true, nil
	}
	return false, nil
}


