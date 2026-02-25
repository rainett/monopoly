package http

import (
	"monopoly/auth"
	"monopoly/game"
	"monopoly/store"
	"monopoly/ws"
	"net/http"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
)

type Server struct {
	router       *mux.Router
	handlers     *Handlers
	csrfKey      []byte
}

func NewServer(authService *auth.Service, lobby *game.Lobby, engine *game.Engine, wsManager *ws.Manager, store store.Store) *Server {
	router := mux.NewRouter()
	handlers := NewHandlers(authService, lobby, engine, wsManager, store)

	server := &Server{
		router:   router,
		handlers: handlers,
		csrfKey:  []byte(authService.GetSessionManager().GetCSRFKey()),
	}

	server.setupRoutes(authService)
	return server
}

func (s *Server) setupRoutes(authService *auth.Service) {
	// Apply global middleware
	s.router.Use(LoggingMiddleware)
	s.router.Use(SecurityHeadersMiddleware)
	s.router.Use(CORSMiddleware)

	// CSRF protection for POST requests (excluding WebSocket)
	// Secure flag should be true in production with HTTPS
	csrfMiddleware := csrf.Protect(
		s.csrfKey,
		csrf.Secure(false), // Set to true in production
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	// Rate limiters for auth endpoints
	// Login: 5 requests per minute per IP
	loginLimiter := NewRateLimiter(5.0/60.0, 5)
	// Register: 3 requests per minute per IP
	registerLimiter := NewRateLimiter(3.0/60.0, 3)

	// CSRF token endpoint (public)
	s.router.HandleFunc("/api/csrf-token", s.handlers.GetCSRFToken).Methods("GET")

	// Auth routes (public) with rate limiting and CSRF
	s.router.Handle("/api/auth/register", csrfMiddleware(registerLimiter.Middleware(http.HandlerFunc(s.handlers.Register)))).Methods("POST")
	s.router.Handle("/api/auth/login", csrfMiddleware(loginLimiter.Middleware(http.HandlerFunc(s.handlers.Login)))).Methods("POST")

	// Protected routes with CSRF
	protected := s.router.PathPrefix("/api").Subrouter()
	protected.Use(AuthMiddleware(authService))

	protected.Handle("/auth/logout", csrfMiddleware(http.HandlerFunc(s.handlers.Logout))).Methods("POST")
	protected.HandleFunc("/lobby/games", s.handlers.ListGames).Methods("GET")
	protected.Handle("/lobby/create", csrfMiddleware(http.HandlerFunc(s.handlers.CreateGame))).Methods("POST")
	protected.Handle("/lobby/join/{gameId}", csrfMiddleware(http.HandlerFunc(s.handlers.JoinGame))).Methods("POST")
	protected.HandleFunc("/lobby/games/{gameId}", s.handlers.GetGame).Methods("GET")

	// WebSocket route (protected)
	wsRouter := s.router.PathPrefix("/ws").Subrouter()
	wsRouter.Use(AuthMiddleware(authService))
	wsRouter.HandleFunc("/game/{gameId}", s.handlers.HandleWebSocket)

	// Static files (CSS, JS, Templates)
	s.router.PathPrefix("/css/").Handler(http.StripPrefix("/css/", http.FileServer(http.Dir("./static/css"))))
	s.router.PathPrefix("/js/").Handler(http.StripPrefix("/js/", http.FileServer(http.Dir("./static/js"))))
	s.router.PathPrefix("/templates/").Handler(http.StripPrefix("/templates/", http.FileServer(http.Dir("./static/templates"))))

	// SPA fallback - serve index.html for all other routes
	s.router.PathPrefix("/").HandlerFunc(s.serveSPA)
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/index.html")
}

func (s *Server) GetHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
}
