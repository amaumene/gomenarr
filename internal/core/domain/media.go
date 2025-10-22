package domain

import "time"

// Media represents both movies and TV episodes
type Media struct {
	TraktID    int64     `gorm:"primaryKey;uniqueIndex" json:"trakt_id"`
	IMDB       string    `gorm:"index" json:"imdb"`
	Number     int64     `json:"number"`       // Episode number (0 for movies)
	Season     int64     `json:"season"`       // Season number (0 for movies)
	Title      string    `json:"title"`
	Year       int64     `json:"year"`
	OnDisk     bool      `gorm:"index" json:"on_disk"`
	Path       string    `json:"path"` // Path to downloaded directory
	DownloadID int64     `json:"download_id"`  // NZBGet download ID
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName specifies the table name for GORM
func (Media) TableName() string {
	return "media"
}

// IsMovie returns true if this media is a movie
func (m *Media) IsMovie() bool {
	return m.Season == 0 && m.Number == 0
}

// IsEpisode returns true if this media is an episode
func (m *Media) IsEpisode() bool {
	return m.Season > 0 && m.Number > 0
}
