package utils

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/amaumene/gomenarr/internal/models"
)

// DetermineQuality parses a title string and determines the quality tier
func DetermineQuality(title string) models.Quality {
	titleLower := strings.ToLower(title)

	if strings.Contains(titleLower, "remux") {
		return models.QualityREMUX
	}

	if strings.Contains(titleLower, "web-dl") ||
		strings.Contains(titleLower, "webdl") ||
		strings.Contains(titleLower, "web dl") {
		return models.QualityWEBDL
	}

	return models.QualityOther
}

// RankByQuality sorts NZBs by:
// 1. Season packs (preferred over individual episodes for favorites)
// 2. Quality (REMUX > WEB-DL > OTHER)
// 3. Size (larger is better)
func RankByQuality(nzbs []*models.NZB) []*models.NZB {
	sorted := make([]*models.NZB, len(nzbs))
	copy(sorted, nzbs)

	sort.Slice(sorted, func(i, j int) bool {
		// PRIORITY 1: Season packs are preferred over individual episodes
		if sorted[i].IsSeasonPack != sorted[j].IsSeasonPack {
			return sorted[i].IsSeasonPack // Season pack wins
		}

		// PRIORITY 2: Compare by quality
		qualityI := qualityValue(sorted[i].Quality)
		qualityJ := qualityValue(sorted[j].Quality)

		if qualityI != qualityJ {
			return qualityI > qualityJ // Higher quality first
		}

		// PRIORITY 3: If quality is the same, larger size wins
		return sorted[i].Size > sorted[j].Size
	})

	return sorted
}

// qualityValue assigns a numeric value to each quality tier for comparison
func qualityValue(q models.Quality) int {
	switch q {
	case models.QualityREMUX:
		return 3
	case models.QualityWEBDL:
		return 2
	case models.QualityOther:
		return 1
	default:
		return 0
	}
}

var yearRegex = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)

// ExtractYear extracts a 4-digit year from an NZB title
// Returns 0 if no year is found
// Matches years like: (2009), 2009, [2009], etc.
func ExtractYear(title string) int {
	matches := yearRegex.FindStringSubmatch(title)
	if len(matches) > 1 {
		year, err := strconv.Atoi(matches[1])
		if err == nil {
			return year
		}
	}
	return 0
}
