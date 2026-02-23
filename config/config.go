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
}

func Load() *Config {
	secret := generateSessionSecret()

	return &Config{
		ServerPort:    ":8080",
		DBPath:        "./monopoly.db",
		SessionSecret: secret,
	}
}

func generateSessionSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal("Failed to generate session secret:", err)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
