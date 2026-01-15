// Copyright 2025 Certen Protocol
//
// Certen Proof Artifact Service
// A standalone service for storing, retrieving, and exploring proof artifacts
//
// This service provides:
// - REST API for proof discovery and retrieval
// - PostgreSQL storage for proof artifacts
// - Web UI for proof exploration (served separately)

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/certen/proofs-service/pkg/config"
	"github.com/certen/proofs-service/pkg/database"
	"github.com/certen/proofs-service/pkg/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if os.Getenv("DEVELOPMENT_MODE") == "true" {
		if err := cfg.ValidateForDevelopment(); err != nil {
			log.Fatalf("Configuration validation failed: %v", err)
		}
		log.Println("Running in DEVELOPMENT mode - relaxed security validation")
	}

	// Set up logging
	logger := log.New(os.Stdout, "[ProofService] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("Starting Certen Proof Artifact Service...")
	logger.Printf("Service ID: %s", cfg.ValidatorID)

	// Connect to database
	logger.Printf("Connecting to database...")
	dbClient, err := database.NewClient(cfg)
	if err != nil {
		if cfg.DatabaseRequired {
			logger.Fatalf("Failed to connect to database: %v", err)
		}
		logger.Printf("WARNING: Database connection failed: %v", err)
		logger.Printf("Service will run with limited functionality")
	} else {
		logger.Printf("Database connected successfully")
		defer dbClient.Close()
	}

	// Create repositories
	var repos *database.Repositories
	if dbClient != nil {
		repos = database.NewRepositories(dbClient)
	}

	// Create HTTP handlers
	proofHandlers := server.NewProofHandlers(repos, cfg.ValidatorID, logger)
	bundleHandlers := server.NewBundleHandlers(repos, &server.BundleHandlersConfig{
		ValidatorID:        cfg.ValidatorID,
		RateLimitPerMinute: cfg.RateLimitRequests,
	}, logger)

	// Set up HTTP router
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"
		if dbClient == nil {
			status = "degraded"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"%s","service":"proof-service","version":"1.0.0"}`, status)
	})

	// API v1 Proof Discovery endpoints
	mux.HandleFunc("/api/v1/proofs/tx/", proofHandlers.HandleGetProofByTxHash)
	mux.HandleFunc("/api/v1/proofs/account/", proofHandlers.HandleGetProofsByAccount)
	mux.HandleFunc("/api/v1/proofs/batch/", proofHandlers.HandleGetProofsByBatch)
	mux.HandleFunc("/api/v1/proofs/anchor/", proofHandlers.HandleGetProofsByAnchor)
	mux.HandleFunc("/api/v1/proofs/query", proofHandlers.HandleQueryProofs)

	// API v1 Proof Request endpoints
	mux.HandleFunc("/api/v1/proofs/request", bundleHandlers.HandleRequestProof)
	mux.HandleFunc("/api/v1/proofs/request/", bundleHandlers.HandleGetRequestStatus)

	// API v1 Verification endpoints
	mux.HandleFunc("/api/v1/proofs/verify/merkle", bundleHandlers.HandleVerifyMerkle)
	mux.HandleFunc("/api/v1/proofs/verify/governance", bundleHandlers.HandleVerifyGovernance)

	// API v1 Proof Detail endpoints (with sub-paths)
	mux.HandleFunc("/api/v1/proofs/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/bundle/verify"):
			bundleHandlers.HandleVerifyBundle(w, r)
		case strings.HasSuffix(path, "/bundle"):
			bundleHandlers.HandleDownloadBundle(w, r)
		case strings.HasSuffix(path, "/custody"):
			bundleHandlers.HandleGetCustodyChain(w, r)
		default:
			proofHandlers.HandleGetProofByID(w, r)
		}
	})

	// Wrap with CORS middleware
	handler := corsMiddleware(cfg.CORSOrigins)(mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Printf("API server listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Println("Server exited gracefully")
}

// corsMiddleware returns a middleware that handles CORS
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
