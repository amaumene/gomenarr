package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type MediaService struct {
	repo        ports.MediaRepository
	traktClient ports.TraktClient
	cfg         config.TraktConfig
}

func NewMediaService(repo ports.MediaRepository, traktClient ports.TraktClient, cfg config.TraktConfig) *MediaService {
	return &MediaService{
		repo:        repo,
		traktClient: traktClient,
		cfg:         cfg,
	}
}

func (s *MediaService) SyncMovies(ctx context.Context) error {
	log.Info().Msg("Syncing movies from Trakt")

	// Get watchlist movies
	watchlist, err := s.traktClient.GetWatchlistMovies(ctx)
	if err != nil {
		return fmt.Errorf("failed to get watchlist movies: %w", err)
	}

	// Get favorite movies
	favorites, err := s.traktClient.GetFavoriteMovies(ctx)
	if err != nil {
		return fmt.Errorf("failed to get favorite movies: %w", err)
	}

	// Combine and deduplicate
	movieMap := make(map[int64]ports.TraktMovie)
	for _, movie := range watchlist {
		if movie.TraktID > 0 && movie.IMDB != "" {
			movieMap[movie.TraktID] = movie
		}
	}
	for _, movie := range favorites {
		if movie.TraktID > 0 && movie.IMDB != "" {
			movieMap[movie.TraktID] = movie
		}
	}

	// Upsert to database
	count := 0
	for _, movie := range movieMap {
		media := &domain.Media{
			TraktID: movie.TraktID,
			IMDB:    movie.IMDB,
			Title:   movie.Title,
			Year:    movie.Year,
			Number:  0,
			Season:  0,
		}

		if err := s.repo.Upsert(ctx, media); err != nil {
			log.Error().Err(err).Int64("trakt_id", movie.TraktID).Msg("Failed to upsert movie")
			continue
		}
		count++
	}

	log.Info().Int("count", count).Msg("Synced movies from Trakt")
	return nil
}

func (s *MediaService) SyncEpisodes(ctx context.Context) error {
	log.Info().Msg("Syncing episodes from Trakt")

	// Get watchlist shows (1 episode each)
	watchlistShows, err := s.traktClient.GetWatchlistShows(ctx)
	if err != nil {
		return fmt.Errorf("failed to get watchlist shows: %w", err)
	}

	// Get favorite shows (N episodes each)
	favoriteShows, err := s.traktClient.GetFavoriteShows(ctx)
	if err != nil {
		return fmt.Errorf("failed to get favorite shows: %w", err)
	}

	// Job structure for parallel processing
	type showJob struct {
		show         ports.TraktShow
		episodeLimit int
		showType     string
	}

	// Create job queue with all shows
	totalShows := len(watchlistShows) + len(favoriteShows)
	jobs := make(chan showJob, totalShows)
	var wg sync.WaitGroup
	var countMutex sync.Mutex
	count := 0

	// Use worker pool for parallel episode fetching (5 concurrent workers)
	const numWorkers = 5
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				// Fetch episodes for this show
				episodes, err := s.traktClient.GetNextNEpisodes(ctx, job.show.TraktID, job.episodeLimit)
				if err != nil {
					log.Error().
						Err(err).
						Int64("show_id", job.show.TraktID).
						Str("show_type", job.showType).
						Int("worker_id", workerID).
						Msg("Failed to get next episodes")
					continue
				}

				// Upsert each episode
				for _, ep := range episodes {
					if err := s.upsertEpisode(ctx, ep); err != nil {
						log.Error().
							Err(err).
							Int64("episode_id", ep.TraktID).
							Int("worker_id", workerID).
							Msg("Failed to upsert episode")
						continue
					}
					countMutex.Lock()
					count++
					countMutex.Unlock()
				}
			}
		}(i)
	}

	// Queue watchlist shows (1 episode each)
	for _, show := range watchlistShows {
		jobs <- showJob{
			show:         show,
			episodeLimit: 1,
			showType:     "watchlist",
		}
	}

	// Queue favorite shows (N episodes each)
	for _, show := range favoriteShows {
		jobs <- showJob{
			show:         show,
			episodeLimit: s.cfg.FavoritesEpisodeLimit,
			showType:     "favorite",
		}
	}

	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	log.Info().Int("count", count).Msg("Synced episodes from Trakt")
	return nil
}

func (s *MediaService) upsertEpisode(ctx context.Context, ep ports.TraktEpisode) error {
	// Validate episode data with detailed logging
	if ep.TraktID <= 0 {
		log.Error().
			Int64("trakt_id", ep.TraktID).
			Str("title", ep.Title).
			Int64("season", ep.Season).
			Int64("episode", ep.Number).
			Str("show_imdb", ep.ShowIMDB).
			Msg("Invalid episode: TraktID must be positive")
		return fmt.Errorf("invalid episode data: TraktID must be positive")
	}

	if ep.ShowIMDB == "" {
		log.Warn().
			Int64("trakt_id", ep.TraktID).
			Str("title", ep.Title).
			Int64("season", ep.Season).
			Int64("episode", ep.Number).
			Msg("Skipping episode: Show has no IMDB ID (required for NZB search)")
		return fmt.Errorf("invalid episode data: Show IMDB ID is empty")
	}

	if ep.Season <= 0 {
		log.Error().
			Int64("trakt_id", ep.TraktID).
			Str("title", ep.Title).
			Int64("season", ep.Season).
			Int64("episode", ep.Number).
			Str("show_imdb", ep.ShowIMDB).
			Msg("Invalid episode: Season must be positive")
		return fmt.Errorf("invalid episode data: Season must be positive")
	}

	if ep.Number <= 0 {
		log.Error().
			Int64("trakt_id", ep.TraktID).
			Str("title", ep.Title).
			Int64("season", ep.Season).
			Int64("episode", ep.Number).
			Str("show_imdb", ep.ShowIMDB).
			Msg("Invalid episode: Episode number must be positive")
		return fmt.Errorf("invalid episode data: Episode number must be positive")
	}

	media := &domain.Media{
		TraktID: ep.TraktID,
		IMDB:    ep.ShowIMDB,
		Title:   ep.Title,
		Season:  ep.Season,
		Number:  ep.Number,
	}

	return s.repo.Upsert(ctx, media)
}

func (s *MediaService) GetAll(ctx context.Context) ([]*domain.Media, error) {
	return s.repo.FindAll(ctx)
}

func (s *MediaService) GetNotOnDisk(ctx context.Context) ([]*domain.Media, error) {
	return s.repo.FindNotOnDisk(ctx)
}

func (s *MediaService) GetByTraktID(ctx context.Context, traktID int64) (*domain.Media, error) {
	return s.repo.FindByTraktID(ctx, traktID)
}

func (s *MediaService) Update(ctx context.Context, media *domain.Media) error {
	return s.repo.Update(ctx, media)
}
