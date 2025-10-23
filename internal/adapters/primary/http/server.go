package http

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type Server struct {
	app      *fiber.App
	cfg      config.ServerConfig
	handlers *Handlers
}

func NewServer(cfg config.ServerConfig, handlers *Handlers) *Server {
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		ErrorHandler: errorHandler,
	})

	// Middleware
	app.Use(requestid.New())
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	s := &Server{
		app:      app,
		cfg:      cfg,
		handlers: handlers,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Health checks
	s.app.Get("/health", s.handlers.Health)
	s.app.Get("/ready", s.handlers.Ready)

	// API routes
	api := s.app.Group("/api")
	{
		api.Post("/notify", s.handlers.Notify)
		api.Post("/refresh", s.handlers.Refresh)
		api.Get("/media", s.handlers.GetMedia)
		api.Get("/nzbs", s.handlers.GetNZBs)
	}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	log.Info().Str("addr", addr).Msg("Starting HTTP server")
	return s.app.Listen(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down HTTP server")
	return s.app.ShutdownWithContext(ctx)
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	log.Error().Err(err).Int("status", code).Msg("HTTP error")

	return c.Status(code).JSON(fiber.Map{
		"error":   message,
		"status":  code,
		"request_id": c.Locals("requestid"),
	})
}
