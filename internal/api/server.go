package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/amaumene/gomenarr/internal/api/handlers"
	"github.com/amaumene/gomenarr/internal/api/middleware"
	"github.com/amaumene/gomenarr/internal/config"
	"github.com/amaumene/gomenarr/internal/controllers"
	"github.com/amaumene/gomenarr/internal/models"
	"github.com/sirupsen/logrus"
)

// Server represents the HTTP server
type Server struct {
	server       *http.Server
	db           *models.Database
	downloadCtrl *controllers.DownloadController
	logger       *logrus.Logger
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.Config, db *models.Database, downloadCtrl *controllers.DownloadController, logger *logrus.Logger) *Server {
	s := &Server{
		db:           db,
		downloadCtrl: downloadCtrl,
		logger:       logger,
	}

	mux := http.NewServeMux()
	s.setupRoutes(mux, cfg)

	s.server = &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      middleware.Logging(mux, logger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes(mux *http.ServeMux, cfg *config.Config) {
	// Health check
	healthHandler := handlers.NewHealthHandler(s.logger)
	mux.HandleFunc("/health", healthHandler.ServeHTTP)

	// Status endpoint
	statusHandler := handlers.NewStatusHandler(s.db, s.logger)
	mux.HandleFunc("/status", statusHandler.ServeHTTP)

	// TorBox webhook
	webhookHandler := handlers.NewWebhookHandler(s.downloadCtrl, s.logger)
	mux.HandleFunc("/api/webhook/torbox", webhookHandler.ServeHTTP)
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.WithField("port", s.server.Addr).Info("Starting HTTP server")

	errChan := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

// Shutdown gracefully shuts down the HTTP server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.server.Shutdown(shutdownCtx)
}
