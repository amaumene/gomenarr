package controllers

import (
	"context"
	"fmt"

	"github.com/amaumene/gomenarr/internal/models"
	"github.com/amaumene/gomenarr/internal/services/torbox"
	"github.com/amaumene/gomenarr/internal/services/trakt"
	"github.com/sirupsen/logrus"
)

// CleanupController handles cleanup of watched and removed content
type CleanupController struct {
	db           *models.Database
	torboxClient *torbox.Client
	traktClient  *trakt.Client
	syncDays     int
	logger       *logrus.Logger
}

// NewCleanupController creates a new cleanup controller
func NewCleanupController(db *models.Database, torboxClient *torbox.Client, traktClient *trakt.Client, syncDays int, logger *logrus.Logger) *CleanupController {
	return &CleanupController{
		db:           db,
		torboxClient: torboxClient,
		traktClient:  traktClient,
		syncDays:     syncDays,
		logger:       logger,
	}
}

// CleanupRemovedFromTrakt removes media items that are no longer in Trakt lists
// This is called immediately after sync
func (c *CleanupController) CleanupRemovedFromTrakt(ctx context.Context) error {
	c.logger.Info("Starting cleanup of content removed from Trakt")

	medias, err := c.db.GetMediasNotInTrakt()
	if err != nil {
		return fmt.Errorf("failed to get medias not in Trakt: %w", err)
	}

	c.logger.WithField("count", len(medias)).Info("Found medias removed from Trakt")

	for _, media := range medias {
		c.logger.WithFields(logrus.Fields{
			"media_id": media.ID,
			"title":    media.Title,
		}).Info("Cleaning up removed media")

		// Get all NZBs for this media
		nzbs, err := c.db.GetNZBsByMediaID(media.ID)
		if err != nil {
			c.logger.WithError(err).Error("Failed to get NZBs")
			continue
		}

		// Cancel/delete TorBox jobs
		for _, nzb := range nzbs {
			if nzb.TorBoxJobID != "" {
				if err := c.torboxClient.DeleteJob(nzb.TorBoxJobID); err != nil {
					c.logger.WithError(err).WithField("job_id", nzb.TorBoxJobID).Warn("Failed to delete TorBox job")
				}
			}
		}

		// Delete NZBs from database
		if err := c.db.DeleteNZBsByMediaID(media.ID); err != nil {
			c.logger.WithError(err).Error("Failed to delete NZBs")
		}

		// Delete media from database
		if err := c.db.DeleteMedia(media.ID); err != nil {
			c.logger.WithError(err).Error("Failed to delete media")
		}
	}

	c.logger.WithField("cleaned", len(medias)).Info("Cleanup of removed content completed")
	return nil
}

// CleanupWatched cleans up watched content (conditional cleanup)
// This runs hourly
func (c *CleanupController) CleanupWatched(ctx context.Context) error {
	c.logger.Info("Starting cleanup of watched content")

	// Get recently watched items from Trakt
	watchedItems, err := c.traktClient.GetRecentlyWatched(ctx, c.syncDays)
	if err != nil {
		c.logger.WithError(err).Error("Failed to get watched items, skipping cleanup")
		return fmt.Errorf("failed to get watched items: %w", err)
	}

	c.logger.WithField("count", len(watchedItems)).Debug("Retrieved watched items")

	cleanedCount := 0

	for _, item := range watchedItems {
		if item.MediaType == "movie" {
			// Movies: delete immediately
			if err := c.cleanupMovie(item); err != nil {
				c.logger.WithError(err).Error("Failed to cleanup movie")
			} else {
				cleanedCount++
			}
		} else if item.MediaType == "episode" {
			// Episodes: check if part of season pack or single episode
			if err := c.cleanupEpisode(ctx, item); err != nil {
				c.logger.WithError(err).Error("Failed to cleanup episode")
			} else {
				cleanedCount++
			}
		}
	}

	c.logger.WithField("cleaned", cleanedCount).Info("Cleanup of watched content completed")
	return nil
}

// cleanupMovie deletes a watched movie
func (c *CleanupController) cleanupMovie(item trakt.WatchedItem) error {
	// Find media
	media, err := c.db.GetMediaByIMDBID(item.IMDBId, models.MediaTypeMovie, nil, nil)
	if err != nil {
		// Media not found, already cleaned up or never downloaded
		return nil
	}

	// Only clean up if still in Trakt (InTrakt=true)
	if !media.InTrakt {
		return nil
	}

	c.logger.WithFields(logrus.Fields{
		"media_id": media.ID,
		"title":    media.Title,
	}).Info("Cleaning up watched movie")

	return c.deleteMedia(media)
}

// cleanupEpisode handles cleanup of watched episodes
func (c *CleanupController) cleanupEpisode(ctx context.Context, item trakt.WatchedItem) error {
	// Find all NZBs that might contain this episode
	allMedias, err := c.db.GetAllMedias()
	if err != nil {
		return err
	}

	for _, media := range allMedias {
		if media.IMDBId != item.IMDBId || media.MediaType != models.MediaTypeTV {
			continue
		}

		// Only process if still in Trakt
		if !media.InTrakt {
			continue
		}

		// Get NZBs for this media
		nzbs, err := c.db.GetNZBsByMediaID(media.ID)
		if err != nil {
			c.logger.WithError(err).Error("Failed to get NZBs")
			continue
		}

		for _, nzb := range nzbs {
			if nzb.IsSeasonPack {
				// Season pack: update watched status and check if last episode
				if err := c.handleSeasonPackWatched(ctx, nzb, item); err != nil {
					c.logger.WithError(err).Error("Failed to handle season pack")
				}
			} else {
				// Single episode: delete if matches
				if media.SeasonNumber != nil && *media.SeasonNumber == item.Season &&
					media.EpisodeNumber != nil && *media.EpisodeNumber == item.Episode {
					c.logger.WithFields(logrus.Fields{
						"media_id": media.ID,
						"season":   item.Season,
						"episode":  item.Episode,
					}).Info("Cleaning up watched episode")
					return c.deleteMedia(media)
				}
			}
		}
	}

	return nil
}

// handleSeasonPackWatched updates season pack watched status and deletes if last episode watched
func (c *CleanupController) handleSeasonPackWatched(ctx context.Context, nzb *models.NZB, item trakt.WatchedItem) error {
	// Update episode watched status
	updated := false
	for i := range nzb.Episodes {
		if nzb.Episodes[i].EpisodeNumber == item.Episode {
			nzb.Episodes[i].Watched = true
			watchedAt := item.WatchedAt
			nzb.Episodes[i].WatchedAt = &watchedAt
			updated = true
			break
		}
	}

	if !updated {
		return nil
	}

	// Save updated NZB
	if err := c.db.UpdateNZB(nzb); err != nil {
		return err
	}

	// Check if last episode is watched
	if len(nzb.Episodes) > 0 {
		lastEpisode := nzb.Episodes[len(nzb.Episodes)-1]
		if lastEpisode.Watched {
			c.logger.WithFields(logrus.Fields{
				"nzb_id": nzb.ID,
				"season": nzb.Season,
			}).Info("Last episode of season pack watched, cleaning up")

			// Get media and delete
			media, err := c.db.GetMediaByID(nzb.MediaID)
			if err != nil {
				return err
			}
			return c.deleteMedia(media)
		}
	}

	return nil
}

// deleteMedia deletes a media item and its associated data
func (c *CleanupController) deleteMedia(media *models.Media) error {
	// Get all NZBs
	nzbs, err := c.db.GetNZBsByMediaID(media.ID)
	if err != nil {
		return err
	}

	// Delete TorBox jobs
	for _, nzb := range nzbs {
		if nzb.TorBoxJobID != "" {
			if err := c.torboxClient.DeleteJob(nzb.TorBoxJobID); err != nil {
				c.logger.WithError(err).Warn("Failed to delete TorBox job")
			}
		}
	}

	// Delete NZBs
	if err := c.db.DeleteNZBsByMediaID(media.ID); err != nil {
		return err
	}

	// Delete media
	return c.db.DeleteMedia(media.ID)
}
