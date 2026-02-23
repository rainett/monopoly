package http

import (
	"log"
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

	// Static files
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static")))
}

func (s *Server) Start(addr string) error {
	log.Printf("Server starting on %s", addr)
	return http.ListenAndServe(addr, s.router)
}
