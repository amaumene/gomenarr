package utils

import (
	"bufio"
	"os"
	"strings"
)

// Blacklist holds blacklist terms for filtering NZB results
type Blacklist struct {
	terms []string
}

// LoadBlacklist loads blacklist terms from a file
func LoadBlacklist(path string) (*Blacklist, error) {
	// If file doesn't exist, return empty blacklist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Blacklist{terms: []string{}}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var terms []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		term := strings.TrimSpace(scanner.Text())
		if term != "" && !strings.HasPrefix(term, "#") {
			terms = append(terms, term)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &Blacklist{terms: terms}, nil
}

// IsBlacklisted checks if a title matches any blacklist term
// Returns (isBlacklisted, matchedTerm)
func (b *Blacklist) IsBlacklisted(title string) (bool, string) {
	titleLower := strings.ToLower(title)

	for _, term := range b.terms {
		termLower := strings.ToLower(term)
		if strings.Contains(titleLower, termLower) {
			return true, term
		}
	}

	return false, ""
}
