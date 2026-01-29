package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// APIKey represents an API key for authenticating REST API requests
type APIKey struct {
	ID         string
	Label      string
	KeyPrefix  string     // First 10 characters of the key for display
	KeyHash    string     // SHA-256 hash of the full key
	CreatedAt  time.Time
	LastUsedAt *time.Time
	CreatedBy  string
	IsActive   bool
}

// GenerateAPIKey generates a new random API key in the format "ak_" + 48 hex characters
// Returns the full key (to be shown once to the user)
func GenerateAPIKey() (string, error) {
	// Generate 24 random bytes (192 bits of entropy)
	randomBytes := make([]byte, 24)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("crypto/rand.Read: %w", err)
	}

	// Convert to hex and prefix with "ak_"
	key := "ak_" + hex.EncodeToString(randomBytes)
	return key, nil
}

// HashAPIKey creates a SHA-256 hash of the API key for storage
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// ExtractKeyPrefix extracts the first 10 characters of the key for display purposes
func ExtractKeyPrefix(key string) string {
	if len(key) < 10 {
		return key
	}
	return key[:10]
}

// ScanRow scans a database row into an APIKey struct
func (a *APIKey) ScanRow(row interface {
	Scan(dest ...interface{}) error
}) error {
	var lastUsedAt sql.NullTime

	err := row.Scan(
		&a.ID,
		&a.Label,
		&a.KeyPrefix,
		&a.KeyHash,
		&a.CreatedAt,
		&lastUsedAt,
		&a.CreatedBy,
		&a.IsActive,
	)
	if err != nil {
		return fmt.Errorf("row.Scan: %w", err)
	}

	if lastUsedAt.Valid {
		a.LastUsedAt = &lastUsedAt.Time
	}

	return nil
}
