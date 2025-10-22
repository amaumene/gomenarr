package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/amaumene/gomenarr/internal/infra"
	"github.com/rs/zerolog/log"
)

func main() {
	// Initialize application
	app, err := infra.InitializeApplication()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Info().Msg("Gomenarr worker started")

	// Start orchestrator (blocking)
	go func() {
		if err := app.Orchestrator.Start(ctx); err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("Orchestrator error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down worker...")
	cancel()

	log.Info().Msg("Shutdown complete")
}
