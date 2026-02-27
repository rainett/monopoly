package http

import (
	"monopoly/auth"
	"monopoly/game"
	"monopoly/store"
	"monopoly/ws"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type Server struct {
	router   *mux.Router
	handlers *Handlers
}

func NewServer(authService *auth.Service, lobby *game.Lobby, engine *game.Engine, wsManager *ws.Manager, store store.Store) *Server {
	router := mux.NewRouter()
	handlers := NewHandlers(authService, lobby, engine, wsManager, store)

	server := &Server{
		router:   router,
		handlers: handlers,
	}

	server.setupRoutes(authService)
	return server
}

func (s *Server) setupRoutes(authService *auth.Service) {
	// Apply global middleware
	s.router.Use(LoggingMiddleware)
	s.router.Use(SecurityHeadersMiddleware)
	s.router.Use(CORSMiddleware)

	// CSRF note: SameSite=Lax on the session cookie prevents cross-site POST
	// requests from including the cookie, providing CSRF protection for all
	// state-changing endpoints without needing a token-based scheme.

	// Rate limiters for auth endpoints
	loginLimiter := NewRateLimiter(5.0/60.0, 5)
	registerLimiter := NewRateLimiter(3.0/60.0, 3)

	// Auth routes (public) with rate limiting
	s.router.Handle("/api/auth/register", registerLimiter.Middleware(http.HandlerFunc(s.handlers.Register))).Methods("POST")
	s.router.Handle("/api/auth/login", loginLimiter.Middleware(http.HandlerFunc(s.handlers.Login))).Methods("POST")

	// Protected routes
	protected := s.router.PathPrefix("/api").Subrouter()
	protected.Use(AuthMiddleware(authService))

	protected.HandleFunc("/auth/logout", s.handlers.Logout).Methods("POST")
	protected.HandleFunc("/lobby/games", s.handlers.ListGames).Methods("GET")
	protected.HandleFunc("/lobby/create", s.handlers.CreateGame).Methods("POST")
	protected.HandleFunc("/lobby/join/{gameId}", s.handlers.JoinGame).Methods("POST")
	protected.HandleFunc("/lobby/games/{gameId}", s.handlers.GetGame).Methods("GET")

	// WebSocket route (protected)
	wsRouter := s.router.PathPrefix("/ws").Subrouter()
	wsRouter.Use(AuthMiddleware(authService))
	wsRouter.HandleFunc("/game/{gameId}", s.handlers.HandleWebSocket)

	// Catch-all for unmatched API routes — return JSON 404 instead of SPA HTML
	s.router.PathPrefix("/api/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})

	// Static files with cache-control (no-cache forces revalidation via If-Modified-Since)
	s.router.PathPrefix("/css/").Handler(noCacheHandler(http.StripPrefix("/css/", http.FileServer(http.Dir("./static/css")))))
	s.router.PathPrefix("/js/").Handler(noCacheHandler(http.StripPrefix("/js/", http.FileServer(http.Dir("./static/js")))))
	s.router.PathPrefix("/templates/").Handler(noCacheHandler(http.StripPrefix("/templates/", http.FileServer(http.Dir("./static/templates")))))

	// SPA fallback - serve index.html for all other routes
	s.router.PathPrefix("/").HandlerFunc(s.serveSPA)
}

func noCacheHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, "./static/index.html")
}

func (s *Server) GetHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
