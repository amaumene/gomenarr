package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type DownloadService struct {
	mediaRepo      ports.MediaRepository
	nzbRepo        ports.NZBRepository
	downloadClient ports.DownloadClient
	cfg            config.NZBGetConfig
}

func NewDownloadService(
	mediaRepo ports.MediaRepository,
	nzbRepo ports.NZBRepository,
	downloadClient ports.DownloadClient,
	cfg config.NZBGetConfig,
) *DownloadService {
	return &DownloadService{
		mediaRepo:      mediaRepo,
		nzbRepo:        nzbRepo,
		downloadClient: downloadClient,
		cfg:            cfg,
	}
}

func (s *DownloadService) DownloadMedia(ctx context.Context) error {
	log.Info().Msg("Starting download process")

	// Get all media not on disk
	mediaList, err := s.mediaRepo.FindNotOnDisk(ctx)
	if err != nil {
		return fmt.Errorf("failed to get media not on disk: %w", err)
	}

	// Get current queue
	queue, err := s.downloadClient.GetQueue(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queue: %w", err)
	}

	// Get history
	history, err := s.downloadClient.GetHistory(ctx)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	// Track processed season packs
	processedSeasons := make(map[string]bool)

	count := 0
	for _, media := range mediaList {
		// Season pack deduplication
		if media.IsEpisode() {
			seasonKey := fmt.Sprintf("%s_S%d", media.IMDB, media.Season)
			if processedSeasons[seasonKey] {
				log.Debug().Str("season_key", seasonKey).Msg("Skipping episode, season pack already processed")
				continue
			}
		}

		// Get best NZB
		nzb, err := s.nzbRepo.FindBestByTraktID(ctx, media.TraktID)
		if err != nil {
			log.Debug().Int64("trakt_id", media.TraktID).Msg("No NZB found for media")
			continue
		}

		// Check if already in queue
		if s.isInQueue(nzb.Title, queue) {
			log.Debug().Str("title", nzb.Title).Msg("Already in queue")
			continue
		}

		// Check if already in history
		if s.isInHistory(media.DownloadID, history) {
			log.Debug().Int64("download_id", media.DownloadID).Msg("Already in history")
			continue
		}

		// Download
		if err := s.QueueNZB(ctx, media, nzb); err != nil {
			log.Error().Err(err).Str("title", nzb.Title).Msg("Failed to queue NZB")
			continue
		}

		// Mark season as processed if it's a season pack
		if media.IsEpisode() && nzb.IsSeasonPack() {
			seasonKey := fmt.Sprintf("%s_S%d", media.IMDB, media.Season)
			processedSeasons[seasonKey] = true
			log.Info().Str("season_key", seasonKey).Msg("Marked season pack as processed")
		}

		count++
	}

	log.Info().Int("count", count).Msg("Queued downloads")
	return nil
}

func (s *DownloadService) QueueNZB(ctx context.Context, media *domain.Media, nzb *domain.NZB) error {
	log.Info().Str("title", nzb.Title).Int64("trakt_id", media.TraktID).Msg("Queueing NZB")

	// Download NZB file
	resp, err := http.Get(nzb.Link)
	if err != nil {
		return fmt.Errorf("failed to download NZB: %w", err)
	}
	defer resp.Body.Close()

	nzbContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read NZB content: %w", err)
	}

	// Queue to NZBGet
	params := map[string]string{
		"Trakt": fmt.Sprintf("%d", media.TraktID),
	}

	downloadID, err := s.downloadClient.QueueDownload(
		ctx,
		nzbContent,
		nzb.Title+".nzb",
		s.cfg.Category,
		s.cfg.Priority,
		params,
	)
	if err != nil {
		return fmt.Errorf("failed to queue download: %w", err)
	}

	// Update media with download ID
	media.DownloadID = downloadID
	if err := s.mediaRepo.Update(ctx, media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	log.Info().Int64("download_id", downloadID).Msg("Successfully queued NZB")
	return nil
}

func (s *DownloadService) isInQueue(title string, queue []ports.DownloadQueueItem) bool {
	for _, item := range queue {
		if strings.Contains(item.Title, title) || strings.Contains(title, item.Title) {
			return true
		}
	}
	return false
}

func (s *DownloadService) isInHistory(downloadID int64, history []ports.DownloadHistoryItem) bool {
	if downloadID == 0 {
		return false
	}
	for _, item := range history {
		if item.ID == downloadID {
			return true
		}
	}
	return false
}
