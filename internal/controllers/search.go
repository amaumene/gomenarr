package controllers

import (
	"context"
	"fmt"

	"github.com/amaumene/gomenarr/internal/models"
	"github.com/amaumene/gomenarr/internal/services/newznab"
	"github.com/amaumene/gomenarr/internal/services/trakt"
	"github.com/amaumene/gomenarr/internal/utils"
	"github.com/sirupsen/logrus"
)

// SearchController handles search operations
type SearchController struct {
	db            *models.Database
	newznabClient *newznab.Client
	traktClient   *trakt.Client
	blacklist     *utils.Blacklist
	logger        *logrus.Logger
}

// NewSearchController creates a new search controller
func NewSearchController(db *models.Database, newznabClient *newznab.Client, traktClient *trakt.Client, blacklist *utils.Blacklist, logger *logrus.Logger) *SearchController {
	return &SearchController{
		db:            db,
		newznabClient: newznabClient,
		traktClient:   traktClient,
		blacklist:     blacklist,
		logger:        logger,
	}
}

// SearchMedia searches for media based on strategy
func (c *SearchController) SearchMedia(ctx context.Context, media *models.Media, strategy *DownloadStrategy) ([]*models.NZB, error) {
	c.logger.WithFields(logrus.Fields{
		"media_id": media.ID,
		"title":    media.Title,
		"strategy": strategy.Type,
	}).Info("Starting media search")

	var allResults []newznab.SearchResult
	var err error

	switch strategy.Type {
	case StrategySingleMovie:
		allResults, err = c.newznabClient.SearchByIMDBID(media.IMDBId, "movie")
	case StrategySingleEpisode:
		if len(strategy.Episodes) == 0 {
			return nil, fmt.Errorf("no episodes in strategy")
		}
		ep := strategy.Episodes[0]
		allResults, err = c.newznabClient.SearchEpisode(media.IMDBId, ep.Season, ep.Episode)
	case StrategySeasonPack, StrategyNext3Episodes:
		// For favorites: search both season pack and individual episodes
		allResults, err = c.searchFavorites(ctx, media, strategy)
	}

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	c.logger.WithField("count", len(allResults)).Debug("Search results received")

	// Convert and process results
	nzbs := c.processResults(ctx, media, allResults)

	// Save all candidates to database
	for _, nzb := range nzbs {
		if err := c.db.CreateNZB(nzb); err != nil {
			c.logger.WithError(err).Error("Failed to save NZB to database")
		}
	}

	c.logger.WithField("candidates", len(nzbs)).Info("Search completed")
	return nzbs, nil
}

// searchFavorites searches for both season packs and individual episodes for favorites
func (c *SearchController) searchFavorites(ctx context.Context, media *models.Media, strategy *DownloadStrategy) ([]newznab.SearchResult, error) {
	var allResults []newznab.SearchResult

	// Search for season pack
	if strategy.SeasonNumber != nil {
		seasonResults, err := c.newznabClient.SearchSeason(media.IMDBId, *strategy.SeasonNumber)
		if err != nil {
			c.logger.WithError(err).Warn("Season pack search failed")
		} else {
			allResults = append(allResults, seasonResults...)
		}
	}

	// Search for next 3 individual episodes
	episodeCount := len(strategy.Episodes)
	if episodeCount > 3 {
		episodeCount = 3
	}

	c.logger.WithFields(logrus.Fields{
		"total_episodes":  len(strategy.Episodes),
		"searching_count": episodeCount,
	}).Info("Searching for individual episodes")

	for i := 0; i < episodeCount; i++ {
		ep := strategy.Episodes[i]
		c.logger.WithFields(logrus.Fields{
			"index":   i,
			"season":  ep.Season,
			"episode": ep.Episode,
		}).Info("Searching for episode")

		epResults, err := c.newznabClient.SearchEpisode(media.IMDBId, ep.Season, ep.Episode)
		if err != nil {
			c.logger.WithError(err).WithFields(logrus.Fields{
				"season":  ep.Season,
				"episode": ep.Episode,
			}).Warn("Episode search failed")
			continue
		}
		allResults = append(allResults, epResults...)
	}

	return allResults, nil
}

// processResults processes search results into NZB models
func (c *SearchController) processResults(ctx context.Context, media *models.Media, results []newznab.SearchResult) []*models.NZB {
	var nzbs []*models.NZB

	for _, result := range results {
		// Check blacklist
		if isBlacklisted, term := c.blacklist.IsBlacklisted(result.Title); isBlacklisted {
			c.logger.WithFields(logrus.Fields{
				"title": result.Title,
				"term":  term,
			}).Debug("NZB blacklisted")

			nzb := &models.NZB{
				MediaID:        media.ID,
				Title:          result.Title,
				Link:           result.Link,
				GUID:           result.GUID,
				Size:           result.Size,
				Quality:        utils.DetermineQuality(result.Title),
				Status:         models.NZBStatusBlacklisted,
				BlacklistMatch: term,
			}
			nzbs = append(nzbs, nzb)
			continue
		}

		// Determine quality
		quality := utils.DetermineQuality(result.Title)

		// Extract year from NZB title
		year := utils.ExtractYear(result.Title)

		// For movies, filter by year match
		if media.MediaType == models.MediaTypeMovie && year != 0 && media.Year != 0 {
			if year != media.Year {
				c.logger.WithFields(logrus.Fields{
					"title":      result.Title,
					"nzb_year":   year,
					"media_year": media.Year,
				}).Debug("Skipping movie NZB due to year mismatch")
				continue
			}
		}

		// DEBUG: Log NZB creation with link
		c.logger.WithFields(logrus.Fields{
			"title": result.Title,
			"link":  result.Link,
			"year":  year,
		}).Debug("Creating NZB from search result")

		nzb := &models.NZB{
			MediaID:      media.ID,
			Title:        result.Title,
			Link:         result.Link,
			GUID:         result.GUID,
			Size:         result.Size,
			Quality:      quality,
			Year:         year,
			Status:       models.NZBStatusCandidate,
			Season:       result.Season,
			Episode:      result.Episode,
			IsSeasonPack: result.IsSeasonPack,
		}

		// If season pack, populate episode list from Trakt
		if result.IsSeasonPack && result.Season != nil {
			episodes, err := c.populateSeasonPackEpisodes(ctx, media.IMDBId, *result.Season)
			if err != nil {
				c.logger.WithError(err).Warn("Failed to populate season pack episodes")
			} else {
				nzb.Episodes = episodes
			}
		}

		nzbs = append(nzbs, nzb)
	}

	// Rank by quality
	ranked := utils.RankByQuality(nzbs)

	// Selection logic:
	// 1. Season pack → select best season pack
	// 2. Individual episodes → select best for each episode
	// 3. Movies → select best movie

	hasSeasonPack := false
	hasEpisodes := false

	// Check if we have season packs
	for _, nzb := range ranked {
		if nzb.IsSeasonPack && nzb.Status == models.NZBStatusCandidate {
			hasSeasonPack = true
			nzb.Status = models.NZBStatusSelected
			c.logger.WithField("title", nzb.Title).Info("Selected season pack")
			break
		}
	}

	// If no season pack, select best quality for each episode OR best movie
	if !hasSeasonPack {
		selectedEpisodes := make(map[int]bool) // Track which episodes we've selected

		for _, nzb := range ranked {
			if nzb.Status != models.NZBStatusCandidate {
				continue
			}

			// Handle episodes
			if nzb.Episode != nil {
				hasEpisodes = true
				if selectedEpisodes[*nzb.Episode] {
					continue // Already selected this episode
				}
				nzb.Status = models.NZBStatusSelected
				selectedEpisodes[*nzb.Episode] = true
				c.logger.WithFields(logrus.Fields{
					"episode": *nzb.Episode,
					"title":   nzb.Title,
				}).Info("Selected individual episode")
			} else if !hasEpisodes {
				// This is a movie (no episode number) - select the first (best) one
				nzb.Status = models.NZBStatusSelected
				c.logger.WithField("title", nzb.Title).Info("Selected movie")
				break
			}
		}
	}

	return ranked
}

// populateSeasonPackEpisodes gets episode list from Trakt for a season pack
func (c *SearchController) populateSeasonPackEpisodes(ctx context.Context, imdbID string, season int) ([]models.EpisodeInfo, error) {
	seasonInfo, err := c.traktClient.GetSeasonInfo(ctx, imdbID, season)
	if err != nil {
		return nil, err
	}

	episodes := make([]models.EpisodeInfo, 0, len(seasonInfo.Episodes))
	for _, ep := range seasonInfo.Episodes {
		episodes = append(episodes, models.EpisodeInfo{
			EpisodeNumber: ep.Number,
			Watched:       false,
			WatchedAt:     nil,
		})
	}

	return episodes, nil
}
