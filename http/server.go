package http

import (
	"monopoly/auth"
	"monopoly/game"
	"monopoly/store"
	"monopoly/ws"
	"net/http"

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

	// Auth routes (public)
	s.router.HandleFunc("/api/auth/register", s.handlers.Register).Methods("POST")
	s.router.HandleFunc("/api/auth/login", s.handlers.Login).Methods("POST")

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
