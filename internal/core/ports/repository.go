package ports

import (
	"context"
	"github.com/amaumene/gomenarr/internal/core/domain"
)

// MediaRepository defines the interface for media persistence
type MediaRepository interface {
	Create(ctx context.Context, media *domain.Media) error
	Update(ctx context.Context, media *domain.Media) error
	Upsert(ctx context.Context, media *domain.Media) error
	Delete(ctx context.Context, traktID int64) error
	FindByTraktID(ctx context.Context, traktID int64) (*domain.Media, error)
	FindAll(ctx context.Context) ([]*domain.Media, error)
	FindNotOnDisk(ctx context.Context) ([]*domain.Media, error)
	DeleteByTraktIDs(ctx context.Context, traktIDs []int64) error
}

// NZBRepository defines the interface for NZB persistence
type NZBRepository interface {
	Create(ctx context.Context, nzb *domain.NZB) error
	Update(ctx context.Context, nzb *domain.NZB) error
	FindByID(ctx context.Context, id uint) (*domain.NZB, error)
	FindByTraktID(ctx context.Context, traktID int64) ([]*domain.NZB, error)
	FindBestByTraktID(ctx context.Context, traktID int64) (*domain.NZB, error)
	FindSeasonPackByIMDB(ctx context.Context, imdb string, season int64) (*domain.NZB, error)
	MarkAsFailedByTitle(ctx context.Context, title string) error
	DeleteByTraktIDs(ctx context.Context, traktIDs []int64) error
	FindAll(ctx context.Context) ([]*domain.NZB, error)
}
