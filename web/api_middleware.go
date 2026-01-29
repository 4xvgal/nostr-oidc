package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lescuer97/nostr-oicd/storage"
)

const (
	apiKeyContextKey contextKey = "apiKey"
)

// ErrorResponse represents an error response for the API
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// APIKeyMiddleware validates the API key from the Authorization header
func (s *Server) APIKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondWithError(w, http.StatusUnauthorized, "missing_authorization", "Authorization header is required")
			return
		}

		// Check for Bearer token format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondWithError(w, http.StatusUnauthorized, "invalid_authorization", "Authorization header must be in format: Bearer <token>")
			return
		}

		rawKey := parts[1]
		if rawKey == "" {
			respondWithError(w, http.StatusUnauthorized, "missing_token", "API key token is required")
			return
		}

		// Validate the API key
		apiKey, err := s.Storage.ValidateAPIKey(r.Context(), rawKey)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid or inactive API key")
			return
		}

		// Store the API key in context for use in handlers
		ctx := context.WithValue(r.Context(), apiKeyContextKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetAPIKeyFromContext retrieves the API key from the request context
func GetAPIKeyFromContext(ctx context.Context) (*storage.APIKey, bool) {
	apiKey, ok := ctx.Value(apiKeyContextKey).(*storage.APIKey)
	return apiKey, ok
}

// respondWithError writes a JSON error response
func respondWithError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   errorCode,
		Message: message,
	})
}

// respondWithJSON writes a JSON success response
func respondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}
