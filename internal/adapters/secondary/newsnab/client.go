package newsnab

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type Client struct {
	cfg        config.NewsnabConfig
	httpClient *http.Client
}

func NewClient(cfg config.NewsnabConfig) *Client {
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
	}
}

// stripIMDBPrefix removes the "tt" prefix from IMDB IDs
// Newsnab expects IMDB IDs WITHOUT the "tt" prefix for MOVIES (e.g., "1234567" instead of "tt1234567")
// but WITH the "tt" prefix for TV SHOWS (e.g., "tt1234567")
// This function should only be used for movie searches.
func stripIMDBPrefix(imdb string) string {
	return strings.TrimPrefix(imdb, "tt")
}

func (c *Client) SearchMovie(ctx context.Context, imdb string) ([]ports.NewsnabResult, error) {
	params := url.Values{}
	params.Set("t", "movie")
	params.Set("apikey", c.cfg.APIKey)
	params.Set("imdbid", stripIMDBPrefix(imdb))
	// Only set limit if MaxResults > 0 (0 means no limit)
	if c.cfg.MaxResults > 0 {
		params.Set("limit", fmt.Sprintf("%d", c.cfg.MaxResults))
	}

	return c.search(ctx, params)
}

func (c *Client) SearchEpisode(ctx context.Context, imdb string, season, episode int64) ([]ports.NewsnabResult, error) {
	params := url.Values{}
	params.Set("t", "tvsearch")
	params.Set("apikey", c.cfg.APIKey)
	params.Set("imdbid", imdb) // Keep "tt" prefix for TV searches
	params.Set("season", fmt.Sprintf("%d", season))
	params.Set("ep", fmt.Sprintf("%d", episode))
	// Only set limit if MaxResults > 0 (0 means no limit)
	if c.cfg.MaxResults > 0 {
		params.Set("limit", fmt.Sprintf("%d", c.cfg.MaxResults))
	}

	return c.search(ctx, params)
}

func (c *Client) SearchSeasonPack(ctx context.Context, imdb string, season int64) ([]ports.NewsnabResult, error) {
	params := url.Values{}
	params.Set("t", "tvsearch")
	params.Set("apikey", c.cfg.APIKey)
	params.Set("imdbid", imdb) // Keep "tt" prefix for TV searches
	params.Set("season", fmt.Sprintf("%d", season))
	// Only set limit if MaxResults > 0 (0 means no limit)
	if c.cfg.MaxResults > 0 {
		params.Set("limit", fmt.Sprintf("%d", c.cfg.MaxResults))
	}

	return c.search(ctx, params)
}

func (c *Client) search(ctx context.Context, params url.Values) ([]ports.NewsnabResult, error) {
	// Validate that base URL is configured
	if c.cfg.URL == "" {
		return nil, fmt.Errorf("newsnab URL is not configured")
	}

	// Construct absolute URL by combining base URL with /api endpoint
	searchURL := fmt.Sprintf("%s/api?%s", c.cfg.URL, params.Encode())

	log.Debug().
		Str("url", searchURL).
		Str("base_url", c.cfg.URL).
		Str("search_type", params.Get("t")).
		Str("imdbid", params.Get("imdbid")).
		Str("season", params.Get("season")).
		Str("episode", params.Get("ep")).
		Msg("Searching Newsnab indexer")

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", searchURL).Msg("Newsnab request failed")
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body first for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Str("url", searchURL).Msg("Failed to read Newsnab response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response status and body preview
	bodyPreview := string(bodyBytes)
	if len(bodyPreview) > 1000 {
		bodyPreview = bodyPreview[:1000] + "..."
	}

	log.Debug().
		Int("status_code", resp.StatusCode).
		Str("content_type", resp.Header.Get("Content-Type")).
		Int("body_length", len(bodyBytes)).
		Str("body_preview", bodyPreview).
		Msg("Received Newsnab response")

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("url", searchURL).
			Str("response", string(bodyBytes)).
			Msg("Newsnab API error")
		return nil, fmt.Errorf("newsnab API error: %d %s", resp.StatusCode, string(bodyBytes))
	}

	var rss struct {
		Channel struct {
			Items []struct {
				Title   string `xml:"title"`
				Link    string `xml:"link"`
				PubDate string `xml:"pubDate"`
				Size    int64  `xml:"size"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	// Decode XML from the bytes we read
	if err := xml.Unmarshal(bodyBytes, &rss); err != nil {
		log.Error().
			Err(err).
			Str("url", searchURL).
			Str("response_body", string(bodyBytes)).
			Msg("Failed to parse Newsnab XML response")
		return nil, fmt.Errorf("XML parse error: %w", err)
	}

	log.Debug().
		Int("items_in_channel", len(rss.Channel.Items)).
		Msg("Parsed Newsnab XML response")

	results := make([]ports.NewsnabResult, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubDate, _ := time.Parse(time.RFC1123Z, item.PubDate)
		results = append(results, ports.NewsnabResult{
			Title:   item.Title,
			Link:    item.Link,
			Size:    item.Size,
			PubDate: pubDate,
		})

		log.Debug().
			Str("title", item.Title).
			Str("link", item.Link).
			Int64("size", item.Size).
			Msg("Found NZB result")
	}

	log.Info().
		Int("count", len(results)).
		Str("search_type", params.Get("t")).
		Str("imdbid", params.Get("imdbid")).
		Str("season", params.Get("season")).
		Str("episode", params.Get("ep")).
		Msg("Newsnab search completed")

	return results, nil
}
