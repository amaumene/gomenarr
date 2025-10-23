package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Data           DataConfig           `mapstructure:"data"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Cache          CacheConfig          `mapstructure:"cache"`
	Logging        LoggingConfig        `mapstructure:"logging"`
	Metrics        MetricsConfig        `mapstructure:"metrics"`
	Tracing        TracingConfig        `mapstructure:"tracing"`
	Trakt          TraktConfig          `mapstructure:"trakt"`
	Newsnab        NewsnabConfig        `mapstructure:"newsnab"`
	NZBGet         NZBGetConfig         `mapstructure:"nzbget"`
	Download       DownloadConfig       `mapstructure:"download"`
	Orchestrator   OrchestratorConfig   `mapstructure:"orchestrator"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Retry          RetryConfig          `mapstructure:"retry"`
	RateLimit      RateLimitConfig      `mapstructure:"rate_limit"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

type DataConfig struct {
	Dir           string `mapstructure:"dir"`
	BlacklistFile string `mapstructure:"blacklist_file"`
	TokenFile     string `mapstructure:"token_file"`
}

type DatabaseConfig struct {
	Path            string        `mapstructure:"path"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	WALMode         bool          `mapstructure:"wal_mode"`
}

type CacheConfig struct {
	DefaultExpiration time.Duration `mapstructure:"default_expiration"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type MetricsConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Namespace string `mapstructure:"namespace"`
}

type TracingConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	Endpoint    string `mapstructure:"endpoint"`
	ServiceName string `mapstructure:"service_name"`
}

type TraktConfig struct {
	ClientID              string        `mapstructure:"client_id"`
	ClientSecret          string        `mapstructure:"client_secret"`
	RedirectURI           string        `mapstructure:"redirect_uri"`
	Timeout               time.Duration `mapstructure:"timeout"`
	FavoritesEpisodeLimit int           `mapstructure:"favorites_episode_limit"`
}

type NewsnabConfig struct {
	URL        string        `mapstructure:"url"`
	APIKey     string        `mapstructure:"api_key"`
	Timeout    time.Duration `mapstructure:"timeout"`
	MaxResults int           `mapstructure:"max_results"`
}

type NZBGetConfig struct {
	URL      string        `mapstructure:"url"`
	Username string        `mapstructure:"username"`
	Password string        `mapstructure:"password"`
	Timeout  time.Duration `mapstructure:"timeout"`
	Category string        `mapstructure:"category"`
	Priority int           `mapstructure:"priority"`
}

type DownloadConfig struct {
	MinValidationScore int  `mapstructure:"min_validation_score"`
	MinQualityScore    int  `mapstructure:"min_quality_score"`
	MinTotalScore      int  `mapstructure:"min_total_score"`
	CleanupWatchedDays int  `mapstructure:"cleanup_watched_days"`
	DeleteFiles        bool `mapstructure:"delete_files"`
}

type OrchestratorConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	Interval             time.Duration `mapstructure:"interval"`
	StartupDelay         time.Duration `mapstructure:"startup_delay"`
	TokenRefreshInterval time.Duration `mapstructure:"token_refresh_interval"`
	TaskTimeout          time.Duration `mapstructure:"task_timeout"`
}

type CircuitBreakerConfig struct {
	MaxRequests uint32        `mapstructure:"max_requests"`
	Interval    time.Duration `mapstructure:"interval"`
	Timeout     time.Duration `mapstructure:"timeout"`
}

type RetryConfig struct {
	MaxRetries      int           `mapstructure:"max_retries"`
	InitialInterval time.Duration `mapstructure:"initial_interval"`
	MaxInterval     time.Duration `mapstructure:"max_interval"`
	Multiplier      float64       `mapstructure:"multiplier"`
}

type RateLimitConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerSecond int  `mapstructure:"requests_per_second"`
	Burst             int  `mapstructure:"burst"`
}

// Load loads configuration from file and environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, use defaults and env vars
	}

	// Environment variables
	v.SetEnvPrefix("GOMENARR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicitly bind all environment variables for Unmarshal to work
	bindEnvs(v)

	// Unmarshal
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Normalize paths relative to data.dir
	cfg.normalizePaths()

	return &cfg, nil
}

// bindEnvs explicitly binds all config keys to environment variables.
// This is required for Unmarshal() to work with environment variables.
// AutomaticEnv() alone doesn't work with Unmarshal() for nested structs.
func bindEnvs(v *viper.Viper) {
	// Server
	v.BindEnv("server.host")
	v.BindEnv("server.port")
	v.BindEnv("server.read_timeout")
	v.BindEnv("server.write_timeout")
	v.BindEnv("server.idle_timeout")

	// Data
	v.BindEnv("data.dir")
	v.BindEnv("data.blacklist_file")
	v.BindEnv("data.token_file")

	// Database
	v.BindEnv("database.path")
	v.BindEnv("database.max_open_conns")
	v.BindEnv("database.max_idle_conns")
	v.BindEnv("database.conn_max_lifetime")
	v.BindEnv("database.wal_mode")

	// Cache
	v.BindEnv("cache.default_expiration")
	v.BindEnv("cache.cleanup_interval")

	// Logging
	v.BindEnv("logging.level")
	v.BindEnv("logging.format")
	v.BindEnv("logging.output")

	// Metrics
	v.BindEnv("metrics.enabled")
	v.BindEnv("metrics.namespace")

	// Tracing
	v.BindEnv("tracing.enabled")
	v.BindEnv("tracing.endpoint")
	v.BindEnv("tracing.service_name")

	// Trakt
	v.BindEnv("trakt.client_id")
	v.BindEnv("trakt.client_secret")
	v.BindEnv("trakt.redirect_uri")
	v.BindEnv("trakt.timeout")
	v.BindEnv("trakt.favorites_episode_limit")

	// Newsnab
	v.BindEnv("newsnab.url")
	v.BindEnv("newsnab.api_key")
	v.BindEnv("newsnab.timeout")
	v.BindEnv("newsnab.max_results")

	// NZBGet
	v.BindEnv("nzbget.url")
	v.BindEnv("nzbget.username")
	v.BindEnv("nzbget.password")
	v.BindEnv("nzbget.timeout")
	v.BindEnv("nzbget.category")
	v.BindEnv("nzbget.priority")

	// Download
	v.BindEnv("download.min_validation_score")
	v.BindEnv("download.min_quality_score")
	v.BindEnv("download.min_total_score")
	v.BindEnv("download.cleanup_watched_days")
	v.BindEnv("download.delete_files")

	// Orchestrator
	v.BindEnv("orchestrator.enabled")
	v.BindEnv("orchestrator.interval")
	v.BindEnv("orchestrator.startup_delay")
	v.BindEnv("orchestrator.token_refresh_interval")
	v.BindEnv("orchestrator.task_timeout")

	// Circuit breaker
	v.BindEnv("circuit_breaker.max_requests")
	v.BindEnv("circuit_breaker.interval")
	v.BindEnv("circuit_breaker.timeout")

	// Retry
	v.BindEnv("retry.max_retries")
	v.BindEnv("retry.initial_interval")
	v.BindEnv("retry.max_interval")
	v.BindEnv("retry.multiplier")

	// Rate limit
	v.BindEnv("rate_limit.enabled")
	v.BindEnv("rate_limit.requests_per_second")
	v.BindEnv("rate_limit.burst")
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 3000)
	v.SetDefault("server.read_timeout", "15s")
	v.SetDefault("server.write_timeout", "15s")
	v.SetDefault("server.idle_timeout", "60s")

	// Data
	v.SetDefault("data.dir", "./data")
	v.SetDefault("data.blacklist_file", "./data/blacklist.txt")
	v.SetDefault("data.token_file", "./data/token.json")

	// Database (optimized for SQLite - lower connection count reduces lock contention)
	v.SetDefault("database.path", "./data/gomenarr.db")
	v.SetDefault("database.max_open_conns", 5)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "5m")
	v.SetDefault("database.wal_mode", true)

	// Cache
	v.SetDefault("cache.default_expiration", "1h")
	v.SetDefault("cache.cleanup_interval", "10m")

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Metrics
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.namespace", "gomenarr")

	// Tracing
	v.SetDefault("tracing.enabled", false)
	v.SetDefault("tracing.service_name", "gomenarr")

	// Trakt
	v.SetDefault("trakt.redirect_uri", "urn:ietf:wg:oauth:2.0:oob")
	v.SetDefault("trakt.timeout", "30s")
	v.SetDefault("trakt.favorites_episode_limit", 3)

	// Newsnab
	v.SetDefault("newsnab.timeout", "30s")
	v.SetDefault("newsnab.max_results", 0) // 0 = no limit (recommended for best results)

	// NZBGet
	v.SetDefault("nzbget.timeout", "30s")
	v.SetDefault("nzbget.category", "trakt")
	v.SetDefault("nzbget.priority", 0)

	// Download
	v.SetDefault("download.min_validation_score", 65)
	v.SetDefault("download.min_quality_score", 40)
	v.SetDefault("download.min_total_score", 105)
	v.SetDefault("download.cleanup_watched_days", 5)
	v.SetDefault("download.delete_files", true)

	// Orchestrator
	v.SetDefault("orchestrator.enabled", true)
	v.SetDefault("orchestrator.interval", "6h")
	v.SetDefault("orchestrator.startup_delay", "30s")
	v.SetDefault("orchestrator.token_refresh_interval", "1h")
	v.SetDefault("orchestrator.task_timeout", "5m")

	// Circuit breaker
	v.SetDefault("circuit_breaker.max_requests", 3)
	v.SetDefault("circuit_breaker.interval", "10s")
	v.SetDefault("circuit_breaker.timeout", "60s")

	// Retry
	v.SetDefault("retry.max_retries", 3)
	v.SetDefault("retry.initial_interval", "1s")
	v.SetDefault("retry.max_interval", "30s")
	v.SetDefault("retry.multiplier", 2.0)

	// Rate limit
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_second", 10)
	v.SetDefault("rate_limit.burst", 20)
}

// normalizePaths ensures file paths are relative to data.dir if not explicitly overridden.
// This allows users to set GOMENARR_DATA_DIR and have all child paths automatically update.
// Individual paths can still be overridden with absolute paths or custom relative paths.
func (c *Config) normalizePaths() {
	// Only normalize paths that appear to be defaults (directory is ./data)
	// This preserves user-specified absolute paths and custom overrides

	// Normalize blacklist file path
	if !filepath.IsAbs(c.Data.BlacklistFile) && filepath.Dir(c.Data.BlacklistFile) == "./data" {
		c.Data.BlacklistFile = filepath.Join(c.Data.Dir, filepath.Base(c.Data.BlacklistFile))
	}

	// Normalize token file path
	if !filepath.IsAbs(c.Data.TokenFile) && filepath.Dir(c.Data.TokenFile) == "./data" {
		c.Data.TokenFile = filepath.Join(c.Data.Dir, filepath.Base(c.Data.TokenFile))
	}

	// Normalize database path
	if !filepath.IsAbs(c.Database.Path) && filepath.Dir(c.Database.Path) == "./data" {
		c.Database.Path = filepath.Join(c.Data.Dir, filepath.Base(c.Database.Path))
	}
}
