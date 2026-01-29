package storage

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestGenerateAPIKey(t *testing.T) {
	// Test key generation
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() failed: %v", err)
	}

	// Check prefix
	if !strings.HasPrefix(key, "ak_") {
		t.Errorf("Generated key should start with 'ak_', got: %s", key)
	}

	// Check length: "ak_" (3) + 48 hex chars = 51 total
	expectedLen := 51
	if len(key) != expectedLen {
		t.Errorf("Generated key should be %d characters, got %d", expectedLen, len(key))
	}

	// Check uniqueness by generating multiple keys
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		k, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey() iteration %d failed: %v", i, err)
		}
		if keys[k] {
			t.Errorf("Generated duplicate key: %s", k)
		}
		keys[k] = true
	}
}

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "normal key",
			key:  "ak_1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name: "short key",
			key:  "ak_123",
		},
		{
			name: "empty key",
			key:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := HashAPIKey(tt.key)
			hash2 := HashAPIKey(tt.key)

			// Check consistency
			if hash1 != hash2 {
				t.Errorf("HashAPIKey() produced inconsistent results: %s != %s", hash1, hash2)
			}

			// Check SHA-256 hash length (64 hex characters)
			expectedLen := 64
			if len(hash1) != expectedLen {
				t.Errorf("Hash should be %d characters, got %d", expectedLen, len(hash1))
			}

			// Check different keys produce different hashes
			if tt.key != "" {
				differentKey := tt.key + "x"
				differentHash := HashAPIKey(differentKey)
				if hash1 == differentHash {
					t.Errorf("Different keys should produce different hashes")
				}
			}
		})
	}
}

func TestExtractKeyPrefix(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "normal key",
			key:      "ak_1234567890abcdef",
			expected: "ak_1234567",
		},
		{
			name:     "exactly 10 chars",
			key:      "0123456789",
			expected: "0123456789",
		},
		{
			name:     "less than 10 chars",
			key:      "short",
			expected: "short",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "",
		},
		{
			name:     "long key",
			key:      "ak_1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "ak_1234567",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractKeyPrefix(tt.key)
			if result != tt.expected {
				t.Errorf("ExtractKeyPrefix(%q) = %q, want %q", tt.key, result, tt.expected)
			}
		})
	}
}

func TestAPIKeyScanRow(t *testing.T) {
	now := time.Now()
	lastUsed := now.Add(-1 * time.Hour)

	tests := []struct {
		name        string
		values      []interface{}
		expectError bool
		validate    func(*testing.T, *APIKey)
	}{
		{
			name: "valid row with last_used_at",
			values: []interface{}{
				"id-123",
				"Test Key",
				"ak_1234567",
				"hash123",
				now,
				sql.NullTime{Time: lastUsed, Valid: true},
				"user-456",
				true,
			},
			expectError: false,
			validate: func(t *testing.T, a *APIKey) {
				if a.ID != "id-123" {
					t.Errorf("ID = %q, want %q", a.ID, "id-123")
				}
				if a.Label != "Test Key" {
					t.Errorf("Label = %q, want %q", a.Label, "Test Key")
				}
				if a.KeyPrefix != "ak_1234567" {
					t.Errorf("KeyPrefix = %q, want %q", a.KeyPrefix, "ak_1234567")
				}
				if a.KeyHash != "hash123" {
					t.Errorf("KeyHash = %q, want %q", a.KeyHash, "hash123")
				}
				if a.CreatedBy != "user-456" {
					t.Errorf("CreatedBy = %q, want %q", a.CreatedBy, "user-456")
				}
				if !a.IsActive {
					t.Errorf("IsActive = %v, want true", a.IsActive)
				}
				if a.LastUsedAt == nil {
					t.Errorf("LastUsedAt should not be nil")
				} else if !a.LastUsedAt.Equal(lastUsed) {
					t.Errorf("LastUsedAt = %v, want %v", a.LastUsedAt, lastUsed)
				}
			},
		},
		{
			name: "valid row without last_used_at",
			values: []interface{}{
				"id-789",
				"Another Key",
				"ak_9876543",
				"hash789",
				now,
				sql.NullTime{Valid: false},
				"user-999",
				false,
			},
			expectError: false,
			validate: func(t *testing.T, a *APIKey) {
				if a.ID != "id-789" {
					t.Errorf("ID = %q, want %q", a.ID, "id-789")
				}
				if a.LastUsedAt != nil {
					t.Errorf("LastUsedAt should be nil, got %v", a.LastUsedAt)
				}
				if a.IsActive {
					t.Errorf("IsActive = %v, want false", a.IsActive)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock row scanner
			row := &mockScanner{values: tt.values}

			apiKey := &APIKey{}
			err := apiKey.ScanRow(row)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError && tt.validate != nil {
				tt.validate(t, apiKey)
			}
		})
	}
}

// mockScanner implements the Scan interface for testing
type mockScanner struct {
	values []interface{}
	err    error
}

func (m *mockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) != len(m.values) {
		return sql.ErrNoRows
	}
	for i, v := range m.values {
		switch d := dest[i].(type) {
		case *string:
			if s, ok := v.(string); ok {
				*d = s
			}
		case *time.Time:
			if t, ok := v.(time.Time); ok {
				*d = t
			}
		case *sql.NullTime:
			if nt, ok := v.(sql.NullTime); ok {
				*d = nt
			}
		case *bool:
			if b, ok := v.(bool); ok {
				*d = b
			}
		}
	}
	return nil
}
