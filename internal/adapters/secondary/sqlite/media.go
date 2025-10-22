package sqlite

import (
	"context"
	"errors"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"gorm.io/gorm"
)

// MediaRepository implements ports.MediaRepository
type MediaRepository struct {
	db *gorm.DB
}

// NewMediaRepository creates a new media repository
func NewMediaRepository(db *gorm.DB) *MediaRepository {
	return &MediaRepository{db: db}
}

func (r *MediaRepository) Create(ctx context.Context, media *domain.Media) error {
	if err := r.db.WithContext(ctx).Create(media).Error; err != nil {
		return err
	}
	return nil
}

func (r *MediaRepository) Update(ctx context.Context, media *domain.Media) error {
	if err := r.db.WithContext(ctx).Save(media).Error; err != nil {
		return err
	}
	return nil
}

func (r *MediaRepository) Upsert(ctx context.Context, media *domain.Media) error {
	var existing domain.Media
	err := r.db.WithContext(ctx).Where("trakt_id = ?", media.TraktID).First(&existing).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.Create(ctx, media)
		}
		return err
	}
	
	// Update existing
	media.CreatedAt = existing.CreatedAt
	return r.Update(ctx, media)
}

func (r *MediaRepository) Delete(ctx context.Context, traktID int64) error {
	if err := r.db.WithContext(ctx).Where("trakt_id = ?", traktID).Delete(&domain.Media{}).Error; err != nil {
		return err
	}
	return nil
}

func (r *MediaRepository) FindByTraktID(ctx context.Context, traktID int64) (*domain.Media, error) {
	var media domain.Media
	if err := r.db.WithContext(ctx).Where("trakt_id = ?", traktID).First(&media).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &media, nil
}

func (r *MediaRepository) FindAll(ctx context.Context) ([]*domain.Media, error) {
	var media []*domain.Media
	if err := r.db.WithContext(ctx).Find(&media).Error; err != nil {
		return nil, err
	}
	return media, nil
}

func (r *MediaRepository) FindNotOnDisk(ctx context.Context) ([]*domain.Media, error) {
	var media []*domain.Media
	if err := r.db.WithContext(ctx).Where("on_disk = ?", false).Find(&media).Error; err != nil {
		return nil, err
	}
	return media, nil
}

func (r *MediaRepository) DeleteByTraktIDs(ctx context.Context, traktIDs []int64) error {
	if len(traktIDs) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("trakt_id IN ?", traktIDs).Delete(&domain.Media{}).Error; err != nil {
		return err
	}
	return nil
}
