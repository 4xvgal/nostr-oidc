package web

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lescuer97/nostr-oicd/storage"
	"golang.org/x/text/language"
)

// UserRequest represents the request body for creating/updating a user
type UserRequest struct {
	Pubkey            string `json:"pubkey"`             // 32-byte hex (64 characters)
	PreferredLanguage string `json:"preferred_language"` // e.g., "en", "ko"
	IsAdmin           bool   `json:"is_admin"`
	Active            bool   `json:"active"`
}

// UserResponse represents the response body for user operations
type UserResponse struct {
	ID                string `json:"id"`
	Pubkey            string `json:"pubkey"` // 32-byte hex (64 characters)
	PreferredLanguage string `json:"preferred_language"`
	IsAdmin           bool   `json:"is_admin"`
	Active            bool   `json:"active"`
}

// NewAPIAdminHandler creates a new router for the admin API endpoints
func NewAPIAdminHandler(server *Server) chi.Router {
	router := chi.NewRouter()
	router.Use(server.APIKeyMiddleware)

	router.Get("/users", handleListUsers(server))
	router.Post("/users", handleCreateUser(server))
	router.Get("/users/{pubkey}", handleGetUser(server))
	router.Put("/users/{pubkey}", handleUpdateUser(server))
	router.Delete("/users/{pubkey}", handleDeleteUser(server))

	return router
}

// convertHexToPubKey converts a 32-byte hex public key to btcec.PublicKey
func convertHexToPubKey(hexPubkey string) (*btcec.PublicKey, error) {
	// Validate length (64 hex characters = 32 bytes)
	if len(hexPubkey) != 64 {
		return nil, fmt.Errorf("public key must be exactly 64 hex characters (32 bytes), got %d", len(hexPubkey))
	}

	// Decode hex to bytes
	pubkeyBytes, err := hex.DecodeString(hexPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex encoding: %w", err)
	}

	// Parse as schnorr public key (which uses 32-byte x-only format)
	pubkey, err := schnorr.ParsePubKey(pubkeyBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	return pubkey, nil
}

// convertPubKeyToHex converts a btcec.PublicKey to 32-byte hex format
func convertPubKeyToHex(pubkey *btcec.PublicKey) string {
	// Use schnorr serialization which returns the 32-byte x-coordinate
	pubkeyBytes := schnorr.SerializePubKey(pubkey)
	return hex.EncodeToString(pubkeyBytes)
}

// userToResponse converts a storage.User to UserResponse
func userToResponse(user *storage.User) UserResponse {
	response := UserResponse{
		ID:                user.ID,
		PreferredLanguage: user.PreferredLanguage.String(),
		IsAdmin:           user.IsAdmin,
		Active:            user.Active,
	}

	if user.Npub != nil {
		response.Pubkey = convertPubKeyToHex(user.Npub)
	}

	return response
}

// handleListUsers returns all non-admin users
func handleListUsers(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := server.Storage.GetAllUsers(r.Context())
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to retrieve users")
			return
		}

		responses := make([]UserResponse, 0, len(users))
		for _, user := range users {
			responses = append(responses, userToResponse(&user))
		}

		respondWithJSON(w, http.StatusOK, responses)
	}
}

// handleCreateUser creates a new user
func handleCreateUser(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req UserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body")
			return
		}

		// Validate required fields
		if req.Pubkey == "" {
			respondWithError(w, http.StatusBadRequest, "missing_field", "pubkey is required")
			return
		}

		// Convert hex pubkey to btcec.PublicKey
		pubkey, err := convertHexToPubKey(req.Pubkey)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid_pubkey", err.Error())
			return
		}

		// Check if user already exists
		existingUser, err := server.Storage.CheckUserNpub(pubkey)
		if err == nil && existingUser != nil {
			respondWithError(w, http.StatusConflict, "user_exists", "User with this public key already exists")
			return
		}

		// Parse preferred language
		var preferredLang language.Tag
		if req.PreferredLanguage != "" {
			preferredLang, err = language.Parse(req.PreferredLanguage)
			if err != nil {
				respondWithError(w, http.StatusBadRequest, "invalid_language", "Invalid preferred_language code")
				return
			}
		} else {
			preferredLang = language.English
		}

		// Create user
		user := storage.User{
			ID:                uuid.NewString(),
			Npub:              pubkey,
			PreferredLanguage: preferredLang,
			IsAdmin:           req.IsAdmin,
			Active:            req.Active,
		}

		err = server.Storage.AddUser(r.Context(), user)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to create user")
			return
		}

		respondWithJSON(w, http.StatusCreated, userToResponse(&user))
	}
}

// handleGetUser retrieves a user by public key
func handleGetUser(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkeyHex := chi.URLParam(r, "pubkey")
		if pubkeyHex == "" {
			respondWithError(w, http.StatusBadRequest, "missing_pubkey", "pubkey parameter is required")
			return
		}

		// Convert hex to pubkey
		pubkey, err := convertHexToPubKey(pubkeyHex)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid_pubkey", err.Error())
			return
		}

		// Find user
		user, err := server.Storage.CheckUserNpub(pubkey)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondWithError(w, http.StatusNotFound, "user_not_found", "User not found")
				return
			}
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to retrieve user")
			return
		}

		respondWithJSON(w, http.StatusOK, userToResponse(user))
	}
}

// handleUpdateUser updates an existing user
func handleUpdateUser(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkeyHex := chi.URLParam(r, "pubkey")
		if pubkeyHex == "" {
			respondWithError(w, http.StatusBadRequest, "missing_pubkey", "pubkey parameter is required")
			return
		}

		// Convert hex to pubkey
		pubkey, err := convertHexToPubKey(pubkeyHex)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid_pubkey", err.Error())
			return
		}

		// Find existing user
		existingUser, err := server.Storage.CheckUserNpub(pubkey)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondWithError(w, http.StatusNotFound, "user_not_found", "User not found")
				return
			}
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to retrieve user")
			return
		}

		// Parse request body
		var req UserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body")
			return
		}

		// Update fields
		if req.PreferredLanguage != "" {
			preferredLang, err := language.Parse(req.PreferredLanguage)
			if err != nil {
				respondWithError(w, http.StatusBadRequest, "invalid_language", "Invalid preferred_language code")
				return
			}
			existingUser.PreferredLanguage = preferredLang
		}

		existingUser.IsAdmin = req.IsAdmin
		existingUser.Active = req.Active

		// Update in database
		err = server.Storage.EditUser(r.Context(), *existingUser)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to update user")
			return
		}

		respondWithJSON(w, http.StatusOK, userToResponse(existingUser))
	}
}

// handleDeleteUser deletes a user by public key
func handleDeleteUser(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkeyHex := chi.URLParam(r, "pubkey")
		if pubkeyHex == "" {
			respondWithError(w, http.StatusBadRequest, "missing_pubkey", "pubkey parameter is required")
			return
		}

		// Convert hex to pubkey
		pubkey, err := convertHexToPubKey(pubkeyHex)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid_pubkey", err.Error())
			return
		}

		// Find user to get ID
		user, err := server.Storage.CheckUserNpub(pubkey)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondWithError(w, http.StatusNotFound, "user_not_found", "User not found")
				return
			}
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to retrieve user")
			return
		}

		// Delete user
		err = server.Storage.DeleteUser(r.Context(), user.ID)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "server_error", "Failed to delete user")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
