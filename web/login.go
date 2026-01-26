package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lescuer97/nostr-oicd/storage"
	"github.com/lescuer97/nostr-oicd/vertex"
	"github.com/lescuer97/nostr-oicd/web/templates"
	"github.com/nbd-wtf/go-nostr"
	"github.com/zitadel/oidc/v3/pkg/op"
	"golang.org/x/text/language"
)

const (
	queryAuthRequestID      = "authRequestID"
	notificationTypeError   = "error"
	notificationTypeSuccess = "success"
	notificationTypeWarning = "warning"
)

type login struct {
	authenticate Storage
	router       chi.Router
	callback     func(context.Context, string) string
}

func NewLogin(authenticate Storage, callback func(context.Context, string) string, issuerInterceptor *op.IssuerInterceptor) *login {
	l := &login{
		authenticate: authenticate,
		callback:     callback,
	}
	l.createRouter(issuerInterceptor)
	return l
}
func (l *login) createRouter(issuerInterceptor *op.IssuerInterceptor) {
	l.router = chi.NewRouter()
	l.router.Get("/", l.loginHandler)
	l.router.Post("/", issuerInterceptor.HandlerFunc(l.checkLoginHandler))
}

type authenticate interface {
	CheckUserNpub(publicKey *btcec.PublicKey) (*storage.User, error)
	CheckNostrEventSignature(event nostr.Event) error
	AddUser(ctx context.Context, user storage.User) error

	AddUserIDToAuthRequest(ctx context.Context, id string, userID string) error
	SetAuthRequestDone(ctx context.Context, id string) error

	// gets the config
}

func (l *login) loginHandler(w http.ResponseWriter, r *http.Request) {
	config, err := l.authenticate.GetConfiguration(r.Context())
	if err != nil {
		slog.Error("Failed to generate challenge", slog.Any("error", err))
		http.Error(w, "Error generating challenge", http.StatusInternalServerError)
		return
	}

	// if config.RegistrationType == "manual" {
	// 	templates.NotFoundPage("Users are not freely able to signup in the service. Contact the administrator").Render(r.Context(), w)
	// 	return
	// }

	id := r.URL.Query().Get("authRequestID")

	fmt.Printf("\n id: %+v\n ", id)

	authRequest, err := l.authenticate.AuthRequestByID(r.Context(), id)
	if err != nil {
		slog.Error("l.authenticate.AuthRequestByID(r.Context(), nostrEvent.Content)", slog.Any("error", err))
		templates.ProblemHappened("Could not find the authorization request").Render(r.Context(), w)
		return
	}

	templates.Login(id, templates.CodeLogin, config.RegistrationType != "manual", authRequest.GetScopes()).Render(r.Context(), w)
}

func (l *login) checkLoginHandler(w http.ResponseWriter, r *http.Request) {
	var nostrEvent nostr.Event
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	defer r.Body.Close()
	err = json.Unmarshal(body, &nostrEvent)
	if err != nil {
		return
	}

	// check valid signature
	validSig, err := nostrEvent.CheckSignature()
	if err != nil {
		return
	}
	if !validSig {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	pubkeyBytes, err := hex.DecodeString(nostrEvent.PubKey)
	if err != nil {
		return
	}
	pubkey, err := schnorr.ParsePubKey(pubkeyBytes)
	if err != nil {
		return
	}

	user, err := l.authenticate.CheckUserNpub(pubkey)
	if err != nil {
		slog.Error("l.authenticate.CheckUserNpub(pubkey)", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not verify your user",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	fmt.Printf("\n authId: %+v", nostrEvent.Content)
	// check if the user exists and if you are logging in to the correct client
	authRequest, err := l.authenticate.AuthRequestByID(r.Context(), nostrEvent.Content)
	if err != nil {
		slog.Error("l.authenticate.AuthRequestByID(r.Context(), nostrEvent.Content)", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not validate loggin",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	if authRequest.GetClientID() == storage.OICD_ADMIN_DASHBOARD_CLIENT_ID {
		if !user.IsAdmin {
			writeHtmlNotification(templates.NotifInfo{
				Msg:  "You are not an administrator",
				Type: notificationTypeError,
			}, r, w)
			return
		}
	}

	err = l.authenticate.AddUserIDToAuthRequest(r.Context(), authRequest.GetID(), user.ID)
	if err != nil {
		slog.Error("l.authenticate.AddUserIDToAuthRequest(r.Context(), user.ID)", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not validate loggin",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	err = l.authenticate.SetAuthRequestDone(r.Context(), authRequest.GetID())
	if err != nil {
		slog.Error("l.authenticate.SetAuthRequestDone(r.Context(), authRequest.GetID())", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not validate loggin",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	http.Redirect(w, r, l.callback(r.Context(), nostrEvent.Content), http.StatusFound)
}

// signupHandler handles user registration with Nostr signature verification
type signupHandler struct {
	storage          Storage
	vertex           *vertex.VertexChecker
	mu               sync.Mutex
	activeChallenges map[string]string // maps challenge to IP for one-time use
	bypassWoT        bool
}

// writeHtmlNotification sends a notification template as HTMX response
func writeHtmlNotification(info templates.NotifInfo, r *http.Request, w http.ResponseWriter) {
	w.Header().Add("HX-Retarget", "#notifications")
	w.Header().Add("HX-Reswap", "innerHTML")
	// if info.Type == notificationTypeError {
	// 	w.WriteHeader(400)
	// }
	templates.Notification(info).Render(r.Context(), w)
}

// NewSignupHandler creates a new signup handler
func NewSignupHandler(storage Storage, vtx *vertex.VertexChecker, bypassWoT bool) chi.Router {
	s := &signupHandler{
		storage:          storage,
		activeChallenges: make(map[string]string),
		vertex:           vtx,
		bypassWoT:        bypassWoT,
	}
	router := chi.NewRouter()
	router.Get("/", s.displaySignupForm)
	router.Post("/", s.processSignup)
	return router
}

// generateChallenge creates a random challenge
func (s *signupHandler) generateChallenge() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate challenge: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// displaySignupForm renders the signup form with a server-generated challenge
func (s *signupHandler) displaySignupForm(w http.ResponseWriter, r *http.Request) {
	config, err := s.storage.GetConfiguration(r.Context())
	if err != nil {
		slog.Error("Failed to generate challenge", slog.Any("error", err))
		http.Error(w, "Error generating challenge", http.StatusInternalServerError)
		return
	}

	if config.RegistrationType == "manual" {
		templates.NotFoundPage("Users are not freely able to signup in the service. Contact the administrator").Render(r.Context(), w)
		return
	}

	// Generate a new challenge for this form
	challenge, err := s.generateChallenge()
	if err != nil {
		slog.Error("Failed to generate challenge", slog.Any("error", err))
		http.Error(w, "Error generating challenge", http.StatusInternalServerError)
		return
	}

	// Store the challenge associated with this client
	// In a real system, you might want to associate this with a session ID instead
	clientIP := r.RemoteAddr
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeChallenges[challenge] = clientIP

	// Render the form with the challenge embedded
	templates.SignupPage(challenge).Render(r.Context(), w)
}

// processSignup handles the signup form submission and creates a new user
func (s *signupHandler) processSignup(w http.ResponseWriter, r *http.Request) {
	config, err := s.storage.GetConfiguration(r.Context())
	if err != nil {
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "There was a problem during signup",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	if config.RegistrationType == "manual" {
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Public sign up is currently disabled",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Read the JSON body containing the signed Nostr event
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Error reading request body",
			Type: notificationTypeError,
		}, r, w)
		return
	}
	defer r.Body.Close()

	// Parse the JSON Nostr event
	var nostrEvent nostr.Event
	err = json.Unmarshal(body, &nostrEvent)
	if err != nil {
		slog.Error("Failed to parse nostr event", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid event format",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Verify that the challenge was one we issued and hasn't been used yet
	s.mu.Lock()
	_, isValidChallenge := s.activeChallenges[nostrEvent.Content]
	if !isValidChallenge {
		s.mu.Unlock()
		slog.Warn("Invalid or reused challenge", slog.String("challenge", nostrEvent.Content))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid or expired challenge",
			Type: notificationTypeError,
		}, r, w)
		return
	}
	// Remove the challenge to make it one-time use only
	delete(s.activeChallenges, nostrEvent.Content)
	s.mu.Unlock()

	// Verify the signature
	err = s.storage.CheckNostrEventSignature(nostrEvent)
	if err != nil {
		slog.Error("Signature verification failed", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Signature verification failed",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Parse the public key from the event
	pubkeyBytes, err := hex.DecodeString(nostrEvent.PubKey)
	if err != nil {
		slog.Error("Failed to decode pubkey hex", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid public key format",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	pubkey, err := schnorr.ParsePubKey(pubkeyBytes)
	if err != nil {
		slog.Error("Failed to parse schnorr pubkey", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid Nostr public key",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Check if user with this public key already exists
	_, err = s.storage.CheckUserNpub(pubkey)
	if err == nil {
		// User already exists
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "An account with this Nostr key already exists",
			Type: notificationTypeWarning,
		}, r, w)
		return
	}

	if !s.bypassWoT {
		valid, err := s.vertex.NpubHasEnoughReputation(r.Context(), pubkey)
		if err != nil {
			if errors.Is(err, vertex.RelayError) {
				slog.Error("There was a problem with vertex", slog.String("type", "vertex"), slog.Any("error", err))
				writeHtmlNotification(templates.NotifInfo{
					Msg:  "There was a problem validating your npub",
					Type: notificationTypeError,
				}, r, w)
				return

			}
			// User already exists
			writeHtmlNotification(templates.NotifInfo{
				Msg:  "There was a problem signing you up",
				Type: notificationTypeWarning,
			}, r, w)
			return
		}

		if !valid {
			writeHtmlNotification(templates.NotifInfo{
				Msg:  "Your npub does not have enough web of trust",
				Type: notificationTypeWarning,
			}, r, w)
			return
		}
	} else {
		slog.Info("WoT bypass: skipping reputation check", slog.String("npub", hex.EncodeToString(schnorr.SerializePubKey(pubkey))))
	}

	// Create new user
	userID := uuid.New().String()
	newUser := storage.User{
		ID:                userID,
		Npub:              pubkey,
		PreferredLanguage: language.English,
		IsAdmin:           false,
		Active:            true,
	}

	// Add user to storage
	err = s.storage.AddUser(r.Context(), newUser)
	if err != nil {
		slog.Error("Failed to create user", slog.Any("error", err))
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "Duplicate") {
			writeHtmlNotification(templates.NotifInfo{
				Msg:  "An account with this Nostr key already exists",
				Type: notificationTypeWarning,
			}, r, w)
			return
		}
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "An error occurred while creating your account. Please try again",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Display success message
	w.WriteHeader(http.StatusOK)
	templates.SignupSuccess().Render(r.Context(), w)
}
