package newznab

import (
	"fmt"
	"regexp"
	"strconv"
)

// SearchResult represents a search result from Newznab
type SearchResult struct {
	Title        string
	Link         string
	GUID         string
	Size         int64
	Season       *int
	Episode      *int
	IsSeasonPack bool
}

// SearchByIMDBID searches for content by IMDB ID (movies only)
func (c *Client) SearchByIMDBID(imdbID string, mediaType string) ([]SearchResult, error) {
	if mediaType != "movie" {
		return nil, fmt.Errorf("SearchByIMDBID only supports movies, got: %s", mediaType)
	}

	c.logger.WithField("imdb_id", imdbID).Debug("Searching for movie by IMDB ID")

	items, err := c.search("tvsearch", imdbID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("movie search failed: %w", err)
	}

	return c.convertResults(items), nil
}

// SearchEpisode searches for a specific episode by IMDB ID
func (c *Client) SearchEpisode(imdbID string, season, episode int) ([]SearchResult, error) {
	c.logger.WithFields(map[string]interface{}{
		"imdb_id": imdbID,
		"season":  season,
		"episode": episode,
	}).Debug("Searching for TV episode by IMDB ID")

	items, err := c.search("tvsearch", imdbID, &season, &episode)
	if err != nil {
		return nil, fmt.Errorf("episode search failed: %w", err)
	}

	return c.convertResults(items), nil
}

// SearchSeason searches for a season pack by IMDB ID
func (c *Client) SearchSeason(imdbID string, season int) ([]SearchResult, error) {
	c.logger.WithFields(map[string]interface{}{
		"imdb_id": imdbID,
		"season":  season,
	}).Debug("Searching for TV season pack by IMDB ID")

	// Search with season but no episode to get season packs
	items, err := c.search("tvsearch", imdbID, &season, nil)
	if err != nil {
		return nil, fmt.Errorf("season search failed: %w", err)
	}

	// Convert all results
	results := c.convertResults(items)

	// Filter to only season packs
	var seasonPacks []SearchResult
	for _, result := range results {
		if result.IsSeasonPack {
			seasonPacks = append(seasonPacks, result)
		}
	}

	c.logger.WithField("season_packs", len(seasonPacks)).Debug("Filtered to season packs only")

	return seasonPacks, nil
}

// parseSeasonEpisode extracts season and episode numbers from title
// Returns (season, episode, isSeasonPack)
func parseSeasonEpisode(title string) (*int, *int, bool) {
	// Try to match single episode pattern first: S01E01, S02E05, etc.
	episodeRegex := regexp.MustCompile(`(?i)[\._ ]S(\d{1,2})E(\d{1,2})`)
	if matches := episodeRegex.FindStringSubmatch(title); matches != nil {
		season, _ := strconv.Atoi(matches[1])
		episode, _ := strconv.Atoi(matches[2])
		return &season, &episode, false
	}

	// Pattern for season pack: S01, S02, etc. (no episode number)
	// Only matches if episode pattern didn't match (no E following S##)
	seasonPackRegex := regexp.MustCompile(`(?i)[\._ ]S(\d{1,2})(?:[\._ ]|$)`)
	if matches := seasonPackRegex.FindStringSubmatch(title); matches != nil {
		season, _ := strconv.Atoi(matches[1])
		return &season, nil, true
	}

	return nil, nil, false
}

// convertResults converts Newznab Items to SearchResult format
func (c *Client) convertResults(items []Item) []SearchResult {
	results := make([]SearchResult, 0, len(items))

	for _, item := range items {
		result := SearchResult{
			Title: item.Title,
			Link:  item.Enclosure.URL, // Use the enclosure URL (NZB download link) instead of item.Link (details page)
			GUID:  item.GUID,
		}

		// DEBUG: Log the URL extraction
		c.logger.WithFields(map[string]interface{}{
			"title":         item.Title,
			"enclosure_url": item.Enclosure.URL,
			"link_element":  item.Link,
		}).Debug("Extracted NZB URL from XML")

		// Extract size from attributes
		result.Size = GetAttributeInt64(item, "size")

		// Parse season/episode from title (attributes are not provided by indexer)
		parsedSeason, parsedEpisode, isSeasonPack := parseSeasonEpisode(item.Title)
		result.Season = parsedSeason
		result.Episode = parsedEpisode
		result.IsSeasonPack = isSeasonPack

		results = append(results, result)
	}

	return results
}
