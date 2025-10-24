package config

import (
	"os"
	"testing"
)

func TestLoadWithEnvVars(t *testing.T) {
	// Set test environment variables
	testEnvVars := map[string]string{
		"GOMENARR_NZBGET_URL":      "http://localhost:6789",
		"GOMENARR_NZBGET_USERNAME": "testuser",
		"GOMENARR_NZBGET_PASSWORD": "testpass",
		"GOMENARR_TRAKT_CLIENT_ID": "testclientid",
		"GOMENARR_TRAKT_CLIENT_SECRET": "testclientsecret",
		"GOMENARR_NEWSNAB_URL":     "https://api.nzbgeek.info",
		"GOMENARR_NEWSNAB_API_KEY": "testapikey",
		"GOMENARR_SERVER_PORT":     "8080",
	}

	// Set environment variables
	for key, value := range testEnvVars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	// Load configuration
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify NZBGet configuration from env vars
	if cfg.NZBGet.URL != "http://localhost:6789" {
		t.Errorf("NZBGet.URL = %v, want %v", cfg.NZBGet.URL, "http://localhost:6789")
	}
	if cfg.NZBGet.Username != "testuser" {
		t.Errorf("NZBGet.Username = %v, want %v", cfg.NZBGet.Username, "testuser")
	}
	if cfg.NZBGet.Password != "testpass" {
		t.Errorf("NZBGet.Password = %v, want %v", cfg.NZBGet.Password, "testpass")
	}

	// Verify Trakt configuration from env vars
	if cfg.Trakt.ClientID != "testclientid" {
		t.Errorf("Trakt.ClientID = %v, want %v", cfg.Trakt.ClientID, "testclientid")
	}
	if cfg.Trakt.ClientSecret != "testclientsecret" {
		t.Errorf("Trakt.ClientSecret = %v, want %v", cfg.Trakt.ClientSecret, "testclientsecret")
	}

	// Verify Newsnab configuration from env vars
	if cfg.Newsnab.URL != "https://api.nzbgeek.info" {
		t.Errorf("Newsnab.URL = %v, want %v", cfg.Newsnab.URL, "https://api.nzbgeek.info")
	}
	if cfg.Newsnab.APIKey != "testapikey" {
		t.Errorf("Newsnab.APIKey = %v, want %v", cfg.Newsnab.APIKey, "testapikey")
	}

	// Verify Server configuration from env vars
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %v, want %v", cfg.Server.Port, 8080)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Make sure no relevant env vars are set
	envVars := []string{
		"GOMENARR_NZBGET_URL",
		"GOMENARR_TRAKT_CLIENT_ID",
		"GOMENARR_SERVER_PORT",
		"GOMENARR_DATA_DIR",
		"GOMENARR_DATABASE_PATH",
	}
	for _, key := range envVars {
		os.Unsetenv(key)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify defaults are applied
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %v, want default %v", cfg.Server.Port, 3000)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %v, want default %v", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.NZBGet.Category != "trakt" {
		t.Errorf("NZBGet.Category = %v, want default %v", cfg.NZBGet.Category, "trakt")
	}
}

func TestPathNormalization(t *testing.T) {
	// Create temp directories for absolute path tests
	tempDir := t.TempDir()

	tests := []struct {
		name             string
		envDataDir       string
		envDatabasePath  string
		envBlacklistFile string
		envTokenFile     string
		wantDatabasePath string
		wantBlacklist    string
		wantToken        string
	}{
		{
			name:             "Default paths - all defaults should use data dir",
			envDataDir:       "",
			envDatabasePath:  "",
			envBlacklistFile: "",
			envTokenFile:     "",
			wantDatabasePath: "data/gomenarr.db",     // Normalized from ./data/gomenarr.db
			wantBlacklist:    "data/blacklist.txt",   // Normalized from ./data/blacklist.txt
			wantToken:        "data/token.json",      // Normalized from ./data/token.json
		},
		{
			name:             "Custom data dir - absolute path (using temp dir)",
			envDataDir:       tempDir + "/data",
			envDatabasePath:  "",
			envBlacklistFile: "",
			envTokenFile:     "",
			wantDatabasePath: tempDir + "/data/gomenarr.db",
			wantBlacklist:    tempDir + "/data/blacklist.txt",
			wantToken:        tempDir + "/data/token.json",
		},
		{
			name:             "Custom data dir - relative path",
			envDataDir:       "mydata",
			envDatabasePath:  "",
			envBlacklistFile: "",
			envTokenFile:     "",
			wantDatabasePath: "mydata/gomenarr.db",
			wantBlacklist:    "mydata/blacklist.txt",
			wantToken:        "mydata/token.json",
		},
		{
			name:             "Explicit database path - absolute - should not normalize",
			envDataDir:       tempDir + "/data",
			envDatabasePath:  tempDir + "/custom/path/db.db",
			envBlacklistFile: "",
			envTokenFile:     "",
			wantDatabasePath: tempDir + "/custom/path/db.db",
			wantBlacklist:    tempDir + "/data/blacklist.txt",
			wantToken:        tempDir + "/data/token.json",
		},
		{
			name:             "Explicit database path - custom relative - should not normalize",
			envDataDir:       tempDir + "/data2",
			envDatabasePath:  "custom/db.db",
			envBlacklistFile: "",
			envTokenFile:     "",
			wantDatabasePath: "custom/db.db",
			wantBlacklist:    tempDir + "/data2/blacklist.txt",
			wantToken:        tempDir + "/data2/token.json",
		},
		{
			name:             "All custom absolute paths",
			envDataDir:       tempDir + "/var/lib/app",
			envDatabasePath:  tempDir + "/opt/db/app.db",
			envBlacklistFile: tempDir + "/etc/app/blacklist.txt",
			envTokenFile:     tempDir + "/etc/app/token.json",
			wantDatabasePath: tempDir + "/opt/db/app.db",
			wantBlacklist:    tempDir + "/etc/app/blacklist.txt",
			wantToken:        tempDir + "/etc/app/token.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars first
			envVars := []string{
				"GOMENARR_DATA_DIR",
				"GOMENARR_DATABASE_PATH",
				"GOMENARR_DATA_BLACKLIST_FILE",
				"GOMENARR_DATA_TOKEN_FILE",
			}
			for _, key := range envVars {
				os.Unsetenv(key)
			}

			// Set test-specific env vars
			if tt.envDataDir != "" {
				os.Setenv("GOMENARR_DATA_DIR", tt.envDataDir)
				defer os.Unsetenv("GOMENARR_DATA_DIR")
			}
			if tt.envDatabasePath != "" {
				os.Setenv("GOMENARR_DATABASE_PATH", tt.envDatabasePath)
				defer os.Unsetenv("GOMENARR_DATABASE_PATH")
			}
			if tt.envBlacklistFile != "" {
				os.Setenv("GOMENARR_DATA_BLACKLIST_FILE", tt.envBlacklistFile)
				defer os.Unsetenv("GOMENARR_DATA_BLACKLIST_FILE")
			}
			if tt.envTokenFile != "" {
				os.Setenv("GOMENARR_DATA_TOKEN_FILE", tt.envTokenFile)
				defer os.Unsetenv("GOMENARR_DATA_TOKEN_FILE")
			}

			// Load config
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			// Verify paths
			if cfg.Database.Path != tt.wantDatabasePath {
				t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, tt.wantDatabasePath)
			}
			if cfg.Data.BlacklistFile != tt.wantBlacklist {
				t.Errorf("Data.BlacklistFile = %q, want %q", cfg.Data.BlacklistFile, tt.wantBlacklist)
			}
			if cfg.Data.TokenFile != tt.wantToken {
				t.Errorf("Data.TokenFile = %q, want %q", cfg.Data.TokenFile, tt.wantToken)
			}
		})
	}
}
