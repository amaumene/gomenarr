package services

import (
	"context"
	"fmt"
	"os"

	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type CleanupService struct {
	mediaRepo   ports.MediaRepository
	nzbRepo     ports.NZBRepository
	traktClient ports.TraktClient
	cfg         config.DownloadConfig
}

func NewCleanupService(
	mediaRepo ports.MediaRepository,
	nzbRepo ports.NZBRepository,
	traktClient ports.TraktClient,
	cfg config.DownloadConfig,
) *CleanupService {
	return &CleanupService{
		mediaRepo:   mediaRepo,
		nzbRepo:     nzbRepo,
		traktClient: traktClient,
		cfg:         cfg,
	}
}

func (s *CleanupService) CleanupWatched(ctx context.Context) error {
	log.Info().Int("days", s.cfg.CleanupWatchedDays).Msg("Starting cleanup of watched media")

	// Get watch history from Trakt
	history, err := s.traktClient.GetWatchHistory(ctx, s.cfg.CleanupWatchedDays)
	if err != nil {
		return fmt.Errorf("failed to get watch history: %w", err)
	}

	if len(history) == 0 {
		log.Info().Msg("No watched items found in history")
		return nil
	}

	// Extract Trakt IDs
	traktIDs := make([]int64, 0, len(history))
	for _, item := range history {
		traktIDs = append(traktIDs, item.TraktID)
	}

	// Get media to delete files
	mediaList, err := s.mediaRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get media list: %w", err)
	}

	// Delete directories if configured
	if s.cfg.DeleteFiles {
		deletedDirs := 0
		for _, media := range mediaList {
			for _, traktID := range traktIDs {
				if media.TraktID == traktID && media.Path != "" {
					if err := os.RemoveAll(media.Path); err != nil {
						log.Error().Err(err).Str("path", media.Path).Msg("Failed to delete directory")
					} else {
						log.Info().Str("path", media.Path).Msg("Deleted directory and all contents")
						deletedDirs++
					}
					break
				}
			}
		}
		log.Info().Int("count", deletedDirs).Msg("Deleted directories from disk")
	}

	// Delete media from database
	log.Debug().Int("count", len(traktIDs)).Msg("Deleting media records from database")
	if err := s.mediaRepo.DeleteByTraktIDs(ctx, traktIDs); err != nil {
		return fmt.Errorf("failed to delete media: %w", err)
	}
	log.Info().Int("count", len(traktIDs)).Msg("Deleted media records")

	// Delete NZBs from database
	log.Debug().Int("count", len(traktIDs)).Msg("Deleting NZB records from database")
	if err := s.nzbRepo.DeleteByTraktIDs(ctx, traktIDs); err != nil {
		return fmt.Errorf("failed to delete NZBs: %w", err)
	}
	log.Info().Int("count", len(traktIDs)).Msg("Deleted NZB records")

	// Clear watched cache to force refresh
	s.traktClient.ClearWatchedCache()

	log.Info().Int("count", len(traktIDs)).Msg("Cleaned up watched media (db, nzb, files)")
	return nil
}
