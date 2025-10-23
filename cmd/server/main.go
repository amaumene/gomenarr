package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amaumene/gomenarr/internal/infra"
	"github.com/amaumene/gomenarr/internal/platform/logging"
	"github.com/rs/zerolog/log"
)

func main() {
	// Initialize application
	app, err := infra.InitializeApplication()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
	}

	// Setup logging
	logging.Setup(app.Config.Logging)

	log.Info().Msg("Gomenarr starting...")

	// Check Trakt authentication
	log.Info().Msg("Checking Trakt authentication...")
	if !app.TraktClient.IsAuthenticated() {
		log.Warn().Msg("Not authenticated with Trakt. Starting device code flow...")

		authCtx, authCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer authCancel()

		if err := app.TraktClient.Authenticate(authCtx); err != nil {
			log.Fatal().Err(err).Msg("Trakt authentication failed. Please check your credentials and try again.")
		}

		log.Info().Msg("Authentication successful! Token saved.")
	} else {
		log.Info().Msg("Already authenticated with Trakt")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start orchestrator
	go func() {
		if err := app.Orchestrator.Start(ctx); err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("Orchestrator error")
		}
	}()

	// Start HTTP server
	go func() {
		if err := app.Server.Start(); err != nil {
			log.Fatal().Err(err).Msg("Failed to start HTTP server")
		}
	}()

	log.Info().Msg("Gomenarr started successfully")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down gracefully...")

	// Cancel context to stop orchestrator
	cancel()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	log.Info().Msg("Shutdown complete")
}
