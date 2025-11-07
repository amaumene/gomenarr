package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// Trakt
	TraktClientID     string
	TraktClientSecret string
	TraktSyncDays     int // Days to look back for watched media (default: 3)

	// Newznab
	NewznabURL string
	NewznabKey string

	// TorBox
	TorBoxAPIKey string

	// Download
	DownloadTimeoutMinutes int // Minutes before a download is considered stuck (default: 30)

	// Server
	ServerPort string

	// Paths
	TokenFile     string // $CONFIG_DIR/token.json
	BlacklistFile string // $CONFIG_DIR/blacklist.txt
	DatabaseFile  string // $CONFIG_DIR/gomenarr.db

	// Logging
	LogLevel string
}

// Load loads configuration from environment variables and .env file
func Load() (*Config, error) {
	// Setup viper FIRST to load .env file
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	// Load .env file if it exists (ignore if not found)
	_ = viper.ReadInConfig()

	// Set defaults
	viper.SetDefault("TRAKT_SYNC_DAYS", 3)
	viper.SetDefault("DOWNLOAD_TIMEOUT_MINUTES", 30)
	viper.SetDefault("SERVER_PORT", "8080")
	viper.SetDefault("LOG_LEVEL", "info")

	// NOW read CONFIG_DIR from viper (which has loaded .env file)
	configDir := viper.GetString("CONFIG_DIR")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir = filepath.Join(homeDir, ".config", "gomenarr")
	} else {
		// Convert relative path to absolute path
		absPath, err := filepath.Abs(configDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for CONFIG_DIR: %w", err)
		}
		configDir = absPath
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	config := &Config{
		// Trakt
		TraktClientID:     viper.GetString("TRAKT_CLIENT_ID"),
		TraktClientSecret: viper.GetString("TRAKT_CLIENT_SECRET"),
		TraktSyncDays:     viper.GetInt("TRAKT_SYNC_DAYS"),

		// Newznab
		NewznabURL: viper.GetString("NEWZNAB_URL"),
		NewznabKey: viper.GetString("NEWZNAB_KEY"),

		// TorBox
		TorBoxAPIKey: viper.GetString("TORBOX_API_KEY"),

		// Download
		DownloadTimeoutMinutes: viper.GetInt("DOWNLOAD_TIMEOUT_MINUTES"),

		// Server
		ServerPort: viper.GetString("SERVER_PORT"),

		// Paths
		TokenFile:     filepath.Join(configDir, "token.json"),
		BlacklistFile: filepath.Join(configDir, "blacklist.txt"),
		DatabaseFile:  filepath.Join(configDir, "gomenarr.db"),

		// Logging
		LogLevel: viper.GetString("LOG_LEVEL"),
	}

	// Validate required fields
	if config.TraktClientID == "" {
		return nil, fmt.Errorf("TRAKT_CLIENT_ID is required")
	}
	if config.TraktClientSecret == "" {
		return nil, fmt.Errorf("TRAKT_CLIENT_SECRET is required")
	}
	if config.NewznabURL == "" {
		return nil, fmt.Errorf("NEWZNAB_URL is required")
	}
	if config.NewznabKey == "" {
		return nil, fmt.Errorf("NEWZNAB_KEY is required")
	}
	if config.TorBoxAPIKey == "" {
		return nil, fmt.Errorf("TORBOX_API_KEY is required")
	}

	return config, nil
}
