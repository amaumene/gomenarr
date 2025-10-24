package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
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

	// Track processed items in this run (prevents duplicates within same batch)
	processedSeasons := make(map[string]bool)
	queuedThisRun := make(map[string]bool)
	count := 0

	// Process media sequentially (no race conditions)
	for _, media := range mediaList {
		// Skip if already has download ID (already queued/downloaded)
		if media.DownloadID > 0 {
			log.Debug().
				Int64("trakt_id", media.TraktID).
				Int64("download_id", media.DownloadID).
				Msg("Already has download ID, skipping")
			continue
		}

		// Season pack deduplication check
		if media.IsEpisode() {
			seasonKey := fmt.Sprintf("%s_S%d", media.IMDB, media.Season)
			if processedSeasons[seasonKey] {
				log.Debug().Str("season_key", seasonKey).Msg("Skipping episode, season pack already processed")
				continue
			}
		}

		// Get best NZB - prefer season packs for episodes
		var nzb *domain.NZB

		if media.IsEpisode() && media.IMDB != "" {
			// Try to find season pack first
			nzb, err = s.nzbRepo.FindBestSeasonPack(ctx, media.IMDB, media.Season)
			if err == nil {
				log.Debug().
					Str("imdb", media.IMDB).
					Int64("season", media.Season).
					Str("title", nzb.Title).
					Msg("Found season pack for episode")
			}
		}

		// Fallback to episode-specific NZB if no season pack found
		if nzb == nil {
			nzb, err = s.nzbRepo.FindBestByTraktID(ctx, media.TraktID)
			if err != nil {
				log.Debug().Int64("trakt_id", media.TraktID).Msg("No NZB found for media")
				continue
			}
		}

		// Check if already queued in this run
		if queuedThisRun[nzb.Title] {
			log.Debug().Str("title", nzb.Title).Msg("Already queued in this run")
			continue
		}

		// Check if already in queue
		if s.isInQueue(nzb.Title, queue) {
			log.Debug().Str("title", nzb.Title).Msg("Already in queue")
			continue
		}

		// Check if already in history
		if s.isInHistory(media.DownloadID, history) {
			log.Debug().Int64("download_id", media.DownloadID).Msg("Already in history")
			continue
		}

		// Queue download
		if err := s.QueueNZB(ctx, media, nzb); err != nil {
			log.Error().Err(err).Str("title", nzb.Title).Msg("Failed to queue NZB")
			continue
		}

		// Mark as processed
		queuedThisRun[nzb.Title] = true
		count++

		// Mark season as processed if it's a season pack
		if media.IsEpisode() && nzb.IsSeasonPack {
			seasonKey := fmt.Sprintf("%s_S%d", media.IMDB, media.Season)
			processedSeasons[seasonKey] = true
			log.Info().Str("season_key", seasonKey).Msg("Marked season pack as processed")
		}
	}

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

// normalizeReleaseTitle normalizes a release title for comparison
// Removes quality indicators, codecs, sources, and group tags
// Keeps show name and season/episode information
func normalizeReleaseTitle(title string) string {
	// Remove .nzb extension
	title = strings.TrimSuffix(title, ".nzb")

	// Convert to uppercase for consistent matching
	normalized := strings.ToUpper(title)

	// Remove quality indicators (2160p, 1080p, 720p, 480p, 4K, UHD, etc.)
	qualityPattern := regexp.MustCompile(`(?i)\b(2160P|1080P|720P|480P|4K|UHD|HD|SD)\b`)
	normalized = qualityPattern.ReplaceAllString(normalized, "")

	// Remove codec indicators (H.265, H.264, x265, x264, HEVC, AVC, etc.)
	codecPattern := regexp.MustCompile(`(?i)\b(H\.?265|H\.?264|X265|X264|HEVC|AVC|VC-?1|XVID)\b`)
	normalized = codecPattern.ReplaceAllString(normalized, "")

	// Remove source indicators (WEB-DL, WEBDL, WEB, BluRay, Blu-ray, REMUX, HDTV, DVDRip, etc.)
	sourcePattern := regexp.MustCompile(`(?i)\b(WEB-?DL|WEBRIP|WEB|BLU-?RAY|BRRIP|REMUX|HDTV|DVDRIP|DVD)\b`)
	normalized = sourcePattern.ReplaceAllString(normalized, "")

	// Remove audio indicators (DDP5.1, DD5.1, Atmos, TrueHD, DTS, AAC, FLAC, etc.)
	audioPattern := regexp.MustCompile(`(?i)\b(DDP?A?[0-9.]+|ATMOS|TRUEHD|DTS(-?HD)?(-?MA)?|AAC|FLAC|LPCM|AC3)\b`)
	normalized = audioPattern.ReplaceAllString(normalized, "")

	// Remove HDR/color space indicators (HDR, HDR10, DV, DoVi, SDR, etc.)
	hdrPattern := regexp.MustCompile(`(?i)\b(HDR10\+?|HDR|DV|DOVI|SDR|10BIT)\b`)
	normalized = hdrPattern.ReplaceAllString(normalized, "")

	// Remove hybrid/repack/proper indicators
	flagPattern := regexp.MustCompile(`(?i)\b(HYBRID|REPACK|PROPER|RERIP)\b`)
	normalized = flagPattern.ReplaceAllString(normalized, "")

	// Remove group tags (everything after the last dash)
	dashIndex := strings.LastIndex(normalized, "-")
	if dashIndex > 0 {
		normalized = normalized[:dashIndex]
	}

	// Collapse multiple dots/spaces into single dot
	normalized = regexp.MustCompile(`[.\s]+`).ReplaceAllString(normalized, ".")

	// Trim leading/trailing dots
	normalized = strings.Trim(normalized, ".")

	return normalized
}

func (s *DownloadService) isInQueue(title string, queue []ports.DownloadQueueItem) bool {
	normalizedTitle := normalizeReleaseTitle(title)

	for _, item := range queue {
		normalizedItem := normalizeReleaseTitle(item.Title)
		if normalizedTitle == normalizedItem {
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
