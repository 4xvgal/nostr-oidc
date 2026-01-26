package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/form/v4"
	"github.com/google/uuid"
	"github.com/lescuer97/nostr-oicd/storage"
	"github.com/lescuer97/nostr-oicd/utils"
	"github.com/lescuer97/nostr-oicd/vertex"
	"github.com/lescuer97/nostr-oicd/web/templates"
)

var decoder = form.NewDecoder()

type administration interface {
	AddClient(ctx context.Context, client storage.Client) error
	EditClient(ctx context.Context, client storage.Client) error

	GetUserById(ctx context.Context, id string) (*storage.User, error)
	// AddUser(ctx context.Context, client storage.User) error
	EditUser(ctx context.Context, client storage.User) error

	// New methods for configuration management
	GetConfiguration(ctx context.Context) (*storage.Configuration, error)
	UpdateConfiguration(ctx context.Context, config *storage.Configuration) error

	NsecIsRegistered(ctx context.Context) (bool, error)

	GetAllClients(ctx context.Context) ([]storage.Client, error)
	GetAllUsers(ctx context.Context) ([]storage.User, error)
	GetAllAdminUsers(ctx context.Context) ([]storage.User, error)
}

// NewSignupHandler creates a new signup handler
func NewAdminHandler(server *Server) chi.Router {
	s := &adminHandler{
		server: server,
	}
	router := chi.NewRouter()

	// Existing routes
	// --- Unprotected Routes ---
	// These routes do NOT require OIDC token validation.
	router.Get("/", s.dashboard) // Dashboard is now explicitly unprotected
	router.Get("/login", s.login)
	router.Get("/oidc/callback", s.oidcCallback)

	router.Get("/add_user", s.addUser)
	router.Get("/user/{id}", s.editUserForm)

	router.Get("/add_client", s.addClientRoute)
	router.Get("/client/{id}", s.editClientFormById)

	router.Get("/configuration", s.configuration)
	// --- Protected Routes Group ---
	// All routes mounted within this group will have the AuthMiddleware applied.
	router.Group(func(r chi.Router) {
		r.Use(s.AuthMiddleware) // Apply authentication middleware to all routes within this group

		// Existing protected routes:
		r.Get("/client_form", s.clientFormFragmentHandler)
		r.Post("/client/{id}", s.editClient)
		r.Post("/add_client", s.addClient)

		r.Get("/user_form", s.addUserForm)
		r.Post("/add_user", s.addUserHandler)

		r.Post("/user/{id}", s.editUserHandler)

		r.Get("/configuration_form", s.configurationFormFragmentHandler)
		r.Put("/configuration", s.updateConfiguration)

		r.Get("/clients", s.clientsList)
		r.Get("/users", s.usersList)

		r.Get("/admin_users", s.adminUsersList)
	})

	return router
}

func (s *adminHandler) addClientRoute(w http.ResponseWriter, r *http.Request) {
	templates.ClientFormPage(nil).Render(r.Context(), w)
}

type adminHandler struct {
	server *Server
}

func (s *adminHandler) login(w http.ResponseWriter, r *http.Request) {
	client, err := s.server.Storage.GetClientByClientID(r.Context(), storage.OICD_ADMIN_DASHBOARD_CLIENT_ID)
	if err != nil {
		slog.Error("Failed to retrieve admin client configuration", slog.Any("error", err))
		http.Error(w, "Admin client not configured", http.StatusInternalServerError)
		return
	}

	templates.Pkce(client.GetID(), client.RedirectURIs()[0], "openid").Render(r.Context(), w)
}

func (s *adminHandler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	log.Printf("\n \n request Headers: %+v \n ", r.URL.Query())
	// Construct the token endpoint URL
	tokenEndpoint := &url.URL{
		Scheme: r.URL.Scheme,
		Host:   r.Host,
		Path:   "/oauth/token", // Assuming your token endpoint is at /token
	}

	templates.AdminPKCECallback(tokenEndpoint).Render(r.Context(), w)
}

func (s *adminHandler) clientFormFragmentHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		templates.ClientForm(nil).Render(r.Context(), w)
		return
	}

	client, err := s.server.Storage.GetClientByClientID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			templates.ClientForm(nil).Render(r.Context(), w)
			return
		}
		slog.Error("Client id does not exist", slog.Any("error", err))
		templates.ProblemHappened("could not find the client").Render(r.Context(), w)
		return
	}

	clientInfo := templates.ClientToFormData(client)
	templates.ClientForm(&clientInfo).Render(r.Context(), w)
}

func (s *adminHandler) editClientFormById(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	templates.ClientFormPage(&id).Render(r.Context(), w)
}

func (s *adminHandler) addClient(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid form data",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Decode into your struct
	var user templates.ClientFormData
	if err := decoder.Decode(&user, r.Form); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	client := templates.FormDataToStorageClient(&user, "")
	if client == nil {
		log.Panicf("client should have never been nil")
	}

	if len(client.RedirectURIs()) == 0 {
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "you need at least 1 redirect uri",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	err := s.server.Storage.AddClient(r.Context(), *client)
	if err != nil {
		slog.Error("s.storage.AddClient", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not add client",
			Type: notificationTypeError,
		}, r, w)

	}

	// Success - show success message
	writeHtmlNotification(templates.NotifInfo{
		Msg:  "Client created successfully (database save pending implementation)",
		Type: notificationTypeSuccess,
	}, r, w)
}

func (s *adminHandler) editClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Parse form data
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid form data",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Decode into your struct
	var clienForm templates.ClientFormData
	if err := decoder.Decode(&clienForm, r.Form); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	if id != clienForm.ClientID {
		slog.Error("trying to editing the wrong client")
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Trying to change a channel id without access",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	if len(clienForm.RedirectURIs) == 0 {
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "you need at least 1 redirect uri",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	client := templates.FormDataToStorageClient(&clienForm, "")

	if client == nil {
		log.Panicf("client should have never been nil")
	}
	err := s.server.Storage.EditClient(r.Context(), *client)
	if err != nil {
		slog.Error("s.storage.AddClient", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not add client",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Success - show success message
	writeHtmlNotification(templates.NotifInfo{
		Msg:  "Client created successfully (database save pending implementation)",
		Type: notificationTypeSuccess,
	}, r, w)

}

func (s *adminHandler) editUserForm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	templates.UserFormPage(&id).Render(r.Context(), w)
}

func (s *adminHandler) editUserHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Parse form data
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid form data",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Decode into your struct
	var formUser templates.UserFormData
	if err := decoder.Decode(&formUser, r.Form); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	if id != formUser.ID {
		slog.Error("trying to editing the wrong client")
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Trying to change a channel id without access",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	user, err := templates.FormDataToStorageUser(&formUser)
	if err != nil {
		slog.Error("parsing user for editing went wrong", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "parsing user for editing went wrong",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	if user == nil {
		log.Panicf("client should have never been nil")
	}

	err = s.server.Storage.EditUser(r.Context(), *user)
	if err != nil {
		slog.Error("s.storage.EditUser(r.Context(), *user)", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not add user",
			Type: notificationTypeError,
		}, r, w)

	}

	// Success - show success message
	writeHtmlNotification(templates.NotifInfo{
		Msg:  "User edited successfully",
		Type: notificationTypeSuccess,
	}, r, w)
}

func (s *adminHandler) addUser(w http.ResponseWriter, r *http.Request) {
	templates.UserFormPage(nil).Render(r.Context(), w)
}

func (s *adminHandler) addUserForm(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	user, err := s.server.Storage.GetUserById(r.Context(), id)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("\n inside the no row \n")
			templates.UserForm(nil).Render(r.Context(), w)
			return
		}
		slog.Error("User id does not exist", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "User not found",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	fmt.Printf("\n user %+v", user)
	// Convert storage.User to UserFormData
	userFormData := templates.StorageUserToFormData(user)
	log.Printf("\n userFormData %+v", userFormData)
	templates.UserForm(&userFormData).Render(r.Context(), w)
}

func (s *adminHandler) addUserHandler(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Invalid form data",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Decode into your struct
	var formUser templates.UserFormData
	if err := decoder.Decode(&formUser, r.Form); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	user, err := templates.FormDataToStorageUser(&formUser)
	if err != nil {
		slog.Error("parsing user for editing went wrong", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "parsing user for editing went wrong",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	user.ID = uuid.NewString()
	if user == nil {
		log.Panicf("client should have never been nil")
	}
	user.Active = true

	err = s.server.Storage.AddUser(r.Context(), *user)
	if err != nil {
		slog.Error("s.storage.AddUser", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not add user",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Success - show success message
	writeHtmlNotification(templates.NotifInfo{
		Msg:  "Created user successfully",
		Type: notificationTypeSuccess,
	}, r, w)
}

func (s *adminHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	templates.Dashboard().Render(r.Context(), w)
}

func (s *adminHandler) clientsList(w http.ResponseWriter, r *http.Request) {
	// For now, return empty list - will be populated from storage later

	clientsDb, err := s.server.Storage.GetAllClients(r.Context())
	if err != nil {
		slog.Error("s.server.Storage.GetAllClients(r.Context())", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not get list of clients",
			Type: notificationTypeError,
		}, r, w)
	}

	clients := make([]templates.ClientFormData, len(clientsDb))

	for i, clientDb := range clientsDb {
		client := templates.ClientToFormData(&clientDb)
		clients[i] = client
	}

	templates.ClientList(clients).Render(r.Context(), w)
}

func (s *adminHandler) adminUsersList(w http.ResponseWriter, r *http.Request) {
	// For now, return empty list - will be populated from storage later

	usersDb, err := s.server.Storage.GetAllAdminUsers(r.Context())
	if err != nil {
		slog.Error("s.server.Storage.GetAllAdminUsers(r.Context())", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not get list of admin users",
			Type: notificationTypeError,
		}, r, w)
	}

	users := make([]templates.UserFormData, len(usersDb))
	for i, userDb := range usersDb {
		user := templates.StorageUserToFormData(&userDb)
		users[i] = user
	}

	templates.UserList(users).Render(r.Context(), w)
}

func (s *adminHandler) usersList(w http.ResponseWriter, r *http.Request) {
	usersDb, err := s.server.Storage.GetAllUsers(r.Context())
	if err != nil {
		slog.Error("s.server.Storage.GetAllUsers(r.Context())", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not get list of users",
			Type: notificationTypeError,
		}, r, w)

	}

	users := make([]templates.UserFormData, len(usersDb))
	for i, userDb := range usersDb {
		user := templates.StorageUserToFormData(&userDb)
		users[i] = user
	}

	templates.UserList(users).Render(r.Context(), w)
}

// to do. The new version will handle fetching the config, rendering the template, and processing updates.
func (s *adminHandler) configurationFormFragmentHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.server.Storage.GetConfiguration(r.Context())
	if err != nil || cfg == nil {
		slog.Error("failed to get configuration", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not get the baseline configuration",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	nsecRegistered, err := s.server.Storage.NsecIsRegistered(r.Context())
	if err != nil {
		slog.Error("failed to get configuration", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not check the configuration correctly",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	tmplConfig := templates.ConfigurationForm{
		MaxClients:       cfg.MaxClients,
		MaxUsers:         cfg.MaxUsers,
		LastUpdated:      cfg.LastUpdated,
		RegistrationType: cfg.RegistrationType,
	}

	templates.AdminConfigurationForm(tmplConfig, nsecRegistered).Render(r.Context(), w)
}

func (s *adminHandler) configuration(w http.ResponseWriter, r *http.Request) {
	templates.AdminConfigurationPage().Render(r.Context(), w)
}

func (s *adminHandler) updateConfiguration(w http.ResponseWriter, r *http.Request) {
	var inputConfig templates.ConfigurationForm

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to get form", slog.Any("error", err))

		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not parse form",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	// Use the form decoder to fill the struct
	if err := decoder.Decode(&inputConfig, r.PostForm); err != nil {
		slog.Error("failed to decode form into config struct", slog.Any("error", err))

		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not configuration from form",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	emptyNsecField := len(inputConfig.Nsec) == 0
	if !emptyNsecField {
		vtx, err := vertex.NewVertexChecker()
		if err != nil {
			slog.Error("vertex.NewVertexChecker()", slog.Any("error", err))
			if errors.Is(err, vertex.ErrInvalidNsec) {
				writeHtmlNotification(templates.NotifInfo{
					Msg:  "You don't have a valid nsec",
					Type: notificationTypeError,
				}, r, w)
				return
			}
			writeHtmlNotification(templates.NotifInfo{
				Msg:  "Something went wrong while registering the user",
				Type: notificationTypeError,
			}, r, w)
			return
		}

		s.server.Vertex = vtx
	}

	nsecRegistered, err := s.server.Storage.NsecIsRegistered(r.Context())
	if err != nil {
		slog.Error("failed to check if nsec is registered", slog.Any("error", err))
		// Default to an empty config if not found, to avoid nil pointer dereference
		// In a real app, you might want to create a default config here if not found
		templates.NotFoundPage("Could not check the configuration correctly").Render(r.Context(), w)
		return
	}

	// nsec is required for open registration only when WoT is enabled
	if inputConfig.RegistrationType == "open" && !s.server.BypassWoT && (emptyNsecField || !nsecRegistered) {
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "You don't have a valid nsec. You need one for open registration type (unless BYPASS_WOT is enabled)",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	config, err := s.server.Storage.GetConfiguration(r.Context())
	if err != nil {
		slog.Error("failed to get configuration", slog.Any("error", err))
		// Default to an empty config if not found, to avoid nil pointer dereference
		// In a real app, you might want to create a default config here if not found
		templates.NotFoundPage("Could not check the configuration correctly").Render(r.Context(), w)
		return
	}

	// Update the LastUpdated field
	// INFO: pass the values from the form to the config
	transformConfigurationFormForm(inputConfig, config)

	// INFO: Add check for vertex nsec
	if !emptyNsecField {
		privKey, err := utils.GetBtcPrivateKeyFromNsec(inputConfig.Nsec)
		if err != nil {
			writeHtmlNotification(templates.NotifInfo{
				Msg:  "You don't have a valid nsec. You need one for open registration type",
				Type: notificationTypeError,
			}, r, w)
			return
		}
		config.Nsec = privKey.Serialize()
	}

	if err := s.server.Storage.UpdateConfiguration(r.Context(), config); err != nil {
		slog.Error("failed to update configuration", slog.Any("error", err))
		writeHtmlNotification(templates.NotifInfo{
			Msg:  "Could not update configuration",
			Type: notificationTypeError,
		}, r, w)
		return
	}

	writeHtmlNotification(templates.NotifInfo{
		Msg:  "Configuration updated successfully",
		Type: notificationTypeSuccess,
	}, r, w)
}

func transformConfigurationFormForm(form templates.ConfigurationForm, config *storage.Configuration) {
	config.LastUpdated = uint64(time.Now().Unix())
	config.MaxClients = form.MaxClients
	config.MaxUsers = form.MaxUsers
	config.RegistrationType = form.RegistrationType
}
