package parser

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedInfo represents parsed release information
type ParsedInfo struct {
	Title      string
	Year       int64
	Season     int64
	Episode    int64
	Resolution string
	Source     string
	Codec      string
	IsProper   bool
	IsRepack   bool
}

var (
	yearRegex       = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	seasonEpRegex   = regexp.MustCompile(`[Ss](\d{1,2})[Ee](\d{1,2})`)
	seasonRegex     = regexp.MustCompile(`[Ss]eason[\s\.]?(\d{1,2})`)
	resolutionRegex = regexp.MustCompile(`(?i)(2160p|1080p|720p|480p|4k|uhd)`)
	sourceRegex     = regexp.MustCompile(`(?i)(REMUX|BluRay|Blu-Ray|BRRip|WEB-DL|WEBDL|WEBRip|HDTV|DVDRip|DVD)`)
	codecRegex      = regexp.MustCompile(`(?i)(x265|H\.?265|HEVC|x264|H\.?264|AVC|XviD)`)
)

// Parse parses a release title and extracts structured information
func Parse(title string) *ParsedInfo {
	parsed := &ParsedInfo{}

	// Clean title for parsing
	cleanTitle := title

	// Extract year
	if matches := yearRegex.FindStringSubmatch(title); len(matches) > 0 {
		year, _ := strconv.ParseInt(matches[0], 10, 64)
		parsed.Year = year
		cleanTitle = strings.Replace(cleanTitle, matches[0], "", 1)
	}

	// Extract season and episode (S01E02 format)
	if matches := seasonEpRegex.FindStringSubmatch(title); len(matches) >= 3 {
		season, _ := strconv.ParseInt(matches[1], 10, 64)
		episode, _ := strconv.ParseInt(matches[2], 10, 64)
		parsed.Season = season
		parsed.Episode = episode
	} else if matches := seasonRegex.FindStringSubmatch(title); len(matches) >= 2 {
		// Season pack format
		season, _ := strconv.ParseInt(matches[1], 10, 64)
		parsed.Season = season
		parsed.Episode = 0
	}

	// Extract resolution
	if matches := resolutionRegex.FindStringSubmatch(title); len(matches) > 0 {
		parsed.Resolution = normalizeResolution(matches[0])
	}

	// Extract source
	// Pass the full title to normalizeSource so it can check for REMUX
	// which often appears alongside BluRay (e.g., "BluRay.Remux")
	parsed.Source = normalizeSource(title)

	// Extract codec
	if matches := codecRegex.FindStringSubmatch(title); len(matches) > 0 {
		parsed.Codec = normalizeCodec(matches[0])
	}

	// Check for PROPER or REPACK flags
	titleUpper := strings.ToUpper(title)
	parsed.IsProper = strings.Contains(titleUpper, "PROPER")
	parsed.IsRepack = strings.Contains(titleUpper, "REPACK")

	// Extract title (everything before quality indicators)
	titleParts := strings.FieldsFunc(cleanTitle, func(r rune) bool {
		return r == '.' || r == ' ' || r == '-' || r == '_'
	})
	
	var titleWords []string
	for _, part := range titleParts {
		// Stop at quality indicators
		upper := strings.ToUpper(part)
		if strings.Contains(upper, "1080") || strings.Contains(upper, "720") || 
		   strings.Contains(upper, "BLURAY") || strings.Contains(upper, "WEB") ||
		   strings.Contains(upper, "X264") || strings.Contains(upper, "X265") {
			break
		}
		if len(part) > 0 && !isNumeric(part) {
			titleWords = append(titleWords, part)
		}
	}
	parsed.Title = strings.Join(titleWords, " ")

	return parsed
}

func normalizeResolution(res string) string {
	res = strings.ToUpper(strings.TrimSpace(res))
	switch {
	case strings.Contains(res, "2160"), strings.Contains(res, "4K"), strings.Contains(res, "UHD"):
		return "2160P"
	case strings.Contains(res, "1080"):
		return "1080P"
	case strings.Contains(res, "720"):
		return "720P"
	case strings.Contains(res, "480"):
		return "480P"
	default:
		return res
	}
}

func normalizeSource(source string) string {
	source = strings.ToUpper(strings.TrimSpace(source))
	switch {
	case strings.Contains(source, "REMUX"):
		return "REMUX"
	case strings.Contains(source, "BLURAY"), strings.Contains(source, "BLU-RAY"), strings.Contains(source, "BRRIP"):
		return "BLURAY"
	case strings.Contains(source, "WEB-DL"), strings.Contains(source, "WEBDL"), strings.Contains(source, "WEBRIP"), strings.Contains(source, "WEB"):
		return "WEB-DL"
	case strings.Contains(source, "HDTV"):
		return "HDTV"
	case strings.Contains(source, "DVDRIP"), strings.Contains(source, "DVD"):
		return "DVD"
	default:
		return source
	}
}

func normalizeCodec(codec string) string {
	codec = strings.ToUpper(strings.TrimSpace(codec))
	// Remove dots for easier matching (H.265 -> H265)
	codec = strings.ReplaceAll(codec, ".", "")

	switch {
	case strings.Contains(codec, "X265"), strings.Contains(codec, "HEVC"), strings.Contains(codec, "H265"):
		return "X265"
	case strings.Contains(codec, "X264"), strings.Contains(codec, "H264"), strings.Contains(codec, "AVC"):
		return "X264"
	case strings.Contains(codec, "XVID"):
		return "XVID"
	default:
		return codec
	}
}

// IsSeasonPack checks if a title represents a season pack
func IsSeasonPack(title string) bool {
	titleUpper := strings.ToUpper(title)
	
	// Must have season notation
	hasSeason := strings.Contains(titleUpper, "SEASON") ||
		regexp.MustCompile(`S\d{1,2}[^E]`).MatchString(titleUpper)
	
	// Must NOT have episode notation
	hasEpisode := strings.Contains(titleUpper, "E0") ||
		strings.Contains(titleUpper, "E1") ||
		strings.Contains(titleUpper, "E2") ||
		strings.Contains(titleUpper, "E3") ||
		strings.Contains(titleUpper, "X0") ||
		strings.Contains(titleUpper, "X1") ||
		strings.Contains(titleUpper, "X2") ||
		strings.Contains(titleUpper, "X3")
	
	return hasSeason && !hasEpisode
}

func isNumeric(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}
