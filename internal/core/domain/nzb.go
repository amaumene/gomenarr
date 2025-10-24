package domain

import "time"

// NZB represents a download release candidate
type NZB struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TraktID         int64     `gorm:"index" json:"trakt_id"`
	IMDB            string    `gorm:"index:idx_nzbs_imdb_season_pack,priority:1;index:idx_nzbs_imdb_season,priority:1" json:"imdb"`
	Link            string    `json:"link"`
	Length          int64     `json:"length"`
	Title           string    `json:"title"`
	Failed          bool      `gorm:"index" json:"failed"`
	ParsedTitle     string    `json:"parsed_title"`
	ParsedYear      int64     `json:"parsed_year"`
	ParsedSeason    int64     `gorm:"index:idx_nzbs_imdb_season_pack,priority:2;index:idx_nzbs_imdb_season,priority:2" json:"parsed_season"`
	ParsedEpisode   int64     `json:"parsed_episode"`
	IsSeasonPack    bool      `gorm:"index:idx_nzbs_imdb_season_pack,priority:3,unique" json:"is_season_pack"`
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
