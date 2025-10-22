package scorer

import (
	"github.com/amaumene/gomenarr/pkg/parser"
)

// QualityScore calculates the quality score (0-100)
// Quality-first approach: prioritizes source quality over codec efficiency
// Distribution: Source (50) + Resolution (30) + Codec (15) + Flags (5) = 100
func QualityScore(parsed *parser.ParsedInfo) int {
	score := 0

	// Source (max 50 points) - highest priority for lossless/high-quality sources
	score += sourceScore(parsed.Source)

	// Resolution (max 30 points)
	score += resolutionScore(parsed.Resolution)

	// Codec (max 15 points) - lower priority as it's about efficiency, not source quality
	score += codecScore(parsed.Codec)

	// Flags (max 5 points)
	if parsed.IsProper || parsed.IsRepack {
		score += 5
	}

	return score
}

func resolutionScore(resolution string) int {
	switch resolution {
	case "2160P":
		return 30 // 4K
	case "1080P":
		return 25 // Full HD
	case "720P":
		return 15 // HD
	case "480P":
		return 5 // SD
	default:
		return 0
	}
}

func sourceScore(source string) int {
	switch source {
	case "REMUX":
		return 60 // Lossless - highest quality (increased to ensure REMUX wins even with lower validation scores)
	case "BLURAY":
		return 35 // Physical media encode
	case "WEB-DL":
		return 25 // Direct web source
	case "HDTV":
		return 15 // Broadcast
	case "DVD":
		return 10 // Legacy
	default:
		return 0
	}
}

func codecScore(codec string) int {
	switch codec {
	case "X265":
		return 15 // Modern efficient (HEVC/H.265)
	case "X264":
		return 12 // Standard (AVC/H.264) - commonly used in REMUX
	case "XVID":
		return 5 // Legacy
	default:
		return 0
	}
}
