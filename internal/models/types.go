package models

// MediaType represents the type of media (movie or tv show)
type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
)

// Source represents where the media came from (favorites or watchlist)
type Source string

const (
	SourceFavorites Source = "favorites"
	SourceWatchlist Source = "watchlist"
)

// Status represents the current processing status of a media item
type Status string

const (
	StatusPending     Status = "pending"
	StatusSearching   Status = "searching"
	StatusDownloading Status = "downloading"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

// Quality represents the quality tier of an NZB
type Quality string

const (
	QualityREMUX Quality = "REMUX"
	QualityWEBDL Quality = "WEB-DL"
	QualityOther Quality = "OTHER"
)

// NZBStatus represents the status of an NZB download
type NZBStatus string

const (
	NZBStatusCandidate   NZBStatus = "candidate"   // Found but not selected
	NZBStatusSelected    NZBStatus = "selected"    // Best quality, ready to download
	NZBStatusDownloading NZBStatus = "downloading" // Sent to TorBox
	NZBStatusCompleted   NZBStatus = "completed"   // Successfully downloaded
	NZBStatusFailed      NZBStatus = "failed"      // Download failed
	NZBStatusBlacklisted NZBStatus = "blacklisted" // Matched blacklist
)
