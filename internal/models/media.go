package models

import "time"

// Media represents a media item from Trakt (TV show or movie)
type Media struct {
	ID     uint64 `boltholdKey:"ID"`
	IMDBId string `boltholdIndex:"IMDBId"` // IMDB ID for accurate Newznab searches

	MediaType MediaType // "movie" or "tv"
	Title     string
	Year      int

	// TV Show specific fields
	SeasonNumber  *int // nil for movies
	EpisodeNumber *int // nil for movies/seasons

	// Tracking
	Source  Source // "favorites" or "watchlist"
	Status  Status // "pending", "searching", "downloading", "completed", "failed"
	Watched bool

	// Trakt presence tracking (for cleanup of removed items)
	InTrakt         bool      `boltholdIndex:"InTrakt"` // Currently in Trakt lists?
	LastSeenInTrakt time.Time // Last seen during Trakt sync

	// Metadata
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastSearchedAt *time.Time
	CompletedAt    *time.Time
}
