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
