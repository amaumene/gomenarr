package newznab

import (
	"encoding/xml"
	"testing"
)

func TestXMLParsing(t *testing.T) {
	// Sample Newznab XML response
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/">
  <channel>
    <title>Test Indexer</title>
    <item>
      <title>Test Movie 2024 1080p BluRay x264</title>
      <link>https://example.com/download/12345</link>
      <guid>https://example.com/details/12345</guid>
      <pubDate>Mon, 01 Jan 2024 12:00:00 +0000</pubDate>
      <newznab:attr name="size" value="8589934592"/>
      <newznab:attr name="category" value="2000"/>
    </item>
    <item>
      <title>Test Show S01E01 1080p WEB-DL</title>
      <link>https://example.com/download/12346</link>
      <guid>https://example.com/details/12346</guid>
      <pubDate>Tue, 02 Jan 2024 12:00:00 +0000</pubDate>
      <newznab:attr name="size" value="2147483648"/>
      <newznab:attr name="season" value="1"/>
      <newznab:attr name="episode" value="1"/>
      <newznab:attr name="category" value="5000"/>
    </item>
    <item>
      <title>Test Show S02 1080p WEB-DL Season Pack</title>
      <link>https://example.com/download/12347</link>
      <guid>https://example.com/details/12347</guid>
      <pubDate>Wed, 03 Jan 2024 12:00:00 +0000</pubDate>
      <newznab:attr name="size" value="21474836480"/>
      <newznab:attr name="season" value="2"/>
      <newznab:attr name="category" value="5000"/>
    </item>
  </channel>
</rss>`

	var response NewznabResponse
	err := xml.Unmarshal([]byte(xmlData), &response)
	if err != nil {
		t.Fatalf("Failed to parse XML: %v", err)
	}

	// Verify channel
	if response.Channel.Title != "Test Indexer" {
		t.Errorf("Expected channel title 'Test Indexer', got '%s'", response.Channel.Title)
	}

	// Verify items count
	if len(response.Channel.Items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(response.Channel.Items))
	}

	// Test movie item (no season/episode)
	movieItem := response.Channel.Items[0]
	if movieItem.Title != "Test Movie 2024 1080p BluRay x264" {
		t.Errorf("Movie title mismatch")
	}
	if GetAttributeInt64(movieItem, "size") != 8589934592 {
		t.Errorf("Movie size mismatch")
	}
	if GetAttributeInt(movieItem, "season") != nil {
		t.Errorf("Movie should not have season attribute")
	}

	// Test episode item
	episodeItem := response.Channel.Items[1]
	season := GetAttributeInt(episodeItem, "season")
	episode := GetAttributeInt(episodeItem, "episode")
	if season == nil || *season != 1 {
		t.Errorf("Expected season 1, got %v", season)
	}
	if episode == nil || *episode != 1 {
		t.Errorf("Expected episode 1, got %v", episode)
	}
	if GetAttributeInt64(episodeItem, "size") != 2147483648 {
		t.Errorf("Episode size mismatch")
	}

	// Test season pack item (has season, no episode)
	seasonPackItem := response.Channel.Items[2]
	spSeason := GetAttributeInt(seasonPackItem, "season")
	spEpisode := GetAttributeInt(seasonPackItem, "episode")
	if spSeason == nil || *spSeason != 2 {
		t.Errorf("Expected season 2, got %v", spSeason)
	}
	if spEpisode != nil {
		t.Errorf("Season pack should not have episode attribute, got %v", spEpisode)
	}
	if GetAttributeInt64(seasonPackItem, "size") != 21474836480 {
		t.Errorf("Season pack size mismatch")
	}
}

func TestConvertResults(t *testing.T) {
	// Create mock client (minimal setup for testing)
	client := &Client{}

	// Test items
	items := []Item{
		{
			Title: "Movie Title 2024 1080p",
			Link:  "https://example.com/movie",
			GUID:  "movie-guid",
			Attributes: []Attribute{
				{Name: "size", Value: "1073741824"},
			},
		},
		{
			Title: "Show S01E01",
			Link:  "https://example.com/episode",
			GUID:  "episode-guid",
			Attributes: []Attribute{
				{Name: "size", Value: "2147483648"},
				{Name: "season", Value: "1"},
				{Name: "episode", Value: "1"},
			},
		},
		{
			Title: "Show S02 Complete",
			Link:  "https://example.com/season",
			GUID:  "season-guid",
			Attributes: []Attribute{
				{Name: "size", Value: "21474836480"},
				{Name: "season", Value: "2"},
			},
		},
	}

	results := client.convertResults(items)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Test movie result
	if results[0].IsSeasonPack {
		t.Error("Movie should not be marked as season pack")
	}
	if results[0].Size != 1073741824 {
		t.Errorf("Movie size mismatch: %d", results[0].Size)
	}

	// Test episode result
	if results[1].IsSeasonPack {
		t.Error("Episode should not be marked as season pack")
	}
	if results[1].Season == nil || *results[1].Season != 1 {
		t.Error("Episode season mismatch")
	}
	if results[1].Episode == nil || *results[1].Episode != 1 {
		t.Error("Episode number mismatch")
	}

	// Test season pack result
	if !results[2].IsSeasonPack {
		t.Error("Season pack should be marked as season pack")
	}
	if results[2].Season == nil || *results[2].Season != 2 {
		t.Error("Season pack season mismatch")
	}
	if results[2].Episode != nil {
		t.Error("Season pack should not have episode number")
	}
}
