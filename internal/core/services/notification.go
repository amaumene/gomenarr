package services

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/rs/zerolog/log"
)

type NotificationService struct {
	mediaRepo      ports.MediaRepository
	nzbRepo        ports.NZBRepository
	downloadClient ports.DownloadClient
	downloadSvc    *DownloadService
}

func NewNotificationService(
	mediaRepo ports.MediaRepository,
	nzbRepo ports.NZBRepository,
	downloadClient ports.DownloadClient,
	downloadSvc *DownloadService,
) *NotificationService {
	return &NotificationService{
		mediaRepo:      mediaRepo,
		nzbRepo:        nzbRepo,
		downloadClient: downloadClient,
		downloadSvc:    downloadSvc,
	}
}

func (s *NotificationService) HandleWebhook(ctx context.Context, notification *domain.Notification) error {
	log.Info().
		Str("status", string(notification.Status)).
		Int64("trakt_id", notification.TraktID).
		Str("name", notification.Name).
		Msg("Handling webhook notification")

	if notification.Status == domain.NotificationStatusSuccess {
		return s.handleSuccess(ctx, notification)
	}
	return s.handleFailure(ctx, notification)
}

func (s *NotificationService) handleSuccess(ctx context.Context, notification *domain.Notification) error {
	// Get media
	media, err := s.mediaRepo.FindByTraktID(ctx, notification.TraktID)
	if err != nil {
		return fmt.Errorf("failed to find media: %w", err)
	}

	// Update media
	media.OnDisk = true
	media.Path = notification.Path

	if err := s.mediaRepo.Update(ctx, media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	log.Info().Int64("trakt_id", media.TraktID).Str("path", media.Path).Msg("Media marked as on disk")

	// Delete from NZBGet history with retry
	if notification.DownloadID > 0 {
		operation := func() error {
			return s.downloadClient.DeleteFromHistory(ctx, notification.DownloadID)
		}

		b := backoff.NewExponentialBackOff()
		b.MaxElapsedTime = 1 * time.Minute

		if err := backoff.Retry(operation, b); err != nil {
			log.Error().Err(err).Int64("download_id", notification.DownloadID).Msg("Failed to delete from history after retries")
		} else {
			log.Info().Int64("download_id", notification.DownloadID).Msg("Deleted from NZBGet history")
		}
	}

	return nil
}

func (s *NotificationService) handleFailure(ctx context.Context, notification *domain.Notification) error {
	// Mark NZB as failed
	if err := s.nzbRepo.MarkAsFailedByTitle(ctx, notification.Name); err != nil {
		log.Error().Err(err).Str("name", notification.Name).Msg("Failed to mark NZB as failed")
	}

	log.Info().Str("name", notification.Name).Msg("Marked NZB as failed")

	// Get media
	media, err := s.mediaRepo.FindByTraktID(ctx, notification.TraktID)
	if err != nil {
		return fmt.Errorf("failed to find media: %w", err)
	}

	// Get next best NZB
	nzb, err := s.nzbRepo.FindBestByTraktID(ctx, notification.TraktID)
	if err != nil {
		log.Warn().Int64("trakt_id", notification.TraktID).Msg("No alternative NZB found")
		return nil
	}

	// Queue next NZB
	if err := s.downloadSvc.QueueNZB(ctx, media, nzb); err != nil {
		return fmt.Errorf("failed to queue alternative NZB: %w", err)
	}

	log.Info().Str("title", nzb.Title).Msg("Queued alternative NZB")
	return nil
}
