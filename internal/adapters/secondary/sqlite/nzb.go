package sqlite

import (
	"context"
	"errors"

	"github.com/amaumene/gomenarr/internal/core/domain"
	"gorm.io/gorm"
)

// NZBRepository implements ports.NZBRepository
type NZBRepository struct {
	db *gorm.DB
}

// NewNZBRepository creates a new NZB repository
func NewNZBRepository(db *gorm.DB) *NZBRepository {
	return &NZBRepository{db: db}
}

func (r *NZBRepository) Create(ctx context.Context, nzb *domain.NZB) error {
	if err := r.db.WithContext(ctx).Create(nzb).Error; err != nil {
		return err
	}
	return nil
}

func (r *NZBRepository) Update(ctx context.Context, nzb *domain.NZB) error {
	if err := r.db.WithContext(ctx).Save(nzb).Error; err != nil {
		return err
	}
	return nil
}

func (r *NZBRepository) FindByID(ctx context.Context, id uint) (*domain.NZB, error) {
	var nzb domain.NZB
	if err := r.db.WithContext(ctx).First(&nzb, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &nzb, nil
}

func (r *NZBRepository) FindByTraktID(ctx context.Context, traktID int64) ([]*domain.NZB, error) {
	var nzbs []*domain.NZB
	if err := r.db.WithContext(ctx).
		Where("trakt_id = ?", traktID).
		Order("total_score DESC").
		Find(&nzbs).Error; err != nil {
		return nil, err
	}
	return nzbs, nil
}

func (r *NZBRepository) FindBestByTraktID(ctx context.Context, traktID int64) (*domain.NZB, error) {
	var nzb domain.NZB
	if err := r.db.WithContext(ctx).
		Where("trakt_id = ? AND failed = ?", traktID, false).
		Order("total_score DESC").
		First(&nzb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &nzb, nil
}

func (r *NZBRepository) FindSeasonPackByIMDB(ctx context.Context, imdb string, season int64) (*domain.NZB, error) {
	var nzb domain.NZB
	if err := r.db.WithContext(ctx).
		Where("imdb = ? AND parsed_season = ? AND parsed_episode = ? AND failed = ?", imdb, season, 0, false).
		Order("total_score DESC").
		First(&nzb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &nzb, nil
}

func (r *NZBRepository) FindBestSeasonPack(ctx context.Context, imdb string, season int64) (*domain.NZB, error) {
	var nzb domain.NZB
	if err := r.db.WithContext(ctx).
		Where("imdb = ? AND parsed_season = ? AND is_season_pack = ? AND failed = ?", imdb, season, true, false).
		Order("total_score DESC").
		First(&nzb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &nzb, nil
}

func (r *NZBRepository) MarkAsFailedByTitle(ctx context.Context, title string) error {
	if err := r.db.WithContext(ctx).
		Model(&domain.NZB{}).
		Where("title = ?", title).
		Update("failed", true).Error; err != nil {
		return err
	}
	return nil
}

func (r *NZBRepository) DeleteByTraktIDs(ctx context.Context, traktIDs []int64) error {
	if len(traktIDs) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("trakt_id IN ?", traktIDs).Delete(&domain.NZB{}).Error; err != nil {
		return err
	}
	return nil
}

func (r *NZBRepository) FindAll(ctx context.Context) ([]*domain.NZB, error) {
	var nzbs []*domain.NZB
	if err := r.db.WithContext(ctx).Order("total_score DESC").Find(&nzbs).Error; err != nil {
		return nil, err
	}
	return nzbs, nil
}
