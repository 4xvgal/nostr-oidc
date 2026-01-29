package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// APIClient handles HTTP requests to /api/admin endpoints
type APIClient struct {
	baseURL string        // e.g., "http://localhost:8082"
	client  *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Request represents a generic API request
type Request struct {
	Method   string
	Endpoint string      // e.g., "/users" or "/users/{pubkey}"
	APIKey   string      // Bearer token
	Body     interface{}
}

// APIError represents an error response from the API
type APIError struct {
	ErrorCode string `json:"error"`
	Message   string `json:"message"`
	Status    int    `json:"-"`
}

// Error implements the error interface
func (e *APIError) Error() string {
	return fmt.Sprintf("[%d] %s: %s", e.Status, e.ErrorCode, e.Message)
}

// doRequest performs an HTTP request to the API
func (c *APIClient) doRequest(req *Request, respData interface{}) error {
	url := c.baseURL + "/api/admin" + req.Endpoint

	// Prepare body
	var body io.Reader
	if req.Body != nil {
		jsonBody, err := json.Marshal(req.Body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(jsonBody)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest(req.Method, url, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	// Execute request
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Handle non-2xx responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			apiErr.ErrorCode = "unknown_error"
			apiErr.Message = string(respBody)
		}
		apiErr.Status = resp.StatusCode
		return &apiErr
	}

	// Parse successful response
	if respData != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, respData); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
	}

	return nil
}

// ListUsers retrieves all users
func (c *APIClient) ListUsers(apiKey string) ([]UserResponse, error) {
	var users []UserResponse
	err := c.doRequest(&Request{
		Method:   "GET",
		Endpoint: "/users",
		APIKey:   apiKey,
	}, &users)
	return users, err
}

// GetUser retrieves a user by pubkey
func (c *APIClient) GetUser(apiKey, pubkey string) (*UserResponse, error) {
	var user UserResponse
	err := c.doRequest(&Request{
		Method:   "GET",
		Endpoint: "/users/" + pubkey,
		APIKey:   apiKey,
	}, &user)
	return &user, err
}

// CreateUser creates a new user
func (c *APIClient) CreateUser(apiKey string, req *UserRequest) (*UserResponse, error) {
	var user UserResponse
	err := c.doRequest(&Request{
		Method:   "POST",
		Endpoint: "/users",
		APIKey:   apiKey,
		Body:     req,
	}, &user)
	return &user, err
}

// UpdateUser updates an existing user
func (c *APIClient) UpdateUser(apiKey, pubkey string, req *UserRequest) (*UserResponse, error) {
	var user UserResponse
	err := c.doRequest(&Request{
		Method:   "PUT",
		Endpoint: "/users/" + pubkey,
		APIKey:   apiKey,
		Body:     req,
	}, &user)
	return &user, err
}

// DeleteUser deletes a user
func (c *APIClient) DeleteUser(apiKey, pubkey string) error {
	return c.doRequest(&Request{
		Method:   "DELETE",
		Endpoint: "/users/" + pubkey,
		APIKey:   apiKey,
	}, nil)
}

// ========== API Key Store (In-Memory) ==========

// APIKeyStore manages API keys for users (in-memory, user scoped)
type APIKeyStore struct {
	mu   sync.RWMutex
	keys map[string]string // userID -> apiKey
}

// NewAPIKeyStore creates a new API key store
func NewAPIKeyStore() *APIKeyStore {
	return &APIKeyStore{
		keys: make(map[string]string),
	}
}

// Store stores an API key for a user
func (s *APIKeyStore) Store(userID, apiKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[userID] = apiKey
}

// Get retrieves the API key for a user
func (s *APIKeyStore) Get(userID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, exists := s.keys[userID]
	return key, exists
}

// Delete removes the API key for a user
func (s *APIKeyStore) Delete(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, userID)
}
