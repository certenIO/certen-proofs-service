// Copyright 2025 Certen Protocol
//
// Configuration for Certen Proof Artifact Service
// A standalone service for proof storage, retrieval, and exploration

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds configuration for the Proof Artifact Service
type Config struct {
	// Server Configuration
	ListenAddr  string
	MetricsAddr string
	HealthAddr  string

	// Database Configuration
	DatabaseURL         string
	DatabaseMaxConns    int
	DatabaseMinConns    int
	DatabaseMaxIdleTime int  // seconds
	DatabaseMaxLifetime int  // seconds
	DatabaseRequired    bool // If true, startup fails if database connection fails

	// Service Identification
	ValidatorID string
	LogLevel    string

	// Security Configuration
	JWTSecret   string
	CORSOrigins []string
	TLSEnabled  bool

	// Rate Limiting
	RateLimitRequests int
	RateLimitWindow   int

	// API Configuration
	APIKeyRequired bool
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		// Server Configuration
		ListenAddr:  getEnv("API_HOST", "0.0.0.0") + ":" + getEnv("API_PORT", "8080"),
		MetricsAddr: getEnv("API_HOST", "0.0.0.0") + ":" + getEnv("METRICS_PORT", "9090"),
		HealthAddr:  getEnv("API_HOST", "0.0.0.0") + ":" + getEnv("HEALTH_CHECK_PORT", "8081"),

		// Database Configuration
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://certen:certen@localhost:5432/certen_proofs?sslmode=disable"),
		DatabaseMaxConns:    getEnvInt("DATABASE_MAX_CONNS", 25),
		DatabaseMinConns:    getEnvInt("DATABASE_MIN_CONNS", 5),
		DatabaseMaxIdleTime: getEnvInt("DATABASE_MAX_IDLE_TIME", 300),
		DatabaseMaxLifetime: getEnvInt("DATABASE_MAX_LIFETIME", 3600),
		DatabaseRequired:    getEnvBool("DATABASE_REQUIRED", true),

		// Service Configuration
		ValidatorID: getEnv("SERVICE_ID", "proof-service-1"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		// Security Configuration
		JWTSecret:   getEnv("JWT_SECRET", ""),
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:3001"), ","),
		TLSEnabled:  getEnvBool("TLS_ENABLED", false),

		// Rate Limiting
		RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindow:   getEnvInt("RATE_LIMIT_WINDOW", 60),

		// API Configuration
		APIKeyRequired: getEnvBool("API_KEY_REQUIRED", false),
	}

	return cfg, nil
}

// Validate checks that all required configuration is present
func (c *Config) Validate() error {
	var errors []string

	// Database is required
	if c.DatabaseURL == "" {
		errors = append(errors, "DATABASE_URL is required but not set")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// ValidateForDevelopment performs relaxed validation suitable for local development
func (c *Config) ValidateForDevelopment() error {
	// For development, we just need a database URL
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
