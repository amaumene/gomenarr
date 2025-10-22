package http

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/services"
	"github.com/amaumene/gomenarr/internal/infra/database"
	"github.com/amaumene/gomenarr/internal/orchestrator"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type Handlers struct {
	db              *gorm.DB
	mediaSvc        *services.MediaService
	nzbSvc          *services.NZBService
	notificationSvc *services.NotificationService
	orchestrator    *orchestrator.Orchestrator
	notifyChan      chan *domain.Notification
}

func NewHandlers(
	db *gorm.DB,
	mediaSvc *services.MediaService,
	nzbSvc *services.NZBService,
	notificationSvc *services.NotificationService,
	orch *orchestrator.Orchestrator,
) *Handlers {
	h := &Handlers{
		db:              db,
		mediaSvc:        mediaSvc,
		nzbSvc:          nzbSvc,
		notificationSvc: notificationSvc,
		orchestrator:    orch,
		notifyChan:      make(chan *domain.Notification, 100),
	}

	// Start notification processor
	go h.processNotifications()

	return h
}

func (h *Handlers) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

func (h *Handlers) Ready(c *fiber.Ctx) error {
	if err := database.HealthCheck(h.db); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not ready",
			"error":  err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status": "ready",
	})
}

func (h *Handlers) Notify(c *fiber.Ctx) error {
	// Parse webhook notification from NZBGet
	var req struct {
		Status     string `form:"status"`
		Name       string `form:"name"`
		Path       string `form:"path"`
		DownloadID string `form:"nzbid"`
		TraktID    string `form:"trakt"`
	}

	if err := c.BodyParser(&req); err != nil {
		// Try query params
		req.Status = c.Query("status")
		req.Name = c.Query("name")
		req.Path = c.Query("path")
		req.DownloadID = c.Query("nzbid")
		req.TraktID = c.Query("trakt")
	}

	// Parse status
	var status domain.NotificationStatus
	switch strings.ToUpper(req.Status) {
	case "SUCCESS":
		status = domain.NotificationStatusSuccess
	case "FAILURE", "FAILED":
		status = domain.NotificationStatusFailure
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid status",
		})
	}

	// Parse download ID
	downloadID, _ := strconv.ParseInt(req.DownloadID, 10, 64)

	// Parse Trakt ID
	traktID, err := strconv.ParseInt(req.TraktID, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid trakt_id",
		})
	}

	notification := &domain.Notification{
		Status:     status,
		Name:       req.Name,
		Path:       req.Path,
		DownloadID: downloadID,
		TraktID:    traktID,
	}

	// Send to channel for async processing
	select {
	case h.notifyChan <- notification:
		log.Info().Int64("trakt_id", traktID).Str("status", string(status)).Msg("Notification queued")
	default:
		log.Warn().Msg("Notification channel full, processing synchronously")
		if err := h.notificationSvc.HandleWebhook(c.Context(), notification); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
	}

	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

func (h *Handlers) GetMedia(c *fiber.Ctx) error {
	media, err := h.mediaSvc.GetAll(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"count": len(media),
		"data":  media,
	})
}

func (h *Handlers) GetNZBs(c *fiber.Ctx) error {
	nzbs, err := h.nzbSvc.GetAll(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"count": len(nzbs),
		"data":  nzbs,
	})
}

func (h *Handlers) Refresh(c *fiber.Ctx) error {
	// Manually trigger orchestrator cycle
	go func() {
		log.Info().Msg("Manual refresh triggered")
	}()

	return c.JSON(fiber.Map{
		"status":  "ok",
		"message": "Orchestrator cycle triggered",
	})
}

func (h *Handlers) processNotifications() {
	for notification := range h.notifyChan {
		if err := h.notificationSvc.HandleWebhook(context.Background(), notification); err != nil {
			log.Error().Err(err).Int64("trakt_id", notification.TraktID).Msg("Failed to process notification")
		}
	}
}
