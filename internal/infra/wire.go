//go:build wireinject
// +build wireinject

package infra

import (
	"github.com/google/wire"
	httpAdapter "github.com/amaumene/gomenarr/internal/adapters/primary/http"
	"github.com/amaumene/gomenarr/internal/adapters/secondary/newsnab"
	"github.com/amaumene/gomenarr/internal/adapters/secondary/nzbget"
	"github.com/amaumene/gomenarr/internal/adapters/secondary/sqlite"
	"github.com/amaumene/gomenarr/internal/adapters/secondary/trakt"
	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/core/services"
	"github.com/amaumene/gomenarr/internal/infra/database"
	"github.com/amaumene/gomenarr/internal/orchestrator"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/amaumene/gomenarr/pkg/scorer"
	"gorm.io/gorm"
)

type Application struct {
	Config         *config.Config
	DB             *gorm.DB
	Server         *httpAdapter.Server
	Orchestrator   *orchestrator.Orchestrator
	MediaService   *services.MediaService
	CleanupService *services.CleanupService
	TraktClient    ports.TraktClient
}

func InitializeApplication() (*Application, error) {
	wire.Build(
		// Config
		config.Load,

		// Database
		database.New,
		provideDatabaseConfig,

		// Repositories
		sqlite.NewMediaRepository,
		wire.Bind(new(ports.MediaRepository), new(*sqlite.MediaRepository)),
		sqlite.NewNZBRepository,
		wire.Bind(new(ports.NZBRepository), new(*sqlite.NZBRepository)),

		// External clients
		provideDataDir,
		trakt.NewClient,
		wire.Bind(new(ports.TraktClient), new(*trakt.Client)),
		newsnab.NewClient,
		wire.Bind(new(ports.NZBSearcher), new(*newsnab.Client)),
		nzbget.NewClient,
		wire.Bind(new(ports.DownloadClient), new(*nzbget.Client)),

		// Utilities
		provideBlacklist,

		// Services
		services.NewMediaService,
		services.NewNZBService,
		services.NewDownloadService,
		services.NewNotificationService,
		services.NewCleanupService,

		// Orchestrator
		orchestrator.New,

		// HTTP
		httpAdapter.NewHandlers,
		httpAdapter.NewServer,

		// App
		wire.Struct(new(Application), "*"),

		// Config providers
		provideTraktConfig,
		provideNewsnabConfig,
		provideNZBGetConfig,
		provideDownloadConfig,
		provideOrchestratorConfig,
		provideServerConfig,
	)
	return &Application{}, nil
}

func provideDataDir(cfg *config.Config) string {
	return cfg.Data.Dir
}

func provideBlacklist(cfg *config.Config) *scorer.Blacklist {
	bl := scorer.NewBlacklist()
	if err := bl.Load(cfg.Data.BlacklistFile); err != nil {
		// Log but don't fail
	}
	return bl
}

func provideDatabaseConfig(cfg *config.Config) config.DatabaseConfig {
	return cfg.Database
}

func provideTraktConfig(cfg *config.Config) config.TraktConfig {
	return cfg.Trakt
}

func provideNewsnabConfig(cfg *config.Config) config.NewsnabConfig {
	return cfg.Newsnab
}

func provideNZBGetConfig(cfg *config.Config) config.NZBGetConfig {
	return cfg.NZBGet
}

func provideDownloadConfig(cfg *config.Config) config.DownloadConfig {
	return cfg.Download
}

func provideOrchestratorConfig(cfg *config.Config) config.OrchestratorConfig {
	return cfg.Orchestrator
}

func provideServerConfig(cfg *config.Config) config.ServerConfig {
	return cfg.Server
}
