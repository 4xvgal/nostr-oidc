package web

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lescuer97/nostr-oicd/vertex"
	"github.com/zitadel/logging"
	"golang.org/x/text/language"

	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	pathLoggedOut = "/logged-out"
)

type Storage interface {
	op.Storage
	authenticate
	deviceAuthenticate
	administration
}

type Server struct {
	Storage      Storage
	Vertex       *vertex.VertexChecker
	OIDCProvider op.OpenIDProvider
	BypassWoT    bool
}

// ExternalWhitelistDB interface for external database whitelist checks
type ExternalWhitelistDB interface {
	IsNpubWhitelisted(ctx context.Context, pubkey interface{}) (bool, error)
}

// simple counter for request IDs
var counter atomic.Int64

func LogRequestURL(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fullURL := r.URL.String()
		if r.URL.Scheme == "" {
			// Add scheme and host if missing (common in proxies or local envs)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			fullURL = scheme + "://" + r.Host + r.RequestURI
		}

		log.Printf("Incoming request: %s %s\n", r.Method, fullURL)

		next.ServeHTTP(w, r)
	})
}

// Use one of the pre-made clients in storage/clients.go or register a new one.
func SetupServer(server *Server, extraOptions ...op.Option) chi.Router {

	config, err := server.Storage.GetConfiguration(context.Background())
	if err != nil {
		log.Panicf("could not get the configuration. %+v", err)

	}

	staticFS := getStaticFileSystem()
	fileServer := http.FileServer(staticFS)

	router := chi.NewRouter()
	router.Use(LogRequestURL)
	router.Use(logging.Middleware(
		logging.WithIDFunc(func() slog.Attr {
			return slog.Int64("id", counter.Add(1))
		}),
	))
	router.Use(middleware.Compress(5))
	router.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// for simplicity, we provide a very small default page for users who have signed out
	router.HandleFunc(pathLoggedOut, func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("signed out successfully"))
		// no need to check/log error, this will be handled by the middleware.
	})

	// creation of the OpenIDProvider with the just created in-memory Storage
	provider, err := newOP(
		server.Storage,
		func(insecure bool) (op.IssuerFromRequest, error) {
			return func(r *http.Request) string {
				// issuer := r.Header.Get("issuer")
				// log.Printf("\n issuer: %+v", issuer)
				return "http://localhost:8082"
			}, nil
		},
		[32]byte(config.EncryptionKey),
		extraOptions...,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Store the provider in the server for access in handlers
	server.OIDCProvider = provider

	//the provider will only take care of the OpenID Protocol, so there must be some sort of UI for the login process
	//for the simplicity of the example this means a simple page with username and password field
	//be sure to provide an IssuerInterceptor with the IssuerFromRequest from the OP so the login can select / and pass it to the storage
	l := NewLogin(server.Storage, op.AuthCallbackURL(provider), op.NewIssuerInterceptor(provider.IssuerFromRequest))
	// op.Co
	// oidc.Discog

	// regardless of how many pages / steps there are in the process, the UI must be registered in the router,
	// so we will direct all calls to /login to the login UI
	router.Mount("/login", http.StripPrefix("/login", l.router))

	// Mount signup routes
	signupRouter := NewSignupHandler(server.Storage, server.Vertex, server.BypassWoT)
	router.Mount("/signup", http.StripPrefix("/signup", signupRouter))

	adminRouter := NewAdminHandler(server)
	// router.Mount("/admin", http.StripPrefix("/admin", adminRouter))
	router.Mount("/admin", adminRouter)

	router.Route("/device", func(r chi.Router) {
		registerDeviceAuth(server.Storage, r)
	})

	// router.Route("/device", func(r chi.Router) {
	// 	registerDeviceAuth(server.Storage, r)
	// })

	handler := http.Handler(provider)

	// we register the http handler of the OP on the root, so that the discovery endpoint (/.well-known/openid-configuration)
	// is served on the correct path
	//
	// if your issuer ends with a path (e.g. http://localhost:9998/custom/path/),
	// then you would have to set the path prefix (/custom/path/)
	router.Mount("/", handler)

	return router
}

// newOP will create an OpenID Provider for localhost on a specified port
// and a predefined default logout uri
// it will enable all options (see descriptions)
func newOP(
	storage op.Storage,
	issuer func(insecure bool) (op.IssuerFromRequest, error),
	key [32]byte, // encryption key
	extraOptions ...op.Option,
) (op.OpenIDProvider, error) {
	config := &op.Config{
		CryptoKey: key,

		// will be used if the end_session endpoint is called without a post_logout_redirect_uri
		DefaultLogoutRedirectURI: pathLoggedOut,

		// enables code_challenge_method S256 for PKCE (and therefore PKCE in general)
		CodeMethodS256: true,

		// enables additional client_id/client_secret authentication by form post (not only HTTP Basic Auth)
		AuthMethodPost: false,

		// enables additional authentication by using private_key_jwt
		AuthMethodPrivateKeyJWT: false,

		// enables refresh_token grant use
		GrantTypeRefreshToken: true,

		// Standard OIDC scopes - use "offline_access" to request refresh tokens, not "refresh_token"
		SupportedScopes: []string{"openid", "profile", "email", "offline_access"},

		// enables use of the `request` Object parameter
		RequestObjectSupported: true,

		// this example has only static texts (in English), so we'll set the here accordingly
		SupportedUILocales: []language.Tag{language.English},

		DeviceAuthorization: op.DeviceAuthorizationConfig{
			Lifetime:     5 * time.Minute,
			PollInterval: 5 * time.Second,
			UserFormPath: "/device",
			UserCode:     op.UserCodeBase20,
		},
	}
	provider, err := op.NewProvider(config, storage,
		issuer,
		append([]op.Option{
			// op.Is
			//we must explicitly allow the use of the http issuer
			op.WithAllowInsecure(),
			// as an example on how to customize an endpoint this will change the authorization_endpoint from /authorize to /auth
			// op.WithCustomAuthEndpoint(op.NewEndpoint("auth")),
			// Pass our logger to the OP
			op.WithLogger(slog.Default().WithGroup("op")),
		}, extraOptions...)...,
	)
	if err != nil {
		return nil, err
	}
	return provider, nil
}
