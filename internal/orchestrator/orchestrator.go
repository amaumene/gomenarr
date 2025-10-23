package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/core/services"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/amaumene/gomenarr/internal/platform/metrics"
	"github.com/rs/zerolog/log"
)

type Orchestrator struct {
	mediaSvc    *services.MediaService
	nzbSvc      *services.NZBService
	downloadSvc *services.DownloadService
	cleanupSvc  *services.CleanupService
	traktClient ports.TraktClient
	cfg         config.OrchestratorConfig
	metrics     *metrics.Metrics
}

func New(
	mediaSvc *services.MediaService,
	nzbSvc *services.NZBService,
	downloadSvc *services.DownloadService,
	cleanupSvc *services.CleanupService,
	traktClient ports.TraktClient,
	cfg config.OrchestratorConfig,
	m *metrics.Metrics,
) *Orchestrator {
	return &Orchestrator{
		mediaSvc:    mediaSvc,
		nzbSvc:      nzbSvc,
		downloadSvc: downloadSvc,
		cleanupSvc:  cleanupSvc,
		traktClient: traktClient,
		cfg:         cfg,
		metrics:     m,
	}
}

func (o *Orchestrator) Start(ctx context.Context) error {
	if !o.cfg.Enabled {
		log.Info().Msg("Orchestrator is disabled")
		return nil
	}

	log.Info().Dur("interval", o.cfg.Interval).Msg("Starting orchestrator")

	// Wait for startup delay
	if o.cfg.StartupDelay > 0 {
		log.Info().Dur("delay", o.cfg.StartupDelay).Msg("Waiting before first run")
		select {
		case <-time.After(o.cfg.StartupDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Start token refresh goroutine
	go o.tokenRefreshLoop(ctx)

	// Start main orchestration loop
	ticker := time.NewTicker(o.cfg.Interval)
	defer ticker.Stop()

	// Run immediately
	o.runCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Orchestrator stopped")
			return nil
		case <-ticker.C:
			o.runCycle(ctx)
		}
	}
}

func (o *Orchestrator) runCycle(ctx context.Context) {
	log.Info().Msg("Starting orchestrator cycle")
	start := time.Now()

	// 1. Sync media in parallel (movies and episodes are independent)
	var syncWg sync.WaitGroup
	syncWg.Add(2)

	go func() {
		defer syncWg.Done()
		if err := o.runTask(ctx, "sync_movies", func(ctx context.Context) error {
			return o.mediaSvc.SyncMovies(ctx)
		}); err != nil {
			log.Error().Err(err).Msg("Failed to sync movies")
		}
	}()

	go func() {
		defer syncWg.Done()
		if err := o.runTask(ctx, "sync_episodes", func(ctx context.Context) error {
			return o.mediaSvc.SyncEpisodes(ctx)
		}); err != nil {
			log.Error().Err(err).Msg("Failed to sync episodes")
		}
	}()

	// Wait for both sync tasks to complete before proceeding
	syncWg.Wait()

	// 2. Search for NZBs
	if err := o.runTask(ctx, "search_nzbs", func(ctx context.Context) error {
		return o.searchAllNZBs(ctx)
	}); err != nil {
		log.Error().Err(err).Msg("Failed to search NZBs")
	}

	// 3. Download media
	if err := o.runTask(ctx, "download_media", func(ctx context.Context) error {
		return o.downloadSvc.DownloadMedia(ctx)
	}); err != nil {
		log.Error().Err(err).Msg("Failed to download media")
	}

	// 4. Cleanup watched
	if err := o.runTask(ctx, "cleanup_watched", func(ctx context.Context) error {
		return o.cleanupSvc.CleanupWatched(ctx)
	}); err != nil {
		log.Error().Err(err).Msg("Failed to cleanup watched")
	}

	duration := time.Since(start)
	log.Info().Dur("duration", duration).Msg("Orchestrator cycle completed")
}

func (o *Orchestrator) runTask(ctx context.Context, taskName string, task func(context.Context) error) error {
	start := time.Now()
	log.Info().Str("task", taskName).Dur("timeout", o.cfg.TaskTimeout).Msg("Running task")

	// Create context with timeout
	taskCtx, cancel := context.WithTimeout(ctx, o.cfg.TaskTimeout)
	defer cancel()

	err := task(taskCtx)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		if err == context.DeadlineExceeded {
			status = "timeout"
			log.Error().Str("task", taskName).Dur("timeout", o.cfg.TaskTimeout).Msg("Task timed out")
		} else {
			status = "error"
		}
	}

	if o.metrics != nil && o.metrics.OrchestratorTasksTotal != nil {
		o.metrics.OrchestratorTasksTotal.WithLabelValues(taskName, status).Inc()
		o.metrics.OrchestratorTaskDuration.WithLabelValues(taskName).Observe(duration)
	}

	log.Info().Str("task", taskName).Str("status", status).Dur("duration", time.Duration(duration*float64(time.Second))).Msg("Task completed")
	return err
}

func (o *Orchestrator) searchAllNZBs(ctx context.Context) error {
	mediaList, err := o.mediaSvc.GetNotOnDisk(ctx)
	if err != nil {
		return err
	}

	log.Info().Int("count", len(mediaList)).Msg("Searching NZBs for media not on disk")

	// Use worker pool for parallel NZB searches (5 concurrent workers)
	const numWorkers = 5
	jobs := make(chan *domain.Media, len(mediaList))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for media := range jobs {
				if err := o.nzbSvc.SearchForMedia(ctx, media); err != nil {
					log.Error().
						Err(err).
						Int64("trakt_id", media.TraktID).
						Int("worker_id", workerID).
						Msg("Failed to search for media")
				}
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

	return nil
}

func (o *Orchestrator) tokenRefreshLoop(ctx context.Context) {
	interval := o.cfg.TokenRefreshInterval
	if interval <= 0 {
		interval = 1 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := o.traktClient.RefreshToken(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to refresh Trakt token")
			} else {
				log.Debug().Msg("Trakt token refreshed successfully")
			}
		}
	}
}
