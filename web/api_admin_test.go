package web

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

func TestConvertHexToPubKey(t *testing.T) {
	// Valid test case: use a known valid 32-byte hex pubkey
	validHex := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

	tests := []struct {
		name        string
		hexPubkey   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid 32-byte hex",
			hexPubkey:   validHex,
			expectError: false,
		},
		{
			name:        "invalid length - too short",
			hexPubkey:   "3bf0c63fcb93463407af97a5e5ee64fa",
			expectError: true,
			errorMsg:    "must be exactly 64 hex characters",
		},
		{
			name:        "invalid length - too long",
			hexPubkey:   validHex + "00",
			expectError: true,
			errorMsg:    "must be exactly 64 hex characters",
		},
		{
			name:        "invalid hex characters",
			hexPubkey:   "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg",
			expectError: true,
			errorMsg:    "invalid hex encoding",
		},
		{
			name:        "empty string",
			hexPubkey:   "",
			expectError: true,
			errorMsg:    "must be exactly 64 hex characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pubkey, err := convertHexToPubKey(tt.hexPubkey)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if pubkey == nil {
					t.Errorf("Expected valid pubkey but got nil")
				}
			}
		})
	}
}

func TestConvertPubKeyToHex(t *testing.T) {
	// Create a valid public key from hex
	validHex := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	pubkeyBytes, err := hex.DecodeString(validHex)
	if err != nil {
		t.Fatalf("Failed to decode test hex: %v", err)
	}

	pubkey, err := schnorr.ParsePubKey(pubkeyBytes)
	if err != nil {
		t.Fatalf("Failed to parse test pubkey: %v", err)
	}

	// Convert to hex
	resultHex := convertPubKeyToHex(pubkey)

	// Check length
	if len(resultHex) != 64 {
		t.Errorf("Expected 64 hex characters, got %d", len(resultHex))
	}

	// Check format (lowercase hex)
	for _, c := range resultHex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Result contains invalid hex character: %c", c)
		}
	}

	// The result should match the original hex
	if resultHex != validHex {
		t.Errorf("Round-trip conversion failed: got %q, want %q", resultHex, validHex)
	}
}

func TestPubKeyRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		hexKeys []string
	}{
		{
			name: "single key",
			hexKeys: []string{
				"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
			},
		},
		{
			name: "multiple keys",
			hexKeys: []string{
				"82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2",
				"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
				"0000000000000000000000000000000000000000000000000000000000000001",
				"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, originalHex := range tt.hexKeys {
				// Convert hex to pubkey
				pubkey, err := convertHexToPubKey(originalHex)
				if err != nil {
					t.Errorf("convertHexToPubKey(%q) failed: %v", originalHex, err)
					continue
				}

				// Convert pubkey back to hex
				resultHex := convertPubKeyToHex(pubkey)

				// Compare
				if resultHex != originalHex {
					t.Errorf("Round-trip failed for %q: got %q", originalHex, resultHex)
				}
			}
		})
	}
}

func TestUserToResponse(t *testing.T) {
	// This test requires storage.User which may have dependencies
	// For now, we'll skip this test or implement it when we have better mock setup
	t.Skip("Skipping userToResponse test - requires storage.User setup")
}

func BenchmarkConvertHexToPubKey(b *testing.B) {
	validHex := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := convertHexToPubKey(validHex)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkConvertPubKeyToHex(b *testing.B) {
	validHex := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	pubkeyBytes, _ := hex.DecodeString(validHex)
	pubkey, _ := schnorr.ParsePubKey(pubkeyBytes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertPubKeyToHex(pubkey)
	}
}
