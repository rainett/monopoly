package config

import (
	"crypto/rand"
	"encoding/base64"
	"log"
)

type Config struct {
	ServerPort    string
	DBPath        string
	SessionSecret string
	MaxOpenConns  int
	MaxIdleConns  int
}

func Load() *Config {
	secret := generateSessionSecret()

	return &Config{
		ServerPort:    ":8080",
		DBPath:        "./monopoly.db",
		SessionSecret: secret,
		MaxOpenConns:  25,
		MaxIdleConns:  5,
	}
}

func generateSessionSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal("Failed to generate session secret:", err)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
