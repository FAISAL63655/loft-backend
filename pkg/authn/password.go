// Package authn provides authentication utilities including password hashing and JWT management
package authn

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Argon2id parameters for secure password hashing
// These parameters provide high security while maintaining reasonable performance
const (
	// Memory usage in KB (64MB)
	argon2Memory = 64 * 1024
	// Number of iterations (time parameter)
	argon2Time = 3
	// Number of parallel threads
	argon2Parallelism = 2
	// Salt length in bytes
	saltLength = 32
	// Key length in bytes
	keyLength = 32
)

var (
	// ErrInvalidHash is returned when the password hash format is invalid
	ErrInvalidHash = errors.New("invalid hash format")
	// ErrHashMismatch is returned when the password doesn't match the hash
	ErrHashMismatch = errors.New("password hash mismatch")
)

// HashPassword generates a secure Argon2id hash for the given password
// Returns the hash in the format: $argon2id$v=19$m=65536,t=3,p=2$salt$hash
func HashPassword(password string) (string, error) {
	// Generate a random salt
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Generate the hash using Argon2id
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Parallelism, keyLength)

	// Encode salt and hash to base64
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	// Return the formatted hash string
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Parallelism, saltB64, hashB64), nil
}

// VerifyPassword verifies if the given password matches the hash
func VerifyPassword(password, hash string) error {
	// Support both Argon2id (preferred) and bcrypt for backward compatibility
	if strings.HasPrefix(hash, "$argon2id$") {
		// Parse the hash to extract parameters
		salt, hashBytes, memory, time, parallelism, err := parseHash(hash)
		if err != nil {
			return err
		}

		// Generate hash with the same parameters
		computedHash := argon2.IDKey([]byte(password), salt, time, memory, parallelism, keyLength)

		// Compare hashes using constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare(hashBytes, computedHash) == 1 {
			return nil
		}

		return ErrHashMismatch
	}

	// Fallback to bcrypt if hash indicates bcrypt format
	if strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$") {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
			return ErrHashMismatch
		}
		return nil
	}

	// Unknown hash format
	return ErrInvalidHash
}

// parseHash parses the Argon2id hash string and extracts parameters
// Expected format: $argon2id$v=19$m=65536,t=3,p=2$salt$hash
func parseHash(hash string) (salt, hashBytes []byte, memory, time uint32, parallelism uint8, err error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	// Check algorithm
	if parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	// Parse version (we expect v=19)
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}
	if version != argon2.Version {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	// Parse parameters
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &parallelism); err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	// Decode salt
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	// Decode hash
	hashBytes, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	return salt, hashBytes, memory, time, parallelism, nil
}

// NeedsRehash checks if the hash parameters differ from current Argon2id params
func NeedsRehash(hash string) bool {
	_, _, m, t, p, err := parseHash(hash)
	if err != nil {
		return false
	}
	return m != argon2Memory || t != argon2Time || p != argon2Parallelism
}

// IsValidPassword performs basic password validation
// Returns true if the password meets minimum security requirements
func IsValidPassword(password string) bool {
	// Minimum length of 8 characters
	if len(password) < 8 {
		return false
	}

	// Maximum length of 128 characters to prevent DoS attacks
	if len(password) > 128 {
		return false
	}

	// Require at least one letter (supports Unicode, e.g. Arabic) and one digit
	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			break
		}
	}

	return hasLetter && hasDigit
}
