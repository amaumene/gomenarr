package controllers

import (
	"context"
	"fmt"

	"github.com/amaumene/gomenarr/internal/models"
	"github.com/amaumene/gomenarr/internal/services/trakt"
	"github.com/sirupsen/logrus"
)

// StrategyType represents the type of download strategy
type StrategyType string

const (
	StrategySingleEpisode StrategyType = "single_episode"
	StrategySeasonPack    StrategyType = "season_pack"
	StrategyNext3Episodes StrategyType = "next_3_episodes"
	StrategySingleMovie   StrategyType = "single_movie"
)

// DownloadStrategy represents a download strategy decision
type DownloadStrategy struct {
	Type         StrategyType
	Episodes     []trakt.Episode
	SeasonNumber *int
}

// StrategyController determines download strategies
type StrategyController struct {
	db          *models.Database
	traktClient *trakt.Client
	logger      *logrus.Logger
}

// NewStrategyController creates a new strategy controller
func NewStrategyController(db *models.Database, traktClient *trakt.Client, logger *logrus.Logger) *StrategyController {
	return &StrategyController{
		db:          db,
		traktClient: traktClient,
		logger:      logger,
	}
}

// DetermineStrategy determines the best download strategy for a media item
func (c *StrategyController) DetermineStrategy(ctx context.Context, media *models.Media) (*DownloadStrategy, error) {
	// Movies: Always single movie
	if media.MediaType == models.MediaTypeMovie {
		c.logger.WithFields(logrus.Fields{
			"media_id": media.ID,
			"title":    media.Title,
		}).Debug("Strategy: Single movie")

		return &DownloadStrategy{
			Type:     StrategySingleMovie,
			Episodes: []trakt.Episode{},
		}, nil
	}

	// TV Shows: Strategy depends on source
	if media.Source == models.SourceWatchlist {
		// Watchlist: Next single episode
		return c.nextEpisodeStrategy(ctx, media)
	}

	// Favorites: Compare season pack vs next 3 episodes
	// We'll return both strategies and let the search controller handle comparison
	return c.favoritesStrategy(ctx, media)
}

// nextEpisodeStrategy determines strategy for next single episode
func (c *StrategyController) nextEpisodeStrategy(ctx context.Context, media *models.Media) (*DownloadStrategy, error) {
	progress, err := c.traktClient.GetShowProgress(ctx, media.IMDBId)
	if err != nil {
		return nil, fmt.Errorf("failed to get show progress: %w", err)
	}

	if progress.NextEpisode == nil {
		return nil, fmt.Errorf("no unwatched episodes found")
	}

	c.logger.WithFields(logrus.Fields{
		"media_id": media.ID,
		"title":    media.Title,
		"season":   progress.NextEpisode.Season,
		"episode":  progress.NextEpisode.Episode,
	}).Debug("Strategy: Single episode from watchlist")

	return &DownloadStrategy{
		Type:     StrategySingleEpisode,
		Episodes: []trakt.Episode{*progress.NextEpisode},
	}, nil
}

// favoritesStrategy determines strategy for favorites (season pack or next 3 episodes)
func (c *StrategyController) favoritesStrategy(ctx context.Context, media *models.Media) (*DownloadStrategy, error) {
	progress, err := c.traktClient.GetShowProgress(ctx, media.IMDBId)
	if err != nil {
		return nil, fmt.Errorf("failed to get show progress: %w", err)
	}

	if len(progress.UnwatchedEpisodes) == 0 {
		return nil, fmt.Errorf("no unwatched episodes found")
	}

	// DEBUG: Log ALL unwatched episodes from Trakt
	c.logger.WithFields(logrus.Fields{
		"media_id":        media.ID,
		"title":           media.Title,
		"total_unwatched": len(progress.UnwatchedEpisodes),
	}).Info("Trakt unwatched episodes")
	for i, ep := range progress.UnwatchedEpisodes {
		c.logger.WithFields(logrus.Fields{
			"index":   i,
			"season":  ep.Season,
			"episode": ep.Episode,
		}).Info("Unwatched episode from Trakt")
	}

	// Get the season of the first unwatched episode
	firstUnwatched := progress.UnwatchedEpisodes[0]
	season := firstUnwatched.Season

	// Count unwatched episodes in this season
	unwatchedInSeason := []trakt.Episode{}
	for _, ep := range progress.UnwatchedEpisodes {
		if ep.Season == season {
			unwatchedInSeason = append(unwatchedInSeason, ep)
		}
	}

	c.logger.WithFields(logrus.Fields{
		"media_id":               media.ID,
		"title":                  media.Title,
		"season":                 season,
		"unwatched_in_season":    len(unwatchedInSeason),
		"total_unwatched":        len(progress.UnwatchedEpisodes),
	}).Debug("Strategy: Season pack for favorites")

	// Return strategy to search for season pack
	// Search controller will also search for next 3 episodes and compare
	return &DownloadStrategy{
		Type:         StrategySeasonPack,
		Episodes:     unwatchedInSeason,
		SeasonNumber: &season,
	}, nil
}
