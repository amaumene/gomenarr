package services

import (
	"context"
	"fmt"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/amaumene/gomenarr/pkg/parser"
	"github.com/amaumene/gomenarr/pkg/scorer"
	"github.com/rs/zerolog/log"
)

type NZBService struct {
	repo       ports.NZBRepository
	mediaRepo  ports.MediaRepository
	searcher   ports.NZBSearcher
	blacklist  *scorer.Blacklist
	cfg        config.DownloadConfig
}

func NewNZBService(
	repo ports.NZBRepository,
	mediaRepo ports.MediaRepository,
	searcher ports.NZBSearcher,
	blacklist *scorer.Blacklist,
	cfg config.DownloadConfig,
) *NZBService {
	return &NZBService{
		repo:      repo,
		mediaRepo: mediaRepo,
		searcher:  searcher,
		blacklist: blacklist,
		cfg:       cfg,
	}
}

func (s *NZBService) SearchForMedia(ctx context.Context, media *domain.Media) error {
	log.Info().Int64("trakt_id", media.TraktID).Str("title", media.Title).Msg("Searching for media")

	var results []ports.NewsnabResult
	var err error

	if media.IsMovie() {
		results, err = s.searcher.SearchMovie(ctx, media.IMDB)
	} else if media.IsEpisode() {
		// Try season pack first
		seasonResults, err := s.searcher.SearchSeasonPack(ctx, media.IMDB, media.Season)
		if err != nil {
			log.Error().Err(err).Msg("Failed to search season pack")
		} else {
			// Filter for valid season packs
			validPacks := make([]ports.NewsnabResult, 0)
			for _, r := range seasonResults {
				if parser.IsSeasonPack(r.Title) && !s.blacklist.Contains(r.Title) {
					validPacks = append(validPacks, r)
				}
			}

			if len(validPacks) > 0 {
				log.Info().Int("count", len(validPacks)).Msg("Found valid season packs")
				results = validPacks
			}
		}

		// Fallback to single episode search
		if len(results) == 0 {
			results, err = s.searcher.SearchEpisode(ctx, media.IMDB, media.Season, media.Number)
		}
	} else {
		return fmt.Errorf("invalid media type")
	}

	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	log.Info().Int("count", len(results)).Msg("Found NZB results")

	// Validate and score
	return s.ValidateAndScore(ctx, media, results)
}

func (s *NZBService) ValidateAndScore(ctx context.Context, media *domain.Media, results []ports.NewsnabResult) error {
	count := 0
	filtered := struct {
		blacklisted    int
		validationFail int
		qualityFail    int
		totalFail      int
	}{}

	log.Debug().
		Int("result_count", len(results)).
		Int64("trakt_id", media.TraktID).
		Str("title", media.Title).
		Msg("Validating and scoring NZB results")

	// Track best candidate for fallback
	var bestCandidate *domain.NZB

	for _, result := range results {
		// Check blacklist - always skip blacklisted items, even for fallback
		if s.blacklist.Contains(result.Title) {
			log.Debug().
				Str("release", result.Title).
				Msg("Filtered: Blacklisted")
			filtered.blacklisted++
			continue
		}

		// Parse release title
		parsed := parser.Parse(result.Title)

		// Calculate scores
		mediaInfo := scorer.MediaInfo{
			Title:  media.Title,
			Year:   media.Year,
			Season: media.Season,
			Number: media.Number,
		}
		validationScore := scorer.ValidationScore(mediaInfo, parsed)
		qualityScore := scorer.QualityScore(parsed)
		totalScore := validationScore + qualityScore

		// Create NZB candidate (we may need this for fallback)
		nzb := &domain.NZB{
			TraktID:         media.TraktID,
			IMDB:            media.IMDB,
			Link:            result.Link,
			Length:          result.Size,
			Title:           result.Title,
			ParsedTitle:     parsed.Title,
			ParsedYear:      parsed.Year,
			ParsedSeason:    parsed.Season,
			ParsedEpisode:   parsed.Episode,
			Resolution:      parsed.Resolution,
			Source:          parsed.Source,
			Codec:           parsed.Codec,
			ValidationScore: validationScore,
			QualityScore:    qualityScore,
			TotalScore:      totalScore,
		}

		// Track best candidate by total score (for fallback)
		if bestCandidate == nil || totalScore > bestCandidate.TotalScore {
			bestCandidate = nzb
		}

		// Check thresholds
		if validationScore < s.cfg.MinValidationScore {
			log.Debug().
				Str("release", result.Title).
				Int("validation_score", validationScore).
				Int("min_required", s.cfg.MinValidationScore).
				Msg("Filtered: Validation score too low")
			filtered.validationFail++
			continue
		}

		if qualityScore < s.cfg.MinQualityScore {
			log.Debug().
				Str("release", result.Title).
				Int("quality_score", qualityScore).
				Int("min_required", s.cfg.MinQualityScore).
				Msg("Filtered: Quality score too low")
			filtered.qualityFail++
			continue
		}

		if totalScore < s.cfg.MinTotalScore {
			log.Debug().
				Str("release", result.Title).
				Int("total_score", totalScore).
				Int("min_required", s.cfg.MinTotalScore).
				Msg("Filtered: Total score too low")
			filtered.totalFail++
			continue
		}

		log.Info().
			Str("release", result.Title).
			Int("validation_score", validationScore).
			Int("quality_score", qualityScore).
			Int("total_score", totalScore).
			Msg("Accepted NZB result")

		// For season packs, check if we already have one stored for this show/season
		if nzb.IsSeasonPack() && nzb.IMDB != "" {
			existing, err := s.repo.FindSeasonPackByIMDB(ctx, nzb.IMDB, nzb.ParsedSeason)
			if err == nil && existing != nil {
				log.Debug().
					Str("release", result.Title).
					Str("imdb", nzb.IMDB).
					Int64("season", nzb.ParsedSeason).
					Str("existing_release", existing.Title).
					Msg("Season pack already exists - skipping duplicate")
				continue
			}
		}

		if err := s.repo.Create(ctx, nzb); err != nil {
			log.Error().Err(err).Str("title", result.Title).Msg("Failed to create NZB")
			continue
		}
		count++
	}

	// Fallback logic: if nothing passed validation and we have a best candidate, store it
	if count == 0 && bestCandidate != nil {
		// Check for duplicate season pack before storing fallback
		shouldStore := true
		if bestCandidate.IsSeasonPack() && bestCandidate.IMDB != "" {
			existing, err := s.repo.FindSeasonPackByIMDB(ctx, bestCandidate.IMDB, bestCandidate.ParsedSeason)
			if err == nil && existing != nil {
				log.Debug().
					Str("release", bestCandidate.Title).
					Str("imdb", bestCandidate.IMDB).
					Int64("season", bestCandidate.ParsedSeason).
					Str("existing_release", existing.Title).
					Msg("Season pack already exists - skipping fallback duplicate")
				shouldStore = false
			}
		}

		if shouldStore {
			log.Warn().
				Str("release", bestCandidate.Title).
				Int("validation_score", bestCandidate.ValidationScore).
				Int("quality_score", bestCandidate.QualityScore).
				Int("total_score", bestCandidate.TotalScore).
				Int("total_results", len(results)).
				Int("non_blacklisted", len(results)-filtered.blacklisted).
				Msg("No releases passed thresholds - storing best candidate as fallback")

			if err := s.repo.Create(ctx, bestCandidate); err != nil {
				log.Error().Err(err).Str("title", bestCandidate.Title).Msg("Failed to create fallback NZB")
			} else {
				count = 1
			}
		}
	}

	log.Info().
		Int("stored", count).
		Int("total_results", len(results)).
		Int("blacklisted", filtered.blacklisted).
		Int("validation_fail", filtered.validationFail).
		Int("quality_fail", filtered.qualityFail).
		Int("total_score_fail", filtered.totalFail).
		Msg("NZB validation and scoring complete")

	return nil
}

func (s *NZBService) GetBestNZB(ctx context.Context, traktID int64) (*domain.NZB, error) {
	return s.repo.FindBestByTraktID(ctx, traktID)
}

func (s *NZBService) MarkAsFailed(ctx context.Context, title string) error {
	return s.repo.MarkAsFailedByTitle(ctx, title)
}

func (s *NZBService) GetAll(ctx context.Context) ([]*domain.NZB, error) {
	return s.repo.FindAll(ctx)
}
