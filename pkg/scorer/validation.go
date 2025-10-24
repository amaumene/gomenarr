package scorer

import (
	"math"
	"strings"
	"unicode"

	"github.com/agnivade/levenshtein"
	"github.com/amaumene/gomenarr/pkg/parser"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ValidationScore calculates the validation score (0-100)
// Based on title similarity, year match, and season/episode match
func ValidationScore(media MediaInfo, parsed *parser.ParsedInfo) int {
	var score float64

	// Title similarity (max 50 points)
	titleScore := calculateTitleSimilarity(media.Title, parsed.Title)
	score += titleScore * 50

	// Year match (max 30 points)
	yearScore := calculateYearScore(media.Year, parsed.Year)
	score += float64(yearScore)

	// Season/Episode match (max 20 points)
	if media.IsEpisode() {
		if parsed.Season == media.Season {
			score += 10
			// Season packs (Episode == 0) get full season/episode points
			if parsed.Episode == 0 {
				score += 10
			}
		}
		if parsed.Episode == media.Number {
			score += 10
		}
	}

	return int(math.Round(score))
}

// MediaInfo represents media information for scoring
type MediaInfo struct {
	Title  string
	Year   int64
	Season int64
	Number int64
}

func (m MediaInfo) IsEpisode() bool {
	return m.Season > 0 && m.Number > 0
}

// calculateTitleSimilarity calculates similarity between two titles (0.0 - 1.0)
func calculateTitleSimilarity(title1, title2 string) float64 {
	// Normalize titles
	t1 := normalizeTitle(title1)
	t2 := normalizeTitle(title2)

	if t1 == t2 {
		return 1.0
	}

	// Calculate Levenshtein distance
	distance := levenshtein.ComputeDistance(t1, t2)
	maxLen := max(len(t1), len(t2))

	if maxLen == 0 {
		return 0.0
	}

	// Convert distance to similarity
	similarity := 1.0 - (float64(distance) / float64(maxLen))
	return math.Max(0, similarity)
}

func normalizeTitle(title string) string {
	// Remove accents/diacritics first
	title = removeAccents(title)

	// Convert to lowercase
	title = strings.ToLower(title)

	// Remove special characters
	title = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			return r
		}
		return -1
	}, title)

	// Trim and collapse spaces
	title = strings.TrimSpace(title)
	title = strings.Join(strings.Fields(title), " ")

	return title
}

// removeAccents removes diacritical marks from Unicode text
// Examples: "Néro" -> "Nero", "Pokémon" -> "Pokemon", "café" -> "cafe"
func removeAccents(s string) string {
	// Transform using NFD (Canonical Decomposition) followed by removing combining marks
	// NFD separates base characters from their diacritical marks
	// Then we remove all marks (category Mn = nonspacing marks)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

// calculateYearScore calculates the year match score
func calculateYearScore(mediaYear, parsedYear int64) int {
	if parsedYear == 0 {
		return 0
	}

	diff := abs(mediaYear - parsedYear)
	switch diff {
	case 0:
		return 30
	case 1:
		return 20
	case 2:
		return 10
	default:
		return 0
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
