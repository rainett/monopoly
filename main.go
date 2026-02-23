package main

import (
	"context"
	"log"
	"monopoly/auth"
	"monopoly/config"
	"monopoly/game"
	"monopoly/http"
	"monopoly/store"
	"monopoly/ws"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("Starting Monopoly server...")

	// Load configuration
	cfg := config.Load()
	log.Printf("Configuration loaded - Server port: %s, DB path: %s", cfg.ServerPort, cfg.DBPath)

	// Initialize database
	db, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Println("Database initialized successfully")

	// Initialize services
	sessionManager := auth.NewSessionManager(cfg.SessionSecret)
	authService := auth.NewService(db, sessionManager)
	lobby := game.NewLobby(db)
	engine := game.NewEngine(db)
	wsManager := ws.NewManager(engine)

	// Initialize HTTP server
	server := http.NewServer(authService, lobby, engine, wsManager, db)

	// Start server in a goroutine
	go func() {
		log.Printf("Server listening on http://localhost%s", cfg.ServerPort)
		if err := server.Start(cfg.ServerPort); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Cleanup (database will be closed by defer)
	select {
	case <-ctx.Done():
		log.Println("Shutdown timeout exceeded")
	default:
		log.Println("Server stopped")
	}
}
