package models

import (
	"fmt"
	"time"

	"github.com/timshannon/bolthold"
	"go.etcd.io/bbolt"
)

// Database wraps the bolthold store
type Database struct {
	store *bolthold.Store
}

// NewDatabase creates a new database connection
func NewDatabase(path string) (*Database, error) {
	store, err := bolthold.Open(path, 0600, &bolthold.Options{
		Options: &bbolt.Options{
			Timeout: 1 * time.Second,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Database{store: store}, nil
}

// Close closes the database connection
func (db *Database) Close() error {
	return db.store.Close()
}

// Media operations

// CreateMedia creates a new media item in the database
func (db *Database) CreateMedia(media *Media) error {
	media.CreatedAt = time.Now()
	media.UpdatedAt = time.Now()
	return db.store.Insert(bolthold.NextSequence(), media)
}

// UpdateMedia updates an existing media item
func (db *Database) UpdateMedia(media *Media) error {
	media.UpdatedAt = time.Now()
	return db.store.Update(media.ID, media)
}

// GetMediaByID retrieves a media item by ID
func (db *Database) GetMediaByID(id uint64) (*Media, error) {
	var media Media
	err := db.store.Get(id, &media)
	if err != nil {
		return nil, err
	}
	return &media, nil
}

// GetPendingMedias retrieves all media items with pending status
func (db *Database) GetPendingMedias() ([]*Media, error) {
	var medias []*Media
	err := db.store.Find(&medias, bolthold.Where("Status").Eq(StatusPending))
	return medias, err
}

// GetMediaByIMDBID retrieves a media item by IMDB ID and type
func (db *Database) GetMediaByIMDBID(imdbID string, mediaType MediaType, season *int, episode *int) (*Media, error) {
	var medias []*Media
	query := bolthold.Where("IMDBId").Eq(imdbID).And("MediaType").Eq(mediaType)

	err := db.store.Find(&medias, query)
	if err != nil {
		return nil, err
	}

	// Filter by season and episode if provided
	for _, media := range medias {
		if season != nil && media.SeasonNumber != nil && *media.SeasonNumber != *season {
			continue
		}
		if season == nil && media.SeasonNumber != nil {
			continue
		}
		if episode != nil && media.EpisodeNumber != nil && *media.EpisodeNumber != *episode {
			continue
		}
		if episode == nil && media.EpisodeNumber != nil {
			continue
		}
		return media, nil
	}

	return nil, bolthold.ErrNotFound
}

// GetAllMedias retrieves all media items
func (db *Database) GetAllMedias() ([]*Media, error) {
	var medias []*Media
	err := db.store.Find(&medias, nil)
	return medias, err
}

// GetMediasNotInTrakt retrieves all media items not currently in Trakt
func (db *Database) GetMediasNotInTrakt() ([]*Media, error) {
	var medias []*Media
	err := db.store.Find(&medias, bolthold.Where("InTrakt").Eq(false))
	return medias, err
}

// DeleteMedia deletes a media item by ID
func (db *Database) DeleteMedia(id uint64) error {
	return db.store.Delete(id, &Media{})
}

// MarkAllMediasNotInTrakt marks all media items as not in Trakt
func (db *Database) MarkAllMediasNotInTrakt() error {
	var medias []*Media
	err := db.store.Find(&medias, nil)
	if err != nil {
		return err
	}

	for _, media := range medias {
		media.InTrakt = false
		media.UpdatedAt = time.Now()
		if err := db.store.Update(media.ID, media); err != nil {
			return err
		}
	}

	return nil
}

// NZB operations

// CreateNZB creates a new NZB record
func (db *Database) CreateNZB(nzb *NZB) error {
	nzb.CreatedAt = time.Now()
	nzb.UpdatedAt = time.Now()
	return db.store.Insert(bolthold.NextSequence(), nzb)
}

// UpdateNZB updates an existing NZB record
func (db *Database) UpdateNZB(nzb *NZB) error {
	nzb.UpdatedAt = time.Now()
	return db.store.Update(nzb.ID, nzb)
}

// GetNZBByID retrieves an NZB by ID
func (db *Database) GetNZBByID(id uint64) (*NZB, error) {
	var nzb NZB
	err := db.store.Get(id, &nzb)
	if err != nil {
		return nil, err
	}
	return &nzb, nil
}

// GetNZBsByMediaID retrieves all NZBs for a media item
func (db *Database) GetNZBsByMediaID(mediaID uint64) ([]*NZB, error) {
	var nzbs []*NZB
	err := db.store.Find(&nzbs, bolthold.Where("MediaID").Eq(mediaID))
	return nzbs, err
}

// GetNZBByTorBoxJobID retrieves an NZB by TorBox job ID
func (db *Database) GetNZBByTorBoxJobID(jobID string) (*NZB, error) {
	var nzbs []*NZB
	err := db.store.Find(&nzbs, bolthold.Where("TorBoxJobID").Eq(jobID))
	if err != nil {
		return nil, err
	}
	if len(nzbs) == 0 {
		return nil, bolthold.ErrNotFound
	}
	return nzbs[0], nil
}

// GetNZBByTitle retrieves an NZB by its title (download name)
func (db *Database) GetNZBByTitle(title string) (*NZB, error) {
	var nzbs []*NZB
	err := db.store.Find(&nzbs, bolthold.Where("Title").Eq(title))
	if err != nil {
		return nil, err
	}
	if len(nzbs) == 0 {
		return nil, bolthold.ErrNotFound
	}
	return nzbs[0], nil
}

// GetBestCandidateNZB retrieves the best candidate NZB for a media item
func (db *Database) GetBestCandidateNZB(mediaID uint64) (*NZB, error) {
	var nzbs []*NZB
	err := db.store.Find(&nzbs,
		bolthold.Where("MediaID").Eq(mediaID).
		And("Status").Eq(NZBStatusCandidate))
	if err != nil {
		return nil, err
	}
	if len(nzbs) == 0 {
		return nil, bolthold.ErrNotFound
	}
	// First NZB is the best (should be pre-sorted by quality)
	return nzbs[0], nil
}

// GetNZBsByStatus retrieves all NZBs with a specific status
func (db *Database) GetNZBsByStatus(status NZBStatus) ([]*NZB, error) {
	var nzbs []*NZB
	err := db.store.Find(&nzbs, bolthold.Where("Status").Eq(status))
	return nzbs, err
}

// GetNZBByHash retrieves an NZB by its TorBox hash
func (db *Database) GetNZBByHash(hash string) (*NZB, error) {
	var nzb NZB
	err := db.store.FindOne(&nzb, bolthold.Where("TorBoxHash").Eq(hash))
	if err != nil {
		return nil, err
	}
	return &nzb, nil
}

// DeleteNZBsByMediaID deletes all NZBs for a media item
func (db *Database) DeleteNZBsByMediaID(mediaID uint64) error {
	var nzbs []*NZB
	err := db.store.Find(&nzbs, bolthold.Where("MediaID").Eq(mediaID))
	if err != nil {
		return err
	}

	for _, nzb := range nzbs {
		if err := db.store.Delete(nzb.ID, &NZB{}); err != nil {
			return err
		}
	}

	return nil
}
