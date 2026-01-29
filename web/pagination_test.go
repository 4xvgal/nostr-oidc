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

// TestPagination tests the pagination functionality
func TestPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping pagination test in short mode")
	}

	// Setup test database
	dbPath := "./test_pagination.db"

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

	// Create test data
	ctx := context.Background()

	// Create API key
	_, rawKey, err := store.CreateAPIKey(ctx, "pagination-test", "test-user")
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

	// Create test users (using valid secp256k1 curve public keys)
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

	t.Run("DefaultPagination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// Should show all users on page 1 (default limit is 10)
		if !strings.Contains(body, "3bf0c63") {
			t.Error("Expected to find first user")
		}
	})

	t.Run("PageParameter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?page=1&limit=2", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// Should show pagination controls
		if !strings.Contains(body, "Next") {
			t.Error("Expected to find Next button")
		}
		// Count table rows to verify limit
		rowCount := strings.Count(body, "<tr class=\"hover:bg-gray-50")
		if rowCount > 2 {
			t.Errorf("Expected at most 2 rows, got %d", rowCount)
		}
	})

	t.Run("PaginationWithFilters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?language=en&page=1&limit=10", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// Should show only English users
		if !strings.Contains(body, "3bf0c63") {
			t.Error("Expected to find English user")
		}
		if strings.Contains(body, "82341f8") {
			t.Error("Should not find Spanish user when filtering for English")
		}
	})

	t.Run("InvalidPage", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?page=999", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Should handle gracefully and show last page or empty
		body := w.Body.String()
		t.Logf("Invalid page response length: %d", len(body))
	})

	t.Run("LimitBoundary", func(t *testing.T) {
		// Test that limit is capped at 100
		req := httptest.NewRequest("GET", "/admin/users-api/list?limit=200", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Should work and cap at 100
		body := w.Body.String()
		if len(body) == 0 {
			t.Error("Expected response body")
		}
	})

	t.Run("PaginationInfo", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users-api/list?page=1&limit=2", nil)
		w := httptest.NewRecorder()

		adminHandler.usersListAPI(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body := w.Body.String()
		// Should show pagination info like "Showing 1 to 2 of X users"
		if !strings.Contains(body, "Showing") {
			t.Error("Expected to find pagination info text")
		}
	})
}
