package ports

import (
	"context"
	"time"
)

// TraktMovie represents a movie from Trakt
type TraktMovie struct {
	TraktID int64
	IMDB    string
	Title   string
	Year    int64
}

// TraktShow represents a TV show from Trakt
type TraktShow struct {
	TraktID int64
	IMDB    string
	Title   string
	Year    int64
}

// TraktEpisode represents an episode from Trakt
type TraktEpisode struct {
	TraktID int64
	Season  int64
	Number  int64
	Title   string
	ShowIMDB string
}

// TraktHistoryItem represents a watched item
type TraktHistoryItem struct {
	TraktID   int64
	WatchedAt time.Time
	Type      string // "movie" or "episode"
}

// TraktClient defines the interface for Trakt.tv integration
type TraktClient interface {
	// Authenticate performs the OAuth device flow
	Authenticate(ctx context.Context) error
	// IsAuthenticated checks if we have a valid token
	IsAuthenticated() bool
	// RefreshToken refreshes the access token if needed
	RefreshToken(ctx context.Context) error
	
	// GetWatchlistMovies returns movies from the user's watchlist
	GetWatchlistMovies(ctx context.Context) ([]TraktMovie, error)
	// GetFavoriteMovies returns favorited movies
	GetFavoriteMovies(ctx context.Context) ([]TraktMovie, error)
	
	// GetWatchlistShows returns shows from the user's watchlist
	GetWatchlistShows(ctx context.Context) ([]TraktShow, error)
	// GetFavoriteShows returns favorited shows
	GetFavoriteShows(ctx context.Context) ([]TraktShow, error)
	
	// GetNextEpisode gets the next unwatched episode for a show
	GetNextEpisode(ctx context.Context, showTraktID int64) (*TraktEpisode, error)
	// GetNextNEpisodes gets the next N unwatched episodes for a show
	GetNextNEpisodes(ctx context.Context, showTraktID int64, limit int) ([]TraktEpisode, error)
	
	// GetWatchHistory returns recently watched items
	GetWatchHistory(ctx context.Context, days int) ([]TraktHistoryItem, error)
}

// NewsnabResult represents a search result from Newsnab
type NewsnabResult struct {
	Title  string
	Link   string
	Size   int64
	PubDate time.Time
}

// NZBSearcher defines the interface for searching NZB indexers
type NZBSearcher interface {
	SearchMovie(ctx context.Context, imdb string) ([]NewsnabResult, error)
	SearchEpisode(ctx context.Context, imdb string, season, episode int64) ([]NewsnabResult, error)
	SearchSeasonPack(ctx context.Context, imdb string, season int64) ([]NewsnabResult, error)
}

// DownloadQueueItem represents an item in the download queue
type DownloadQueueItem struct {
	ID    int64
	Title string
}

// DownloadHistoryItem represents an item in download history
type DownloadHistoryItem struct {
	ID     int64
	Title  string
	Status string
}

// DownloadClient defines the interface for download client operations
type DownloadClient interface {
	// QueueDownload adds a download to the queue
	QueueDownload(ctx context.Context, nzbContent []byte, filename string, category string, priority int, params map[string]string) (int64, error)
	
	// GetQueue returns current queue
	GetQueue(ctx context.Context) ([]DownloadQueueItem, error)
	
	// GetHistory returns download history
	GetHistory(ctx context.Context) ([]DownloadHistoryItem, error)
	
	// DeleteFromHistory removes an item from history
	DeleteFromHistory(ctx context.Context, downloadID int64) error
}
