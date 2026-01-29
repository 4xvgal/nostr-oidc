package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lescuer97/nostr-oicd/storage"
	"golang.org/x/text/language"
)

// mockStorage implements a minimal Storage interface for testing
type mockStorage struct {
	users           map[string]*storage.User // keyed by hex pubkey
	apiKeys         map[string]*storage.APIKey
	shouldFailUsers bool
	shouldFailAPIKey bool
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		users:   make(map[string]*storage.User),
		apiKeys: make(map[string]*storage.APIKey),
	}
}

func (m *mockStorage) ValidateAPIKey(ctx context.Context, rawKey string) (*storage.APIKey, error) {
	if m.shouldFailAPIKey {
		return nil, &mockError{"invalid API key"}
	}

	keyHash := storage.HashAPIKey(rawKey)
	apiKey, ok := m.apiKeys[keyHash]
	if !ok {
		return nil, &mockError{"API key not found"}
	}
	return apiKey, nil
}

func (m *mockStorage) GetAllUsers(ctx context.Context) ([]storage.User, error) {
	if m.shouldFailUsers {
		return nil, &mockError{"database error"}
	}

	users := make([]storage.User, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, *user)
	}
	return users, nil
}

func (m *mockStorage) CheckUserNpub(pubkey *btcec.PublicKey) (*storage.User, error) {
	hexKey := convertPubKeyToHex(pubkey)
	user, ok := m.users[hexKey]
	if !ok {
		return nil, &mockError{"user not found"}
	}
	return user, nil
}

func (m *mockStorage) AddUser(ctx context.Context, user storage.User) error {
	if m.shouldFailUsers {
		return &mockError{"database error"}
	}

	hexKey := convertPubKeyToHex(user.Npub)
	if _, exists := m.users[hexKey]; exists {
		return &mockError{"user already exists"}
	}
	m.users[hexKey] = &user
	return nil
}

func (m *mockStorage) EditUser(ctx context.Context, user storage.User) error {
	if m.shouldFailUsers {
		return &mockError{"database error"}
	}

	hexKey := convertPubKeyToHex(user.Npub)
	m.users[hexKey] = &user
	return nil
}

func (m *mockStorage) DeleteUser(ctx context.Context, id string) error {
	if m.shouldFailUsers {
		return &mockError{"database error"}
	}

	for key, user := range m.users {
		if user.ID == id {
			delete(m.users, key)
			return nil
		}
	}
	return &mockError{"user not found"}
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

// Helper function to create test server with mock storage
func setupTestServer() (*Server, *mockStorage) {
	mock := newMockStorage()

	// Add a valid API key
	validKey, _ := storage.GenerateAPIKey()
	keyHash := storage.HashAPIKey(validKey)
	mock.apiKeys[keyHash] = &storage.APIKey{
		ID:       uuid.NewString(),
		Label:    "Test Key",
		KeyHash:  keyHash,
		IsActive: true,
	}

	server := &Server{
		Storage: mock,
	}

	return server, mock
}

func TestAPIKeyMiddleware(t *testing.T) {
	server, mock := setupTestServer()

	// Get the valid key from mock
	var validKey string
	for hash := range mock.apiKeys {
		// Find the original key by checking all possibilities (in real test, store it)
		testKey, _ := storage.GenerateAPIKey()
		if storage.HashAPIKey(testKey) == hash {
			validKey = testKey
			break
		}
	}

	// For simplicity, generate a new key and add it
	validKey, _ = storage.GenerateAPIKey()
	mock.apiKeys[storage.HashAPIKey(validKey)] = &storage.APIKey{
		ID:       uuid.NewString(),
		IsActive: true,
	}

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid API key",
			authHeader:     "Bearer " + validKey,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid format - no Bearer",
			authHeader:     validKey,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid API key",
			authHeader:     "Bearer invalid_key_12345",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that the middleware will wrap
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with middleware
			wrappedHandler := server.APIKeyMiddleware(handler)

			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Record response
			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestHandleListUsers(t *testing.T) {
	server, mock := setupTestServer()

	// Add some test users
	pubkey1, _ := convertHexToPubKey("3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d")
	pubkey2, _ := convertHexToPubKey("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2")

	mock.users["3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"] = &storage.User{
		ID:                uuid.NewString(),
		Npub:              pubkey1,
		PreferredLanguage: language.English,
		IsAdmin:           false,
		Active:            true,
	}
	mock.users["82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"] = &storage.User{
		ID:                uuid.NewString(),
		Npub:              pubkey2,
		PreferredLanguage: language.Korean,
		IsAdmin:           false,
		Active:            true,
	}

	// Create request
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	rr := httptest.NewRecorder()

	// Call handler
	handler := handleListUsers(server)
	handler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Parse response
	var users []UserResponse
	err := json.NewDecoder(rr.Body).Decode(&users)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check number of users
	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}
}

func TestHandleCreateUser(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    UserRequest
		expectedStatus int
		shouldExist    bool
	}{
		{
			name: "valid user creation",
			requestBody: UserRequest{
				Pubkey:            "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
				PreferredLanguage: "en",
				IsAdmin:           false,
				Active:            true,
			},
			expectedStatus: http.StatusCreated,
			shouldExist:    false,
		},
		{
			name: "duplicate user",
			requestBody: UserRequest{
				Pubkey:            "82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2",
				PreferredLanguage: "ko",
				IsAdmin:           false,
				Active:            true,
			},
			expectedStatus: http.StatusConflict,
			shouldExist:    true,
		},
		{
			name: "invalid pubkey - wrong length",
			requestBody: UserRequest{
				Pubkey:            "invalid",
				PreferredLanguage: "en",
				IsAdmin:           false,
				Active:            true,
			},
			expectedStatus: http.StatusBadRequest,
			shouldExist:    false,
		},
		{
			name: "missing pubkey",
			requestBody: UserRequest{
				Pubkey:            "",
				PreferredLanguage: "en",
				IsAdmin:           false,
				Active:            true,
			},
			expectedStatus: http.StatusBadRequest,
			shouldExist:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, mock := setupTestServer()

			// Add existing user if needed
			if tt.shouldExist {
				pubkey, _ := convertHexToPubKey(tt.requestBody.Pubkey)
				mock.users[tt.requestBody.Pubkey] = &storage.User{
					ID:     uuid.NewString(),
					Npub:   pubkey,
					Active: true,
				}
			}

			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Call handler
			handler := handleCreateUser(server)
			handler.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleGetUser(t *testing.T) {
	tests := []struct {
		name           string
		pubkey         string
		userExists     bool
		expectedStatus int
	}{
		{
			name:           "user exists",
			pubkey:         "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
			userExists:     true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "user not found",
			pubkey:         "82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2",
			userExists:     false,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid pubkey",
			pubkey:         "invalid",
			userExists:     false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, mock := setupTestServer()

			// Add user if needed
			if tt.userExists {
				pubkey, err := convertHexToPubKey(tt.pubkey)
				if err == nil {
					mock.users[tt.pubkey] = &storage.User{
						ID:     uuid.NewString(),
						Npub:   pubkey,
						Active: true,
					}
				}
			}

			// Create request with chi context
			req := httptest.NewRequest("GET", "/api/admin/users/"+tt.pubkey, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("pubkey", tt.pubkey)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rr := httptest.NewRecorder()

			// Call handler
			handler := handleGetUser(server)
			handler.ServeHTTP(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleDeleteUser(t *testing.T) {
	server, mock := setupTestServer()

	// Add a test user
	testPubkey := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	pubkey, _ := convertHexToPubKey(testPubkey)
	userID := uuid.NewString()

	mock.users[testPubkey] = &storage.User{
		ID:     userID,
		Npub:   pubkey,
		Active: true,
	}

	// Create request
	req := httptest.NewRequest("DELETE", "/api/admin/users/"+testPubkey, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("pubkey", testPubkey)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()

	// Call handler
	handler := handleDeleteUser(server)
	handler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusNoContent {
		t.Errorf("Expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	// Verify user was deleted
	if _, exists := mock.users[testPubkey]; exists {
		t.Errorf("User should have been deleted")
	}
}

func TestRespondWithError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorCode  string
		message    string
	}{
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			errorCode:  "invalid_api_key",
			message:    "Invalid API key",
		},
		{
			name:       "bad request",
			statusCode: http.StatusBadRequest,
			errorCode:  "invalid_request",
			message:    "Invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			respondWithError(rr, tt.statusCode, tt.errorCode, tt.message)

			// Check status code
			if rr.Code != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, rr.Code)
			}

			// Check content type
			contentType := rr.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", contentType)
			}

			// Parse response
			var errResp ErrorResponse
			err := json.NewDecoder(rr.Body).Decode(&errResp)
			if err != nil {
				t.Fatalf("Failed to decode error response: %v", err)
			}

			if errResp.Error != tt.errorCode {
				t.Errorf("Expected error code %q, got %q", tt.errorCode, errResp.Error)
			}
			if errResp.Message != tt.message {
				t.Errorf("Expected message %q, got %q", tt.message, errResp.Message)
			}
		})
	}
}
