package scorer

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

// Blacklist manages blacklisted words
type Blacklist struct {
	words []string
	mu    sync.RWMutex
}

// NewBlacklist creates a new blacklist
func NewBlacklist() *Blacklist {
	return &Blacklist{
		words: make([]string, 0),
	}
}

// Load loads blacklist from file
func (b *Blacklist) Load(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, that's okay
			return nil
		}
		return err
	}
	defer file.Close()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.words = make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" && !strings.HasPrefix(word, "#") {
			b.words = append(b.words, strings.ToLower(word))
		}
	}

	return scanner.Err()
}

// Contains checks if the title contains any blacklisted word
func (b *Blacklist) Contains(title string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	titleLower := strings.ToLower(title)
	for _, word := range b.words {
		if strings.Contains(titleLower, word) {
			return true
		}
	}
	return false
}

// Words returns all blacklisted words
func (b *Blacklist) Words() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	words := make([]string, len(b.words))
	copy(words, b.words)
	return words
}
