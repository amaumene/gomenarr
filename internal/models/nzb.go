package models

import "time"

// NZB represents an NZB search result and download attempt
type NZB struct {
	ID      uint64 `boltholdKey:"ID"`
	MediaID uint64 `boltholdIndex:"MediaID"`

	// NZB details
	Title   string
	Link    string
	GUID    string
	Size    int64   // bytes
	Quality Quality
	Year    int // Extracted from NZB title (for movies)

	// Download tracking
	TorBoxJobID   string    `boltholdIndex:"TorBoxJobID"`
	TorBoxHash    string    `boltholdIndex:"TorBoxHash"` // Hash from TorBox for webhook matching
	Status        NZBStatus `boltholdIndex:"Status"`
	RetryCount    int
	FailureReason string

	// Blacklist check
	BlacklistMatch string // Which blacklist term matched (if any)

	// Episode/Season tracking (parsed from NZB title)
	Season       *int // Season number (for individual episodes AND season packs)
	Episode      *int // Episode number (nil for season packs)
	IsSeasonPack bool

	// Season pack episode list (populated from Trakt API when IsSeasonPack=true)
	Episodes []EpisodeInfo // Episodes in this pack (from Trakt)

	// Metadata
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DownloadedAt *time.Time
}

// EpisodeInfo tracks individual episodes in a season pack
type EpisodeInfo struct {
	EpisodeNumber int
	Watched       bool
	WatchedAt     *time.Time
}
