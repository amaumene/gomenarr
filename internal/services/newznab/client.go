package newznab

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/amaumene/gomenarr/internal/config"
	"github.com/sirupsen/logrus"
)

// NewznabResponse represents the XML RSS response from Newznab API
type NewznabResponse struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}

// Channel represents the channel element in RSS
type Channel struct {
	Title string `xml:"title"`
	Items []Item `xml:"item"`
}

// Item represents a single search result
type Item struct {
	Title      string      `xml:"title"`
	Link       string      `xml:"link"`      // Details page (not for download)
	GUID       string      `xml:"guid"`
	PubDate    string      `xml:"pubDate"`
	Enclosure  Enclosure   `xml:"enclosure"` // The actual NZB download URL
	Attributes []Attribute `xml:"attr"`
}

// Enclosure represents the enclosure element containing the NZB download URL
type Enclosure struct {
	URL    string `xml:"url,attr"`    // The actual NZB download URL
	Length int64  `xml:"length,attr"` // File size
	Type   string `xml:"type,attr"`   // Usually "application/x-nzb"
}

// Attribute represents a Newznab attribute (e.g., season, episode, size)
type Attribute struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Client wraps direct Newznab API HTTP calls
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewClient creates a new Newznab client with direct HTTP calls
func NewClient(cfg *config.Config, logger *logrus.Logger) (*Client, error) {
	if cfg.NewznabURL == "" {
		return nil, fmt.Errorf("newznab URL is required")
	}
	if cfg.NewznabKey == "" {
		return nil, fmt.Errorf("newznab API key is required")
	}

	return &Client{
		baseURL: cfg.NewznabURL,
		apiKey:  cfg.NewznabKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}, nil
}

// search performs Newznab API search
// searchType: always "tvsearch" (works for both movies and TV shows)
// imdbID: IMDB ID of the media (e.g., "tt0133093")
// season: required for TV (always provided), nil for movies
// episode: nil for movies and season packs, set for specific episodes
func (c *Client) search(searchType string, imdbID string, season *int, episode *int) ([]Item, error) {
	// Build base URL
	apiURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid newznab URL: %w", err)
	}

	// Ensure path is /api
	if apiURL.Path == "" || apiURL.Path == "/" {
		apiURL.Path = "/api"
	}

	// Build query parameters
	params := url.Values{}
	params.Add("t", searchType)
	params.Add("apikey", c.apiKey)
	params.Add("imdbid", imdbID)

	// Add season parameter for TV searches
	if season != nil {
		params.Add("season", strconv.Itoa(*season))
	}

	// Add episode parameter for specific episodes
	if episode != nil {
		params.Add("ep", strconv.Itoa(*episode))
	}

	apiURL.RawQuery = params.Encode()
	finalURL := apiURL.String()

	// Log the request
	c.logger.WithFields(logrus.Fields{
		"url":         finalURL,
		"search_type": searchType,
		"imdb_id":     imdbID,
		"season":      season,
		"episode":     episode,
	}).Debug("Performing Newznab search")

	// Make HTTP request
	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "gomenarr/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("newznab API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("Newznab API returned non-OK status")
		return nil, fmt.Errorf("newznab API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse XML response
	var nzResponse NewznabResponse
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&nzResponse); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	c.logger.WithField("count", len(nzResponse.Channel.Items)).Debug("Newznab search completed")

	return nzResponse.Channel.Items, nil
}

// GetAttributeValue extracts an attribute value by name from an Item
func GetAttributeValue(item Item, attrName string) string {
	for _, attr := range item.Attributes {
		if attr.Name == attrName {
			return attr.Value
		}
	}
	return ""
}

// GetAttributeInt extracts an attribute value as integer
func GetAttributeInt(item Item, attrName string) *int {
	value := GetAttributeValue(item, attrName)
	if value == "" {
		return nil
	}

	intVal, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}

	return &intVal
}

// GetAttributeInt64 extracts an attribute value as int64
func GetAttributeInt64(item Item, attrName string) int64 {
	value := GetAttributeValue(item, attrName)
	if value == "" {
		return 0
	}

	intVal, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}

	return intVal
}

// DownloadNZB downloads the actual NZB file from the enclosure URL
// Returns the NZB file content as bytes (can be up to 10MB)
func (c *Client) DownloadNZB(enclosureURL string) ([]byte, error) {
	c.logger.WithField("url", enclosureURL).Debug("Downloading NZB file")

	// Create HTTP request
	req, err := http.NewRequest("GET", enclosureURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create NZB download request: %w", err)
	}

	req.Header.Set("User-Agent", "gomenarr/1.0")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download NZB: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NZB download failed with status %d", resp.StatusCode)
	}

	// Read NZB file content (limit to 15MB to be safe)
	const maxNZBSize = 15 * 1024 * 1024 // 15MB
	limitReader := io.LimitReader(resp.Body, maxNZBSize)
	nzbData, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read NZB content: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"size_bytes": len(nzbData),
		"size_kb":    len(nzbData) / 1024,
	}).Debug("NZB file downloaded successfully")

	return nzbData, nil
}
