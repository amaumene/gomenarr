package torbox

import (
	"fmt"

	"github.com/amaumene/gomenarr/internal/config"
	"github.com/sirupsen/logrus"
)

// Client wraps the TorBox SDK
type Client struct {
	apiKey string
	logger *logrus.Logger
}

// NewClient creates a new TorBox client
func NewClient(cfg *config.Config, logger *logrus.Logger) (*Client, error) {
	if cfg.TorBoxAPIKey == "" {
		return nil, fmt.Errorf("TorBox API key is required")
	}

	return &Client{
		apiKey: cfg.TorBoxAPIKey,
		logger: logger,
	}, nil
}
