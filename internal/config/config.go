package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	DatabaseURL        string
	JWTSecret          string
	ServicesTicketsURL string
	InternalToken      string
	LogLevel           string
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8007"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://checkin_user:checkin_pass@localhost:5432/checkin_db?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "super-secure-shared-secret-entradasya-2026"
	}

	ticketsURL := os.Getenv("SERVICES_TICKETS_URL")
	if ticketsURL == "" {
		ticketsURL = "http://localhost:8005"
	}

	internalToken := os.Getenv("INTERNAL_TOKEN")
	if internalToken == "" {
		internalToken = "internal-service-secret-entradasya-2026"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	return &Config{
		Port:               port,
		DatabaseURL:        dbURL,
		JWTSecret:          jwtSecret,
		ServicesTicketsURL: ticketsURL,
		InternalToken:      internalToken,
		LogLevel:           logLevel,
	}
}
