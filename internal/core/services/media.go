package services

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type MediaService struct {
	repo           ports.MediaRepository
	nzbRepo        ports.NZBRepository
	traktClient    ports.TraktClient
	downloadClient ports.DownloadClient
	traktCfg       config.TraktConfig
	downloadCfg    config.DownloadConfig
}

func NewMediaService(
	repo ports.MediaRepository,
	nzbRepo ports.NZBRepository,
	traktClient ports.TraktClient,
	downloadClient ports.DownloadClient,
	traktCfg config.TraktConfig,
	downloadCfg config.DownloadConfig,
) *MediaService {
	return &MediaService{
		repo:           repo,
		nzbRepo:        nzbRepo,
		traktClient:    traktClient,
		downloadClient: downloadClient,
		traktCfg:       traktCfg,
		downloadCfg:    downloadCfg,
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

	// Upsert to database (skip watched content)
	count := 0
	for _, movie := range movieMap {
		// Check if movie is watched (movies use Trakt ID, pass empty IMDB and 0 for season/episode)
		watched, err := s.traktClient.IsWatched(ctx, movie.TraktID, "movie", "", 0, 0)
		if err != nil {
			log.Warn().Err(err).Int64("movie_id", movie.TraktID).Msg("Failed to check watched status, skipping movie")
			continue
		}

		if watched {
			log.Debug().
				Int64("movie_id", movie.TraktID).
				Str("title", movie.Title).
				Msg("Skipping watched movie")
			continue
		}

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

	// Cleanup orphaned movies (in DB but no longer in Trakt lists)
	if err := s.cleanupOrphanedMovies(ctx, movieMap); err != nil {
		log.Error().Err(err).Msg("Failed to cleanup orphaned movies")
		// Don't return error, just log it
	}

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

				// Upsert each episode (skip watched content)
				for _, ep := range episodes {
					// Check if episode is watched (using IMDB, season, episode composite key)
					watched, err := s.traktClient.IsWatched(ctx, ep.TraktID, "episode", ep.ShowIMDB, ep.Season, ep.Number)
					if err != nil {
						log.Warn().
							Err(err).
							Int64("episode_id", ep.TraktID).
							Str("show_imdb", ep.ShowIMDB).
							Int64("season", ep.Season).
							Int64("episode", ep.Number).
							Int("worker_id", workerID).
							Msg("Failed to check watched status, skipping episode")
						continue
					}

					if watched {
						log.Debug().
							Int64("episode_id", ep.TraktID).
							Str("title", ep.Title).
							Str("show_imdb", ep.ShowIMDB).
							Int64("season", ep.Season).
							Int64("episode", ep.Number).
							Int("worker_id", workerID).
							Msg("Skipping watched episode")
						continue
					}

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

	// Queue watchlist shows (3 episodes each)
	for _, show := range watchlistShows {
		jobs <- showJob{
			show:         show,
			episodeLimit: 3,
			showType:     "watchlist",
		}
	}

	// Queue favorite shows (N episodes each)
	for _, show := range favoriteShows {
		jobs <- showJob{
			show:         show,
			episodeLimit: s.traktCfg.FavoritesEpisodeLimit,
			showType:     "favorite",
		}
	}

	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	log.Info().Int("count", count).Msg("Synced episodes from Trakt")

	// Cleanup orphaned episodes (shows no longer in Trakt lists)
	allShows := append(watchlistShows, favoriteShows...)
	if err := s.cleanupOrphanedEpisodes(ctx, allShows); err != nil {
		log.Error().Err(err).Msg("Failed to cleanup orphaned episodes")
		// Don't return error, just log it
	}

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

func (s *MediaService) cleanupOrphanedMovies(ctx context.Context, traktMovies map[int64]ports.TraktMovie) error {
	// Get all media from DB
	allMedia, err := s.repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all media: %w", err)
	}

	// Find orphaned movies (in DB but not in Trakt lists and not watched)
	orphanedIDs := make([]int64, 0)
	orphanedMedia := make([]*domain.Media, 0)
	for _, media := range allMedia {
		if !media.IsMovie() {
			continue // Skip episodes
		}

		// Check if movie is still in Trakt lists
		if _, exists := traktMovies[media.TraktID]; !exists {
			orphanedIDs = append(orphanedIDs, media.TraktID)
			orphanedMedia = append(orphanedMedia, media)
		}
	}

	if len(orphanedIDs) == 0 {
		log.Debug().Msg("No orphaned movies to cleanup")
		return nil
	}

	log.Info().
		Int("count", len(orphanedIDs)).
		Msg("Found orphaned movies removed from Trakt lists")

	// Cancel active downloads in NZBGet queue
	canceledCount := 0
	for _, media := range orphanedMedia {
		if media.DownloadID > 0 {
			if err := s.downloadClient.CancelDownload(ctx, media.DownloadID); err != nil {
				log.Warn().
					Err(err).
					Int64("download_id", media.DownloadID).
					Int64("trakt_id", media.TraktID).
					Msg("Failed to cancel download, continuing cleanup")
			} else {
				canceledCount++
				log.Debug().
					Int64("download_id", media.DownloadID).
					Int64("trakt_id", media.TraktID).
					Msg("Canceled download in NZBGet")
			}
		}
	}
	if canceledCount > 0 {
		log.Info().Int("count", canceledCount).Msg("Canceled active downloads")
	}

	// Delete files from disk if configured
	if s.downloadCfg.DeleteFiles {
		deletedFiles := 0
		for _, media := range orphanedMedia {
			if media.Path != "" {
				if err := os.RemoveAll(media.Path); err != nil {
					log.Error().
						Err(err).
						Str("path", media.Path).
						Int64("trakt_id", media.TraktID).
						Msg("Failed to delete directory")
				} else {
					deletedFiles++
					log.Info().
						Str("path", media.Path).
						Int64("trakt_id", media.TraktID).
						Msg("Deleted directory and all contents")
				}
			}
		}
		if deletedFiles > 0 {
			log.Info().Int("count", deletedFiles).Msg("Deleted orphaned files from disk")
		}
	}

	// Delete NZB records from database
	if err := s.nzbRepo.DeleteByTraktIDs(ctx, orphanedIDs); err != nil {
		log.Error().Err(err).Msg("Failed to delete orphaned NZB records")
		// Don't fail, continue with media deletion
	} else {
		log.Debug().Int("count", len(orphanedIDs)).Msg("Deleted orphaned NZB records")
	}

	// Delete orphaned media from database
	if err := s.repo.DeleteByTraktIDs(ctx, orphanedIDs); err != nil {
		return fmt.Errorf("failed to delete orphaned movies: %w", err)
	}

	log.Info().Int("count", len(orphanedIDs)).Msg("Completed cleanup of orphaned movies")
	return nil
}

func (s *MediaService) cleanupOrphanedEpisodes(ctx context.Context, traktShows []ports.TraktShow) error {
	// Build set of show IMDB IDs that are in Trakt lists
	showIMDBs := make(map[string]bool)
	for _, show := range traktShows {
		if show.IMDB != "" {
			showIMDBs[show.IMDB] = true
		}
	}

	// Get all media from DB
	allMedia, err := s.repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all media: %w", err)
	}

	// Find orphaned episodes (show no longer in Trakt lists)
	orphanedIDs := make([]int64, 0)
	orphanedMedia := make([]*domain.Media, 0)
	for _, media := range allMedia {
		if !media.IsEpisode() {
			continue // Skip movies
		}

		// Check if the show is still in Trakt lists (by IMDB)
		if !showIMDBs[media.IMDB] {
			orphanedIDs = append(orphanedIDs, media.TraktID)
			orphanedMedia = append(orphanedMedia, media)
		}
	}

	if len(orphanedIDs) == 0 {
		log.Debug().Msg("No orphaned episodes to cleanup")
		return nil
	}

	log.Info().
		Int("count", len(orphanedIDs)).
		Msg("Found orphaned episodes (shows removed from Trakt lists)")

	// Cancel active downloads in NZBGet queue
	canceledCount := 0
	for _, media := range orphanedMedia {
		if media.DownloadID > 0 {
			if err := s.downloadClient.CancelDownload(ctx, media.DownloadID); err != nil {
				log.Warn().
					Err(err).
					Int64("download_id", media.DownloadID).
					Int64("trakt_id", media.TraktID).
					Msg("Failed to cancel download, continuing cleanup")
			} else {
				canceledCount++
				log.Debug().
					Int64("download_id", media.DownloadID).
					Int64("trakt_id", media.TraktID).
					Msg("Canceled download in NZBGet")
			}
		}
	}
	if canceledCount > 0 {
		log.Info().Int("count", canceledCount).Msg("Canceled active downloads")
	}

	// Delete files from disk if configured
	if s.downloadCfg.DeleteFiles {
		deletedFiles := 0
		for _, media := range orphanedMedia {
			if media.Path != "" {
				if err := os.RemoveAll(media.Path); err != nil {
					log.Error().
						Err(err).
						Str("path", media.Path).
						Int64("trakt_id", media.TraktID).
						Msg("Failed to delete directory")
				} else {
					deletedFiles++
					log.Info().
						Str("path", media.Path).
						Int64("trakt_id", media.TraktID).
						Msg("Deleted directory and all contents")
				}
			}
		}
		if deletedFiles > 0 {
			log.Info().Int("count", deletedFiles).Msg("Deleted orphaned files from disk")
		}
	}

	// Delete NZB records from database
	if err := s.nzbRepo.DeleteByTraktIDs(ctx, orphanedIDs); err != nil {
		log.Error().Err(err).Msg("Failed to delete orphaned NZB records")
		// Don't fail, continue with media deletion
	} else {
		log.Debug().Int("count", len(orphanedIDs)).Msg("Deleted orphaned NZB records")
	}

	// Delete orphaned media from database
	if err := s.repo.DeleteByTraktIDs(ctx, orphanedIDs); err != nil {
		return fmt.Errorf("failed to delete orphaned episodes: %w", err)
	}

	log.Info().Int("count", len(orphanedIDs)).Msg("Completed cleanup of orphaned episodes")
	return nil
}
