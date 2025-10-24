package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type DownloadService struct {
	mediaRepo      ports.MediaRepository
	nzbRepo        ports.NZBRepository
	downloadClient ports.DownloadClient
	cfg            config.NZBGetConfig
	httpClient     *http.Client
}

func NewDownloadService(
	mediaRepo ports.MediaRepository,
	nzbRepo ports.NZBRepository,
	downloadClient ports.DownloadClient,
	cfg config.NZBGetConfig,
) *DownloadService {
	// Configure HTTP transport with connection pooling for better performance
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	return &DownloadService{
		mediaRepo:      mediaRepo,
		nzbRepo:        nzbRepo,
		downloadClient: downloadClient,
		cfg:            cfg,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (s *DownloadService) DownloadMedia(ctx context.Context) error {
	log.Info().Msg("Starting download process")

	// Get all media not on disk
	mediaList, err := s.mediaRepo.FindNotOnDisk(ctx)
	if err != nil {
		return fmt.Errorf("failed to get media not on disk: %w", err)
	}

	// Get current queue
	queue, err := s.downloadClient.GetQueue(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queue: %w", err)
	}

	// Get history
	history, err := s.downloadClient.GetHistory(ctx)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	// Track processed season packs with concurrent access protection
	processedSeasons := make(map[string]bool)
	var seasonMutex sync.Mutex

	// Use worker pool for parallel download queueing (3 concurrent workers)
	const numWorkers = 3
	jobs := make(chan *domain.Media, len(mediaList))
	var wg sync.WaitGroup
	var countMutex sync.Mutex
	count := 0

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for media := range jobs {
				// Season pack deduplication check (with mutex)
				if media.IsEpisode() {
					seasonKey := fmt.Sprintf("%s_S%d", media.IMDB, media.Season)
					seasonMutex.Lock()
					if processedSeasons[seasonKey] {
						seasonMutex.Unlock()
						log.Debug().Str("season_key", seasonKey).Int("worker_id", workerID).Msg("Skipping episode, season pack already processed")
						continue
					}
					seasonMutex.Unlock()
				}

				// Get best NZB - prefer season packs for episodes
				var nzb *domain.NZB
				var err error

				if media.IsEpisode() && media.IMDB != "" {
					// Try to find season pack first
					nzb, err = s.nzbRepo.FindBestSeasonPack(ctx, media.IMDB, media.Season)
					if err == nil {
						log.Debug().
							Str("imdb", media.IMDB).
							Int64("season", media.Season).
							Str("title", nzb.Title).
							Int("worker_id", workerID).
							Msg("Found season pack for episode")
					}
				}

				// Fallback to episode-specific NZB if no season pack found
				if nzb == nil {
					nzb, err = s.nzbRepo.FindBestByTraktID(ctx, media.TraktID)
					if err != nil {
						log.Debug().Int64("trakt_id", media.TraktID).Int("worker_id", workerID).Msg("No NZB found for media")
						continue
					}
				}

				// Check if already in queue
				if s.isInQueue(nzb.Title, queue) {
					log.Debug().Str("title", nzb.Title).Int("worker_id", workerID).Msg("Already in queue")
					continue
				}

				// Check if already in history
				if s.isInHistory(media.DownloadID, history) {
					log.Debug().Int64("download_id", media.DownloadID).Int("worker_id", workerID).Msg("Already in history")
					continue
				}

				// Download
				if err := s.QueueNZB(ctx, media, nzb); err != nil {
					log.Error().Err(err).Str("title", nzb.Title).Int("worker_id", workerID).Msg("Failed to queue NZB")
					continue
				}

				// Mark season as processed if it's a season pack (with mutex)
				if media.IsEpisode() && nzb.IsSeasonPack {
					seasonKey := fmt.Sprintf("%s_S%d", media.IMDB, media.Season)
					seasonMutex.Lock()
					processedSeasons[seasonKey] = true
					seasonMutex.Unlock()
					log.Info().Str("season_key", seasonKey).Int("worker_id", workerID).Msg("Marked season pack as processed")
				}

				countMutex.Lock()
				count++
				countMutex.Unlock()
			}
		}(i)
	}

	// Send jobs to workers
	for _, media := range mediaList {
		jobs <- media
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	log.Info().Int("count", count).Msg("Queued downloads")
	return nil
}

func (s *DownloadService) QueueNZB(ctx context.Context, media *domain.Media, nzb *domain.NZB) error {
	log.Info().Str("title", nzb.Title).Int64("trakt_id", media.TraktID).Msg("Queueing NZB")

	// Download NZB file with context and configured client
	req, err := http.NewRequestWithContext(ctx, "GET", nzb.Link, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download NZB: %w", err)
	}
	defer resp.Body.Close()

	nzbContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read NZB content: %w", err)
	}

	// Queue to NZBGet
	params := map[string]string{
		"Trakt": fmt.Sprintf("%d", media.TraktID),
	}

	downloadID, err := s.downloadClient.QueueDownload(
		ctx,
		nzbContent,
		nzb.Title+".nzb",
		s.cfg.Category,
		s.cfg.Priority,
		params,
	)
	if err != nil {
		return fmt.Errorf("failed to queue download: %w", err)
	}

	// Update media with download ID
	media.DownloadID = downloadID
	if err := s.mediaRepo.Update(ctx, media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	log.Info().Int64("download_id", downloadID).Msg("Successfully queued NZB")
	return nil
}

func (s *DownloadService) isInQueue(title string, queue []ports.DownloadQueueItem) bool {
	for _, item := range queue {
		if strings.Contains(item.Title, title) || strings.Contains(title, item.Title) {
			return true
		}
	}
	return false
}

func (s *DownloadService) isInHistory(downloadID int64, history []ports.DownloadHistoryItem) bool {
	if downloadID == 0 {
		return false
	}
	for _, item := range history {
		if item.ID == downloadID {
			return true
		}
	}
	return false
}
