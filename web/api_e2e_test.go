package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lescuer97/nostr-oicd/storage"
	"github.com/lescuer97/nostr-oicd/storage/database"
)

// setupTestDB creates a test database and returns storage + cleanup function
func setupTestDB(t *testing.T) (*storage.Storage, func()) {
	// Use test database
	dbPath := "./test_e2e.db"

	// Remove old test DB if exists
	os.Remove(dbPath)

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = database.RunMigrations(db)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	store, err := storage.NewStorage(db)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		db.Close()
		os.Remove(dbPath)
	}

	return &store, cleanup
}

// TestAPIE2E_RealDatabase tests the API with real database
func TestAPIE2E_RealDatabase(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	server := &Server{
		Storage: store,
	}

	// Create API router
	router := NewAPIAdminHandler(server)

	// Step 1: Create API key
	ctx := context.Background()
	apiKey, rawKey, err := store.CreateAPIKey(ctx, "e2e-test-key", "test-admin")
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}
	t.Logf("Created API key: %s (prefix: %s)", apiKey.ID, apiKey.KeyPrefix)

	// Test data
	testPubkey1 := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	testPubkey2 := "82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"

	t.Run("1_CreateUser", func(t *testing.T) {
		createReq := UserRequest{
			Pubkey:            testPubkey1,
			PreferredLanguage: "en",
			IsAdmin:           false,
			Active:            true,
		}

		body, _ := json.Marshal(createReq)
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp UserResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		t.Logf("Created user: %s", resp.ID)
	})

	t.Run("2_ListUsers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", w.Code)
		}

		var users []UserResponse
		json.Unmarshal(w.Body.Bytes(), &users)

		if len(users) != 1 {
			t.Errorf("Expected 1 user, got %d", len(users))
		}
		t.Logf("Listed %d users", len(users))
	})

	t.Run("3_GetUser", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/users/%s", testPubkey1), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("4_UpdateUser", func(t *testing.T) {
		updateReq := UserRequest{
			PreferredLanguage: "ko",
			IsAdmin:           false, // Keep as regular user (GetAllUsers excludes admins)
			Active:            true,
		}

		body, _ := json.Marshal(updateReq)
		req := httptest.NewRequest("PUT", fmt.Sprintf("/users/%s", testPubkey1), bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp UserResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		if resp.PreferredLanguage != "ko" {
			t.Errorf("Expected language 'ko', got %s", resp.PreferredLanguage)
		}
		if resp.IsAdmin {
			t.Errorf("Expected IsAdmin=false, got true")
		}
	})

	t.Run("5_CreateSecondUser", func(t *testing.T) {
		createReq := UserRequest{
			Pubkey:            testPubkey2,
			PreferredLanguage: "es",
			IsAdmin:           false,
			Active:            true,
		}

		body, _ := json.Marshal(createReq)
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("6_ListUsers_ShouldHaveTwo", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var users []UserResponse
		json.Unmarshal(w.Body.Bytes(), &users)

		if len(users) != 2 {
			t.Errorf("Expected 2 users, got %d", len(users))
		}
		t.Logf("Listed %d users", len(users))
	})

	t.Run("7_DeleteUser", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/users/%s", testPubkey1), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected 204, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("8_GetDeletedUser_ShouldFail", func(t *testing.T) {
		req := httptest.NewRequest("GET", fmt.Sprintf("/users/%s", testPubkey1), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", w.Code)
		}
	})

	t.Run("9_ListUsers_ShouldHaveOne", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var users []UserResponse
		json.Unmarshal(w.Body.Bytes(), &users)

		if len(users) != 1 {
			t.Errorf("Expected 1 user after deletion, got %d", len(users))
		}
	})
}

// TestAPIE2E_ErrorCases tests error scenarios
func TestAPIE2E_ErrorCases(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	server := &Server{
		Storage: store,
	}

	router := NewAPIAdminHandler(server)
	ctx := context.Background()
	_, rawKey, _ := store.CreateAPIKey(ctx, "error-test-key", "test-admin")

	t.Run("InvalidPubkey", func(t *testing.T) {
		createReq := UserRequest{
			Pubkey:            "invalid_hex",
			PreferredLanguage: "en",
			IsAdmin:           false,
			Active:            true,
		}

		body, _ := json.Marshal(createReq)
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", w.Code)
		}
	})

	t.Run("MissingAPIKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users", nil)
		// No Authorization header
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", w.Code)
		}
	})

	t.Run("InvalidAPIKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users", nil)
		req.Header.Set("Authorization", "Bearer sk_invalid_key_123")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", w.Code)
		}
	})

	t.Run("DuplicateUser", func(t *testing.T) {
		testPubkey := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

		// Create first user
		createReq := UserRequest{
			Pubkey:            testPubkey,
			PreferredLanguage: "en",
			IsAdmin:           false,
			Active:            true,
		}

		body, _ := json.Marshal(createReq)
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Try to create duplicate
		req = httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("Expected 409, got %d", w.Code)
		}
	})

	t.Run("UserNotFound", func(t *testing.T) {
		nonExistentPubkey := "0000000000000000000000000000000000000000000000000000000000000001"
		req := httptest.NewRequest("GET", fmt.Sprintf("/users/%s", nonExistentPubkey), nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", w.Code)
		}
	})
}
