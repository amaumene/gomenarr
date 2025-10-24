package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

const baseURL = "https://api.trakt.tv"

// Sentinel errors for OAuth device flow polling
var (
	ErrPending     = errors.New("authorization pending - waiting for user approval")
	ErrExpired     = errors.New("device code expired")
	ErrDenied      = errors.New("user denied authorization")
	ErrNotFound    = errors.New("invalid device code")
	ErrAlreadyUsed = errors.New("code already used")
)

type Client struct {
	cfg                config.TraktConfig
	httpClient         *http.Client
	token              *Token
	tokenFile          string
	showIMDBCache      sync.Map          // Cache for show Trakt ID -> IMDB ID mapping
	watchedMovieCache  map[int64]bool    // Cache for watched movie Trakt IDs
	watchedEpisodeCache map[string]bool   // Cache for watched episodes (key: "imdb:season:episode")
	watchedCacheMu     sync.RWMutex
}

func NewClient(cfg config.TraktConfig, dataDir string) *Client {
	// Configure HTTP transport with connection pooling for better performance
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
		tokenFile: dataDir + "/token.json",
	}
}

func (c *Client) Authenticate(ctx context.Context) error {
	// Request device code
	log.Info().Msg("Starting Trakt authentication flow")
	reqBody := map[string]string{"client_id": c.cfg.ClientID}

	resp, err := c.post(ctx, "/oauth/device/code", reqBody, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to request device code from Trakt")
		return fmt.Errorf("failed to request device code: %w", err)
	}

	// Ensure response body is closed
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(bodyBytes)).
			Msg("Unexpected status code from device code request")
		return fmt.Errorf("device code request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var dcr DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		log.Error().Err(err).Msg("Failed to decode device code response")
		return fmt.Errorf("failed to decode device code response: %w", err)
	}

	log.Info().
		Str("user_code", dcr.UserCode).
		Str("verification_url", dcr.VerificationURL).
		Int("expires_in", dcr.ExpiresIn).
		Msg("Device code received successfully")

	log.Info().
		Str("url", dcr.VerificationURL).
		Str("code", dcr.UserCode).
		Msg("Please visit the URL and enter the code to authenticate")

	fmt.Printf("\n=== Trakt Authentication Required ===\n")
	fmt.Printf("1. Go to: %s\n", dcr.VerificationURL)
	fmt.Printf("2. Enter code: %s\n", dcr.UserCode)
	fmt.Printf("3. This code expires in %d seconds\n", dcr.ExpiresIn)
	fmt.Printf("4. Waiting for authorization...\n\n")

	// Poll for token
	ticker := time.NewTicker(time.Duration(dcr.Interval) * time.Second)
	defer ticker.Stop()

	timeout := time.After(time.Duration(dcr.ExpiresIn) * time.Second)

	log.Info().
		Int("interval_seconds", dcr.Interval).
		Msg("Waiting for authentication... (polling)")

	pollCount := 0
	for {
		select {
		case <-ctx.Done():
			log.Error().Err(ctx.Err()).Msg("Authentication cancelled")
			return ctx.Err()
		case <-timeout:
			log.Error().Msg("Authentication timeout - code expired")
			return fmt.Errorf("authentication timeout")
		case <-ticker.C:
			pollCount++
			log.Debug().Int("poll_count", pollCount).Msg("Polling for token...")

			token, err := c.pollToken(ctx, dcr.DeviceCode)
			if err == nil {
				// Successfully obtained token
				c.token = token
				if err := c.saveToken(); err != nil {
					log.Error().Err(err).Msg("Failed to save token")
					return err
				}
				log.Info().Msg("Authentication successful! Token saved")
				fmt.Println("\nAuthentication successful! Token saved.")
				return nil
			}

			// Check if this is a "pending" error (continue polling)
			if errors.Is(err, ErrPending) {
				// This is expected - user hasn't approved yet
				// Continue polling silently (already logged at debug level in pollToken)
				continue
			}

			// Any other error is fatal - stop polling
			log.Error().
				Err(err).
				Msg("Fatal error during authentication polling")
			return fmt.Errorf("authentication failed: %w", err)
		}
	}
}

func (c *Client) pollToken(ctx context.Context, deviceCode string) (*Token, error) {
	reqBody := map[string]string{
		"client_id":     c.cfg.ClientID,
		"client_secret": c.cfg.ClientSecret,
		"code":          deviceCode,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/oauth/device/token", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.cfg.ClientID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for all cases
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle different status codes according to Trakt API documentation
	switch resp.StatusCode {
	case http.StatusOK: // 200 - Success
		var token Token
		if err := json.Unmarshal(bodyBytes, &token); err != nil {
			return nil, fmt.Errorf("failed to decode token: %w", err)
		}
		// If Trakt didn't provide created_at, set it to now
		if token.CreatedAt == 0 {
			token.CreatedAt = time.Now().Unix()
		}
		log.Info().Msg("Successfully obtained access token")
		return &token, nil

	case http.StatusBadRequest: // 400 - Pending (user hasn't approved yet)
		log.Debug().Msg("Still waiting for user approval...")
		return nil, ErrPending

	case http.StatusNotFound: // 404 - Invalid device code
		log.Error().
			Str("device_code", deviceCode).
			Str("response", string(bodyBytes)).
			Msg("Invalid device code")
		return nil, ErrNotFound

	case http.StatusConflict: // 409 - Already used
		// According to Trakt docs, this means the code was already used.
		// The token might be in the response body, so try to decode it.
		log.Info().Msg("Code already used - attempting to retrieve token from response")

		var token Token
		if err := json.Unmarshal(bodyBytes, &token); err != nil {
			// Token not in response, this is an error state
			log.Error().
				Str("response", string(bodyBytes)).
				Err(err).
				Msg("Code already used but token not in response")
			return nil, fmt.Errorf("%w: %s", ErrAlreadyUsed, string(bodyBytes))
		}

		// Successfully decoded token from 409 response
		// If Trakt didn't provide created_at, set it to now
		if token.CreatedAt == 0 {
			token.CreatedAt = time.Now().Unix()
		}
		log.Info().Msg("Successfully retrieved token from 409 response")
		return &token, nil

	case http.StatusGone: // 410 - Expired
		log.Error().Msg("Device code has expired")
		return nil, ErrExpired

	case 418: // 418 - User denied
		log.Error().Msg("User explicitly denied authorization")
		return nil, ErrDenied

	default:
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("response", string(bodyBytes)).
			Msg("Unexpected status code from token endpoint")
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}
}

func (c *Client) IsAuthenticated() bool {
	if c.token == nil {
		if err := c.loadToken(); err != nil {
			return false
		}
	}
	return c.token != nil && !c.token.IsExpired()
}

func (c *Client) RefreshToken(ctx context.Context) error {
	if c.token == nil {
		return fmt.Errorf("no token to refresh")
	}

	if !c.token.IsExpired() {
		return nil
	}

	reqBody := map[string]string{
		"client_id":     c.cfg.ClientID,
		"client_secret": c.cfg.ClientSecret,
		"refresh_token": c.token.RefreshToken,
		"grant_type":    "refresh_token",
	}

	resp, err := c.post(ctx, "/oauth/token", reqBody, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return err
	}
	// If Trakt didn't provide created_at, set it to now
	if token.CreatedAt == 0 {
		token.CreatedAt = time.Now().Unix()
	}

	c.token = &token
	return c.saveToken()
}

func (c *Client) GetWatchlistMovies(ctx context.Context) ([]ports.TraktMovie, error) {
	var result []struct {
		Movie struct {
			IDs struct {
				Trakt int64  `json:"trakt"`
				IMDB  string `json:"imdb"`
			} `json:"ids"`
			Title string `json:"title"`
			Year  int64  `json:"year"`
		} `json:"movie"`
	}

	if err := c.get(ctx, "/sync/watchlist/movies", &result); err != nil {
		return nil, err
	}

	movies := make([]ports.TraktMovie, 0, len(result))
	for _, item := range result {
		if item.Movie.IDs.Trakt > 0 && item.Movie.IDs.IMDB != "" {
			movies = append(movies, ports.TraktMovie{
				TraktID: item.Movie.IDs.Trakt,
				IMDB:    item.Movie.IDs.IMDB,
				Title:   item.Movie.Title,
				Year:    item.Movie.Year,
			})
		}
	}

	return movies, nil
}

func (c *Client) GetFavoriteMovies(ctx context.Context) ([]ports.TraktMovie, error) {
	var result []struct {
		Movie struct {
			IDs struct {
				Trakt int64  `json:"trakt"`
				IMDB  string `json:"imdb"`
			} `json:"ids"`
			Title string `json:"title"`
			Year  int64  `json:"year"`
		} `json:"movie"`
	}

	if err := c.get(ctx, "/sync/favorites/movies", &result); err != nil {
		return nil, err
	}

	movies := make([]ports.TraktMovie, 0, len(result))
	for _, item := range result {
		if item.Movie.IDs.Trakt > 0 && item.Movie.IDs.IMDB != "" {
			movies = append(movies, ports.TraktMovie{
				TraktID: item.Movie.IDs.Trakt,
				IMDB:    item.Movie.IDs.IMDB,
				Title:   item.Movie.Title,
				Year:    item.Movie.Year,
			})
		}
	}

	return movies, nil
}

func (c *Client) GetWatchlistShows(ctx context.Context) ([]ports.TraktShow, error) {
	var result []struct {
		Show struct {
			IDs struct {
				Trakt int64  `json:"trakt"`
				IMDB  string `json:"imdb"`
			} `json:"ids"`
			Title string `json:"title"`
			Year  int64  `json:"year"`
		} `json:"show"`
	}

	if err := c.get(ctx, "/sync/watchlist/shows", &result); err != nil {
		return nil, err
	}

	shows := make([]ports.TraktShow, 0, len(result))
	filteredCount := 0
	for _, item := range result {
		if item.Show.IDs.Trakt > 0 && item.Show.IDs.IMDB != "" {
			shows = append(shows, ports.TraktShow{
				TraktID: item.Show.IDs.Trakt,
				IMDB:    item.Show.IDs.IMDB,
				Title:   item.Show.Title,
				Year:    item.Show.Year,
			})
		} else if item.Show.IDs.IMDB == "" {
			log.Warn().
				Int64("trakt_id", item.Show.IDs.Trakt).
				Str("title", item.Show.Title).
				Msg("Skipping watchlist show: No IMDB ID")
			filteredCount++
		}
	}

	if filteredCount > 0 {
		log.Info().
			Int("total", len(result)).
			Int("with_imdb", len(shows)).
			Int("without_imdb", filteredCount).
			Msg("Filtered watchlist shows without IMDB IDs")
	}

	return shows, nil
}

func (c *Client) GetFavoriteShows(ctx context.Context) ([]ports.TraktShow, error) {
	var result []struct {
		Show struct {
			IDs struct {
				Trakt int64  `json:"trakt"`
				IMDB  string `json:"imdb"`
			} `json:"ids"`
			Title string `json:"title"`
			Year  int64  `json:"year"`
		} `json:"show"`
	}

	if err := c.get(ctx, "/sync/favorites/shows", &result); err != nil {
		return nil, err
	}

	shows := make([]ports.TraktShow, 0, len(result))
	filteredCount := 0
	for _, item := range result {
		if item.Show.IDs.Trakt > 0 && item.Show.IDs.IMDB != "" {
			shows = append(shows, ports.TraktShow{
				TraktID: item.Show.IDs.Trakt,
				IMDB:    item.Show.IDs.IMDB,
				Title:   item.Show.Title,
				Year:    item.Show.Year,
			})
		} else if item.Show.IDs.IMDB == "" {
			log.Warn().
				Int64("trakt_id", item.Show.IDs.Trakt).
				Str("title", item.Show.Title).
				Msg("Skipping favorite show: No IMDB ID")
			filteredCount++
		}
	}

	if filteredCount > 0 {
		log.Info().
			Int("total", len(result)).
			Int("with_imdb", len(shows)).
			Int("without_imdb", filteredCount).
			Msg("Filtered favorite shows without IMDB IDs")
	}

	return shows, nil
}

func (c *Client) GetNextEpisode(ctx context.Context, showTraktID int64) (*ports.TraktEpisode, error) {
	episodes, err := c.GetNextNEpisodes(ctx, showTraktID, 1)
	if err != nil {
		return nil, err
	}
	if len(episodes) == 0 {
		return nil, nil
	}
	return &episodes[0], nil
}

func (c *Client) GetNextNEpisodes(ctx context.Context, showTraktID int64, limit int) ([]ports.TraktEpisode, error) {
	var progress struct {
		Show struct {
			IDs struct {
				IMDB string `json:"imdb"`
			} `json:"ids"`
		} `json:"show"`
		NextEpisode *struct {
			Season int64 `json:"season"`
			Number int64 `json:"number"`
			IDs    struct {
				Trakt int64 `json:"trakt"`
			} `json:"ids"`
			Title string `json:"title"`
		} `json:"next_episode"`
	}

	url := fmt.Sprintf("/shows/%d/progress/watched", showTraktID)
	if err := c.get(ctx, url, &progress); err != nil {
		return nil, err
	}

	if progress.NextEpisode == nil {
		log.Info().
			Int64("show_trakt_id", showTraktID).
			Str("show_imdb", progress.Show.IDs.IMDB).
			Msg("No next episode found - show may be fully watched or no episodes available")
		return []ports.TraktEpisode{}, nil
	}

	log.Debug().
		Int64("show_trakt_id", showTraktID).
		Str("show_imdb_from_progress", progress.Show.IDs.IMDB).
		Int64("next_season", progress.NextEpisode.Season).
		Int64("next_episode", progress.NextEpisode.Number).
		Int64("next_trakt_id", progress.NextEpisode.IDs.Trakt).
		Str("next_title", progress.NextEpisode.Title).
		Msg("Found next episode from Trakt progress")

	// If IMDB ID is missing from progress endpoint, try cache first, then fetch from API
	showIMDB := progress.Show.IDs.IMDB
	if showIMDB == "" {
		// Check cache first
		if cached, ok := c.showIMDBCache.Load(showTraktID); ok {
			showIMDB = cached.(string)
			log.Debug().
				Int64("show_trakt_id", showTraktID).
				Str("show_imdb", showIMDB).
				Msg("Retrieved show IMDB ID from cache")
		} else {
			// Not in cache, fetch from API
			log.Warn().
				Int64("show_trakt_id", showTraktID).
				Msg("Show IMDB ID missing from progress endpoint and cache, fetching from show details")

			var showDetails struct {
				IDs struct {
					IMDB string `json:"imdb"`
				} `json:"ids"`
				Title string `json:"title"`
			}

			showURL := fmt.Sprintf("/shows/%d", showTraktID)
			if err := c.get(ctx, showURL, &showDetails); err != nil {
				log.Error().
					Err(err).
					Int64("show_trakt_id", showTraktID).
					Msg("Failed to fetch show details from Trakt")
				return nil, fmt.Errorf("failed to fetch show details: %w", err)
			}

			showIMDB = showDetails.IDs.IMDB

			log.Info().
				Int64("show_trakt_id", showTraktID).
				Str("show_title", showDetails.Title).
				Str("show_imdb", showIMDB).
				Msg("Fetched show IMDB ID from show details endpoint")

			if showIMDB == "" {
				log.Error().
					Int64("show_trakt_id", showTraktID).
					Str("show_title", showDetails.Title).
					Msg("Show has no IMDB ID in Trakt database - cannot search for NZBs")
				return nil, fmt.Errorf("show %d has no IMDB ID", showTraktID)
			}

			// Store in cache for future use
			c.showIMDBCache.Store(showTraktID, showIMDB)
			log.Debug().
				Int64("show_trakt_id", showTraktID).
				Str("show_imdb", showIMDB).
				Msg("Stored show IMDB ID in cache")
		}
	} else {
		// IMDB was in progress response, store it in cache for future use
		c.showIMDBCache.Store(showTraktID, showIMDB)
	}

	episodes := make([]ports.TraktEpisode, 0, limit)

	// Get the first episode
	episodes = append(episodes, ports.TraktEpisode{
		TraktID:  progress.NextEpisode.IDs.Trakt,
		Season:   progress.NextEpisode.Season,
		Number:   progress.NextEpisode.Number,
		Title:    progress.NextEpisode.Title,
		ShowIMDB: showIMDB,
	})

	log.Debug().
		Int64("episode_trakt_id", progress.NextEpisode.IDs.Trakt).
		Str("episode_title", progress.NextEpisode.Title).
		Int64("season", progress.NextEpisode.Season).
		Int64("episode", progress.NextEpisode.Number).
		Str("show_imdb", showIMDB).
		Msg("Added first episode")

	// Get additional episodes if requested
	for i := 1; i < limit; i++ {
		nextEp, err := c.getEpisode(ctx, showTraktID, progress.NextEpisode.Season, progress.NextEpisode.Number+int64(i))
		if err != nil {
			log.Debug().
				Err(err).
				Int64("show_trakt_id", showTraktID).
				Int64("season", progress.NextEpisode.Season).
				Int64("episode", progress.NextEpisode.Number+int64(i)).
				Msg("Could not fetch additional episode")
			break
		}
		nextEp.ShowIMDB = showIMDB
		episodes = append(episodes, *nextEp)

		log.Debug().
			Int64("episode_trakt_id", nextEp.TraktID).
			Str("episode_title", nextEp.Title).
			Int64("season", nextEp.Season).
			Int64("episode", nextEp.Number).
			Str("show_imdb", showIMDB).
			Msg("Added additional episode")
	}

	log.Info().
		Int64("show_trakt_id", showTraktID).
		Str("show_imdb", showIMDB).
		Int("episode_count", len(episodes)).
		Msg("Successfully fetched episodes with show IMDB ID")

	return episodes, nil
}

func (c *Client) getEpisode(ctx context.Context, showTraktID, season, episode int64) (*ports.TraktEpisode, error) {
	var ep struct {
		IDs struct {
			Trakt int64 `json:"trakt"`
		} `json:"ids"`
		Title string `json:"title"`
	}

	url := fmt.Sprintf("/shows/%d/seasons/%d/episodes/%d", showTraktID, season, episode)
	if err := c.get(ctx, url, &ep); err != nil {
		return nil, err
	}

	return &ports.TraktEpisode{
		TraktID: ep.IDs.Trakt,
		Season:  season,
		Number:  episode,
		Title:   ep.Title,
	}, nil
}

func (c *Client) GetWatchHistory(ctx context.Context, days int) ([]ports.TraktHistoryItem, error) {
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	url := fmt.Sprintf("/sync/history?start_at=%s", startDate)

	var result []struct {
		WatchedAt time.Time `json:"watched_at"`
		Type      string    `json:"type"`
		Movie     *struct {
			IDs struct {
				Trakt int64 `json:"trakt"`
			} `json:"ids"`
		} `json:"movie"`
		Episode *struct {
			IDs struct {
				Trakt int64 `json:"trakt"`
			} `json:"ids"`
		} `json:"episode"`
	}

	if err := c.get(ctx, url, &result); err != nil {
		return nil, err
	}

	items := make([]ports.TraktHistoryItem, 0, len(result))
	for _, item := range result {
		var traktID int64
		if item.Movie != nil {
			traktID = item.Movie.IDs.Trakt
		} else if item.Episode != nil {
			traktID = item.Episode.IDs.Trakt
		}

		if traktID > 0 {
			items = append(items, ports.TraktHistoryItem{
				TraktID:   traktID,
				WatchedAt: item.WatchedAt,
				Type:      item.Type,
			})
		}
	}

	return items, nil
}

func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	if err := c.ensureToken(); err != nil {
		return err
	}

	fullURL := baseURL + path

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return err
	}

	c.setHeaders(req)

	log.Debug().
		Str("url", fullURL).
		Str("method", "GET").
		Msg("Making Trakt API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", fullURL).Msg("Trakt API request failed")
		return err
	}
	defer resp.Body.Close()

	// Read response body for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Str("url", fullURL).Msg("Failed to read Trakt response body")
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response preview
	bodyPreview := string(bodyBytes)
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500] + "..."
	}

	log.Debug().
		Int("status_code", resp.StatusCode).
		Str("url", fullURL).
		Int("body_length", len(bodyBytes)).
		Str("body_preview", bodyPreview).
		Msg("Received Trakt API response")

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("url", fullURL).
			Str("response", string(bodyBytes)).
			Msg("Trakt API error")
		return fmt.Errorf("trakt API error: %d %s", resp.StatusCode, string(bodyBytes))
	}

	// Unmarshal from the bytes we read
	if err := json.Unmarshal(bodyBytes, result); err != nil {
		log.Error().
			Err(err).
			Str("url", fullURL).
			Str("response_body", string(bodyBytes)).
			Msg("Failed to unmarshal Trakt response")
		return fmt.Errorf("JSON unmarshal error: %w", err)
	}

	return nil
}

func (c *Client) post(ctx context.Context, path string, body interface{}, auth bool) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal request body")
		return nil, err
	}

	fullURL := baseURL + path

	// Debug logging: Request details
	log.Debug().
		Str("url", fullURL).
		Str("method", "POST").
		RawJSON("body", data).
		Msg("Preparing HTTP request")

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(data))
	if err != nil {
		log.Error().Err(err).Str("url", fullURL).Msg("Failed to create HTTP request")
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.cfg.ClientID)

	if auth && c.token != nil {
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	}

	// Debug logging: Request headers
	log.Debug().
		Str("Content-Type", req.Header.Get("Content-Type")).
		Str("trakt-api-version", req.Header.Get("trakt-api-version")).
		Str("trakt-api-key", req.Header.Get("trakt-api-key")).
		Bool("has_auth", auth && c.token != nil).
		Msg("Request headers set")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", fullURL).
			Msg("HTTP request failed")
		return nil, err
	}

	// Check if response is nil
	if resp == nil {
		log.Error().Str("url", fullURL).Msg("HTTP response is nil")
		return nil, fmt.Errorf("nil response from HTTP request")
	}

	// Debug logging: Response status
	log.Debug().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Str("url", fullURL).
		Msg("Received HTTP response")

	// Check status code and read body for non-2xx responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "..."
		}

		log.Error().
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Str("url", fullURL).
			Str("response_body", bodyPreview).
			Err(readErr).
			Msg("HTTP request returned non-2xx status")

		if readErr != nil {
			return nil, fmt.Errorf("HTTP %d %s (failed to read response body: %w)", resp.StatusCode, resp.Status, readErr)
		}

		return nil, fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, resp.Status, bodyPreview)
	}

	// Debug logging: Response body preview for successful requests
	if resp.Body != nil {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Error().Err(readErr).Msg("Failed to read response body")
			resp.Body.Close()
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "..."
		}

		log.Debug().
			Str("response_body_preview", bodyPreview).
			Int("body_length", len(bodyBytes)).
			Msg("Response body received")

		// Recreate the response body so it can be read again
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return resp, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.cfg.ClientID)
	if c.token != nil {
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	}
}

func (c *Client) ensureToken() error {
	if c.token == nil {
		if err := c.loadToken(); err != nil {
			return fmt.Errorf("not authenticated")
		}
	}

	if c.token.IsExpired() {
		if err := c.RefreshToken(context.Background()); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) saveToken() error {
	data, err := json.MarshalIndent(c.token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.tokenFile, data, 0600)
}

func (c *Client) loadToken() error {
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return err
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return err
	}

	c.token = &token
	log.Info().Msg("Loaded Trakt token from file")
	return nil
}

// IsWatched checks if a media item is in the watched history
// For movies: uses traktID, for episodes: uses imdb:season:episode composite key
func (c *Client) IsWatched(ctx context.Context, traktID int64, mediaType string, imdb string, season, episode int64) (bool, error) {
	c.watchedCacheMu.RLock()

	if mediaType == "movie" {
		// Check movie cache
		if c.watchedMovieCache != nil {
			watched, exists := c.watchedMovieCache[traktID]
			c.watchedCacheMu.RUnlock()
			if exists {
				return watched, nil
			}
			return false, nil
		}
		c.watchedCacheMu.RUnlock()

		// Cache not populated, fetch from Trakt API
		if err := c.fetchWatchedHistory(ctx, mediaType); err != nil {
			return false, fmt.Errorf("failed to fetch watched history: %w", err)
		}

		// Check again after fetching
		c.watchedCacheMu.RLock()
		watched := c.watchedMovieCache[traktID]
		c.watchedCacheMu.RUnlock()

		return watched, nil
	}

	// Episode check - use composite key
	if c.watchedEpisodeCache != nil {
		episodeKey := fmt.Sprintf("%s:%d:%d", imdb, season, episode)
		watched, exists := c.watchedEpisodeCache[episodeKey]
		c.watchedCacheMu.RUnlock()
		if exists {
			return watched, nil
		}
		return false, nil
	}
	c.watchedCacheMu.RUnlock()

	// Cache not populated, fetch from Trakt API
	if err := c.fetchWatchedHistory(ctx, mediaType); err != nil {
		return false, fmt.Errorf("failed to fetch watched history: %w", err)
	}

	// Check again after fetching
	c.watchedCacheMu.RLock()
	episodeKey := fmt.Sprintf("%s:%d:%d", imdb, season, episode)
	watched := c.watchedEpisodeCache[episodeKey]
	c.watchedCacheMu.RUnlock()

	return watched, nil
}

// fetchWatchedHistory fetches the watched history from Trakt and populates the cache
func (c *Client) fetchWatchedHistory(ctx context.Context, mediaType string) error {
	c.watchedCacheMu.Lock()
	defer c.watchedCacheMu.Unlock()

	if mediaType == "episode" || mediaType == "show" {
		// Double-check cache wasn't populated by another goroutine
		if c.watchedEpisodeCache != nil {
			return nil
		}

		// Initialize episode cache
		c.watchedEpisodeCache = make(map[string]bool)

		// Parse show watched history
		var shows []struct {
			Show struct {
				IDs struct {
					Trakt int64  `json:"trakt"`
					IMDB  string `json:"imdb"`
				} `json:"ids"`
				Title string `json:"title"`
			} `json:"show"`
			Seasons []struct {
				Number   int `json:"number"`
				Episodes []struct {
					Number int `json:"number"`
				} `json:"episodes"`
			} `json:"seasons"`
		}

		endpoint := "/sync/watched/shows"
		if err := c.get(ctx, endpoint, &shows); err != nil {
			return fmt.Errorf("failed to get watched shows: %w", err)
		}

		// Mark all watched episodes using composite key (imdb:season:episode)
		episodeCount := 0
		for _, show := range shows {
			showIMDB := show.Show.IDs.IMDB
			if showIMDB == "" {
				log.Warn().
					Int64("show_trakt_id", show.Show.IDs.Trakt).
					Str("show_title", show.Show.Title).
					Msg("Show missing IMDB ID in watched history, skipping")
				continue
			}

			for _, season := range show.Seasons {
				for _, episode := range season.Episodes {
					episodeKey := fmt.Sprintf("%s:%d:%d", showIMDB, season.Number, episode.Number)
					c.watchedEpisodeCache[episodeKey] = true
					episodeCount++
				}
			}
		}

		log.Info().
			Int("show_count", len(shows)).
			Int("episode_count", episodeCount).
			Msg("Cached watched episodes")
	} else {
		// Double-check cache wasn't populated by another goroutine
		if c.watchedMovieCache != nil {
			return nil
		}

		// Initialize movie cache
		c.watchedMovieCache = make(map[int64]bool)

		// Parse movie watched history
		var movies []struct {
			Movie struct {
				IDs struct {
					Trakt int64 `json:"trakt"`
				} `json:"ids"`
			} `json:"movie"`
		}

		endpoint := "/sync/watched/movies"
		if err := c.get(ctx, endpoint, &movies); err != nil {
			return fmt.Errorf("failed to get watched movies: %w", err)
		}

		// Mark all watched movies
		for _, movie := range movies {
			c.watchedMovieCache[movie.Movie.IDs.Trakt] = true
		}

		log.Info().Int("count", len(movies)).Msg("Cached watched movies")
	}

	return nil
}

// ClearWatchedCache clears the watched cache, forcing a refresh on next check
func (c *Client) ClearWatchedCache() {
	c.watchedCacheMu.Lock()
	c.watchedMovieCache = nil
	c.watchedEpisodeCache = nil
	c.watchedCacheMu.Unlock()
	log.Debug().Msg("Cleared watched cache")
}
