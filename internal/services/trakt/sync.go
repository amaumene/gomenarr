package trakt

import (
	"context"
	"fmt"
	"time"
)

// TraktMedia represents a media item from Trakt API
type TraktMedia struct {
	Type  string // "movie" or "show"
	Movie *struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		IDs   struct {
			IMDB string `json:"imdb"` // e.g. "tt0133093"
		} `json:"ids"`
	} `json:"movie,omitempty"`
	Show *struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
		IDs   struct {
			IMDB string `json:"imdb"` // e.g. "tt0944947"
		} `json:"ids"`
	} `json:"show,omitempty"`
}

// GetFavorites retrieves favorites from Trakt
func (c *Client) GetFavorites(ctx context.Context, mediaType string) ([]TraktMedia, error) {
	path := fmt.Sprintf("/sync/favorites/%s", mediaType)

	var items []TraktMedia
	if err := c.doRequest(ctx, "GET", path, nil, &items); err != nil {
		return nil, fmt.Errorf("failed to get favorites: %w", err)
	}

	return items, nil
}

// GetWatchlist retrieves watchlist from Trakt
func (c *Client) GetWatchlist(ctx context.Context, mediaType string) ([]TraktMedia, error) {
	path := fmt.Sprintf("/sync/watchlist/%s", mediaType)

	var items []TraktMedia
	if err := c.doRequest(ctx, "GET", path, nil, &items); err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}

	return items, nil
}

// WatchedItem represents a watched item from Trakt history
type WatchedItem struct {
	IMDBId    string
	MediaType string // "movie" or "episode"
	Season    int    // for episodes
	Episode   int    // for episodes
	WatchedAt time.Time
}

// GetRecentlyWatched retrieves recently watched items from Trakt
func (c *Client) GetRecentlyWatched(ctx context.Context, days int) ([]WatchedItem, error) {
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	path := fmt.Sprintf("/sync/history?start_at=%s", startDate)

	var historyItems []struct {
		ID        int64     `json:"id"`
		WatchedAt time.Time `json:"watched_at"`
		Action    string    `json:"action"`
		Type      string    `json:"type"`
		Movie     *struct {
			IDs struct {
				IMDB string `json:"imdb"`
			} `json:"ids"`
		} `json:"movie,omitempty"`
		Episode *struct {
			Season int `json:"season"`
			Number int `json:"number"`
		} `json:"episode,omitempty"`
		Show *struct {
			IDs struct {
				IMDB string `json:"imdb"`
			} `json:"ids"`
		} `json:"show,omitempty"`
	}

	if err := c.doRequest(ctx, "GET", path, nil, &historyItems); err != nil {
		return nil, fmt.Errorf("failed to get watched history: %w", err)
	}

	var items []WatchedItem
	for _, item := range historyItems {
		if item.Type == "movie" && item.Movie != nil {
			items = append(items, WatchedItem{
				IMDBId:    item.Movie.IDs.IMDB,
				MediaType: "movie",
				WatchedAt: item.WatchedAt,
			})
		} else if item.Type == "episode" && item.Episode != nil && item.Show != nil {
			items = append(items, WatchedItem{
				IMDBId:    item.Show.IDs.IMDB,
				MediaType: "episode",
				Season:    item.Episode.Season,
				Episode:   item.Episode.Number,
				WatchedAt: item.WatchedAt,
			})
		}
	}

	return items, nil
}

// SeasonInfo represents season information from Trakt
type SeasonInfo struct {
	Number   int
	Episodes []EpisodeBasicInfo
}

// EpisodeBasicInfo represents basic episode information
type EpisodeBasicInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// GetSeasonInfo retrieves information about a specific season
func (c *Client) GetSeasonInfo(ctx context.Context, imdbID string, season int) (*SeasonInfo, error) {
	// First, look up the Trakt ID from the IMDB ID
	traktID, err := c.lookupTraktIDFromIMDB(ctx, imdbID)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/shows/%d/seasons/%d?extended=episodes", traktID, season)

	var episodes []EpisodeBasicInfo
	if err := c.doRequest(ctx, "GET", path, nil, &episodes); err != nil {
		return nil, fmt.Errorf("failed to get season info: %w", err)
	}

	return &SeasonInfo{
		Number:   season,
		Episodes: episodes,
	}, nil
}

// Episode represents an episode reference
type Episode struct {
	Season  int
	Episode int
}

// ShowProgress represents the watch progress for a TV show
type ShowProgress struct {
	NextEpisode       *Episode
	UnwatchedEpisodes []Episode
}

// lookupTraktIDFromIMDB looks up the Trakt ID for a show using its IMDB ID
func (c *Client) lookupTraktIDFromIMDB(ctx context.Context, imdbID string) (int, error) {
	path := fmt.Sprintf("/search/imdb/%s?type=show", imdbID)

	var results []struct {
		Type string `json:"type"`
		Show *struct {
			IDs struct {
				Trakt int `json:"trakt"`
			} `json:"ids"`
		} `json:"show"`
	}

	if err := c.doRequest(ctx, "GET", path, nil, &results); err != nil {
		return 0, fmt.Errorf("failed to lookup Trakt ID: %w", err)
	}

	if len(results) == 0 || results[0].Show == nil {
		return 0, fmt.Errorf("show not found in Trakt for IMDB ID %s", imdbID)
	}

	return results[0].Show.IDs.Trakt, nil
}

// GetShowProgress retrieves the watch progress for a TV show
func (c *Client) GetShowProgress(ctx context.Context, imdbID string) (*ShowProgress, error) {
	// First, look up the Trakt ID from the IMDB ID
	traktID, err := c.lookupTraktIDFromIMDB(ctx, imdbID)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/shows/%d/progress/watched", traktID)

	var progress struct {
		NextEpisode *struct {
			Season int `json:"season"`
			Number int `json:"number"`
		} `json:"next_episode"`
		Seasons []struct {
			Number   int `json:"number"`
			Episodes []struct {
				Number    int  `json:"number"`
				Completed bool `json:"completed"`
			} `json:"episodes"`
		} `json:"seasons"`
	}

	if err := c.doRequest(ctx, "GET", path, nil, &progress); err != nil {
		return nil, fmt.Errorf("failed to get show progress: %w", err)
	}

	result := &ShowProgress{
		UnwatchedEpisodes: []Episode{},
	}

	// Set next episode
	if progress.NextEpisode != nil {
		result.NextEpisode = &Episode{
			Season:  progress.NextEpisode.Season,
			Episode: progress.NextEpisode.Number,
		}
	}

	// Collect unwatched episodes
	for _, season := range progress.Seasons {
		for _, ep := range season.Episodes {
			if !ep.Completed {
				result.UnwatchedEpisodes = append(result.UnwatchedEpisodes, Episode{
					Season:  season.Number,
					Episode: ep.Number,
				})
			}
		}
	}

	return result, nil
}
