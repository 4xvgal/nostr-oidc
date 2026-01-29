package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/lescuer97/nostr-oicd/storage"
	"github.com/lescuer97/nostr-oicd/storage/database"
)

// TestSearchAndFilter tests the search and filter functionality
func TestSearchAndFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping search and filter test in short mode")
	}

	// Setup test database
	dbPath := "./test_search_filter.db"

	// Clean up before test
	_ = os.Remove(dbPath)

	defer func() {
		// Cleanup after test
		os.Remove(dbPath)
	}()

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	err = database.RunMigrations(db)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	store, err := storage.NewStorage(db)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create test data with different attributes
	ctx := context.Background()

	// Create API key
	_, rawKey, err := store.CreateAPIKey(ctx, "search-test", "test-user")
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}

	// Create test server
	server := &Server{
		Storage:     &store,
		APIKeyStore: NewAPIKeyStore(),
	}

	// Setup router with API endpoints
	router := NewAPIAdminHandler(server)

	// Create test users via API (using valid secp256k1 curve public keys)
	testUsers := []UserRequest{
		{Pubkey: "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d", PreferredLanguage: "en", IsAdmin: false, Active: true},
		{Pubkey: "82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2", PreferredLanguage: "es", IsAdmin: false, Active: true},
		{Pubkey: "32e1827635450ebb3c5a7d12c1f8e7b2b514439ac10a67eef3d9fd9c5c68e245", PreferredLanguage: "pt", IsAdmin: false, Active: false},
	}

	for _, tu := range testUsers {
		body, _ := json.Marshal(tu)
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rawKey))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("Failed to create test user %s: status=%d, body=%s", tu.Pubkey[:8], w.Code, w.Body.String())
		}
	}

	// Setup admin handler for testing list view
	adminHandler := &adminHandler{server: server}

	t.Run("FilterBySearch", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?search=3bf0c63", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		if !strings.Contains(body, "3bf0c63") {
			t.Error("Expected to find user with pubkey starting with 3bf0c63")
		}
		if strings.Contains(body, "82341f8") {
			t.Error("Should not find user with pubkey starting with 82341f8")
		}
	})

	t.Run("FilterByLanguage", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?language=es", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		if !strings.Contains(body, "82341f8") {
			t.Error("Expected to find Spanish user")
		}
	})

	t.Run("FilterByStatus", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?status=active", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// Should find active users
		if !strings.Contains(body, "Active") {
			t.Error("Expected to find active status badge")
		}
		if strings.Contains(body, "Inactive") {
			t.Error("Should not find inactive status badge when filtering for active")
		}
	})

	t.Run("FilterByRole", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?role=user", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// All our test users are non-admin
		if strings.Contains(body, ">Admin<") {
			t.Error("Should not find admin role badge when filtering for users")
		}
	})

	t.Run("CombinedFilters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?language=en&status=active", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// Should only show English active users
		if !strings.Contains(body, "3bf0c63") {
			t.Error("Expected to find English active user")
		}
		if strings.Contains(body, "82341f8") || strings.Contains(body, "32e1827") {
			t.Error("Should not find Spanish or Portuguese users")
		}
	})

	t.Run("NoResults", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?search=nonexistent", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		if !strings.Contains(body, "No users found") {
			t.Error("Expected empty state message")
		}
	})

	t.Run("EmptyFilters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Should show all users
		body := w.Body.String()
		t.Logf("All users count: %d", strings.Count(body, "<tr class=\"hover:bg-gray-50"))
	})
}

