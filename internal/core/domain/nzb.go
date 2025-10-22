package domain

import "time"

// NZB represents a download release candidate
type NZB struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TraktID         int64     `gorm:"index" json:"trakt_id"`
	Link            string    `json:"link"`
	Length          int64     `json:"length"`
	Title           string    `json:"title"`
	Failed          bool      `gorm:"index" json:"failed"`
	ParsedTitle     string    `json:"parsed_title"`
	ParsedYear      int64     `json:"parsed_year"`
	ParsedSeason    int64     `json:"parsed_season"`
	ParsedEpisode   int64     `json:"parsed_episode"`
	Resolution      string    `json:"resolution"`
	Source          string    `json:"source"`
	Codec           string    `json:"codec"`
	ValidationScore int       `gorm:"index" json:"validation_score"`
	QualityScore    int       `gorm:"index" json:"quality_score"`
	TotalScore      int       `gorm:"index" json:"total_score"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName specifies the table name for GORM
func (NZB) TableName() string {
	return "nzbs"
}

// IsSeasonPack returns true if this NZB is a season pack
func (n *NZB) IsSeasonPack() bool {
	return n.ParsedSeason > 0 && n.ParsedEpisode == 0
}
