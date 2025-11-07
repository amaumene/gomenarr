package controllers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/amaumene/gomenarr/internal/models"
	"github.com/amaumene/gomenarr/internal/services/newznab"
	"github.com/amaumene/gomenarr/internal/services/torbox"
	"github.com/sirupsen/logrus"
)

const maxRetries = 5

// DownloadController manages download operations
type DownloadController struct {
	db            *models.Database
	torboxClient  *torbox.Client
	newznabClient *newznab.Client
	logger        *logrus.Logger
}

// NewDownloadController creates a new download controller
func NewDownloadController(db *models.Database, torboxClient *torbox.Client, newznabClient *newznab.Client, logger *logrus.Logger) *DownloadController {
	return &DownloadController{
		db:            db,
		torboxClient:  torboxClient,
		newznabClient: newznabClient,
		logger:        logger,
	}
}

// DownloadNZB creates a download job for an NZB
func (c *DownloadController) DownloadNZB(nzb *models.NZB) error {
	c.logger.WithFields(logrus.Fields{
		"nzb_id": nzb.ID,
		"title":  nzb.Title,
		"link":   nzb.Link,
	}).Info("Starting download")

	// Download NZB file from indexer
	nzbData, err := c.newznabClient.DownloadNZB(nzb.Link)
	if err != nil {
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = fmt.Sprintf("failed to download NZB: %v", err)
		c.db.UpdateNZB(nzb)
		return fmt.Errorf("failed to download NZB from indexer: %w", err)
	}

	// Create TorBox job by uploading NZB file
	filename := nzb.Title + ".nzb"
	jobID, response, err := c.torboxClient.CreateDownloadJob(nzbData, filename, nzb.Title)
	if err != nil {
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = fmt.Sprintf("failed to upload to TorBox: %v", err)
		c.db.UpdateNZB(nzb)
		return fmt.Errorf("failed to create download job: %w", err)
	}

	// Update NZB with job ID and hash
	nzb.TorBoxJobID = jobID
	nzb.TorBoxHash = response.Data.Hash
	nzb.Status = models.NZBStatusDownloading
	if err := c.db.UpdateNZB(nzb); err != nil {
		c.logger.WithError(err).Error("Failed to update NZB status")
	}

	// Update media status
	media, err := c.db.GetMediaByID(nzb.MediaID)
	if err != nil {
		c.logger.WithError(err).Error("Failed to get media")
		return err
	}

	media.Status = models.StatusDownloading
	if err := c.db.UpdateMedia(media); err != nil {
		c.logger.WithError(err).Error("Failed to update media status")
	}

	c.logger.WithFields(logrus.Fields{
		"nzb_id": nzb.ID,
		"job_id": jobID,
	}).Info("Download job created")

	// Check if file is cached - if so, mark as completed immediately
	if response != nil && response.Detail == "Found cached usenet download. Using cached download." {
		c.logger.WithFields(logrus.Fields{
			"nzb_id": nzb.ID,
			"job_id": jobID,
		}).Info("File is cached in TorBox, verifying and marking as completed")

		if err := c.HandleCachedDownload(nzb, jobID); err != nil {
			c.logger.WithError(err).Warn("Failed to handle cached download, will wait for webhook")
		}
	}

	return nil
}

// HandleCachedDownload verifies a download is cached and marks it as completed
func (c *DownloadController) HandleCachedDownload(nzb *models.NZB, jobID string) error {
	// Convert jobID to int
	downloadID, err := strconv.Atoi(jobID)
	if err != nil {
		return fmt.Errorf("invalid job ID: %w", err)
	}

	// Verify the download is truly cached
	download, err := c.torboxClient.FindDownloadByID(downloadID)
	if err != nil {
		return fmt.Errorf("failed to find download: %w", err)
	}

	if !download.Cached {
		c.logger.WithFields(logrus.Fields{
			"job_id": jobID,
			"cached": download.Cached,
		}).Info("Download not truly cached yet, waiting for webhook")
		return nil
	}

	// File is cached and ready - mark as completed immediately
	c.logger.WithFields(logrus.Fields{
		"nzb_id": nzb.ID,
		"job_id": jobID,
	}).Info("Download verified as cached, marking as completed")

	// Update NZB status
	nzb.Status = models.NZBStatusCompleted
	now := time.Now()
	nzb.DownloadedAt = &now
	if err := c.db.UpdateNZB(nzb); err != nil {
		return fmt.Errorf("failed to update NZB: %w", err)
	}

	// Update media status
	media, err := c.db.GetMediaByID(nzb.MediaID)
	if err != nil {
		return fmt.Errorf("failed to get media: %w", err)
	}

	media.Status = models.StatusCompleted
	media.CompletedAt = &now
	if err := c.db.UpdateMedia(media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"media_id": media.ID,
		"title":    media.Title,
	}).Info("Cached download marked as completed")

	return nil
}

// HandleWebhook handles webhook callbacks from TorBox
func (c *DownloadController) HandleWebhook(jobID string, status string, errorMsg string) error {
	c.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"status": status,
	}).Info("Processing webhook")

	// Find NZB by job ID
	nzb, err := c.db.GetNZBByTorBoxJobID(jobID)
	if err != nil {
		return fmt.Errorf("NZB not found for job ID %s: %w", jobID, err)
	}

	media, err := c.db.GetMediaByID(nzb.MediaID)
	if err != nil {
		return fmt.Errorf("media not found: %w", err)
	}

	switch status {
	case "completed", "success":
		// Mark as completed
		nzb.Status = models.NZBStatusCompleted
		media.Status = models.StatusCompleted

		now := media.UpdatedAt
		nzb.DownloadedAt = &now
		media.CompletedAt = &now

		c.logger.WithFields(logrus.Fields{
			"media_id": media.ID,
			"title":    media.Title,
		}).Info("Download completed successfully")

	case "failed", "error":
		// Delete from TorBox before trying next candidate
		if nzb.TorBoxJobID != "" {
			if err := c.torboxClient.DeleteJob(nzb.TorBoxJobID); err != nil {
				c.logger.WithError(err).WithField("job_id", nzb.TorBoxJobID).Warn("Failed to delete job from TorBox")
			} else {
				c.logger.WithField("job_id", nzb.TorBoxJobID).Info("Deleted failed download from TorBox")
			}
		}

		// Mark as failed and retry
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = errorMsg
		nzb.RetryCount++

		c.logger.WithFields(logrus.Fields{
			"media_id":    media.ID,
			"retry_count": nzb.RetryCount,
			"error":       errorMsg,
		}).Warn("Download failed")

		// Try next candidate
		if nzb.RetryCount < maxRetries {
			if err := c.RetryWithNextCandidate(nzb.MediaID); err != nil {
				c.logger.WithError(err).Error("Failed to retry with next candidate")
				media.Status = models.StatusFailed
			}
		} else {
			c.logger.WithField("media_id", media.ID).Error("Max retries reached")
			media.Status = models.StatusFailed
		}
	}

	// Update database
	if err := c.db.UpdateNZB(nzb); err != nil {
		return fmt.Errorf("failed to update NZB: %w", err)
	}

	if err := c.db.UpdateMedia(media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	return nil
}

// RetryWithNextCandidate finds and downloads the next best candidate
func (c *DownloadController) RetryWithNextCandidate(mediaID uint64) error {
	c.logger.WithField("media_id", mediaID).Info("Retrying with next candidate")

	// Get next best candidate
	nzb, err := c.db.GetBestCandidateNZB(mediaID)
	if err != nil {
		return fmt.Errorf("no more candidates available: %w", err)
	}

	// Mark as selected and download
	nzb.Status = models.NZBStatusSelected
	if err := c.db.UpdateNZB(nzb); err != nil {
		return err
	}

	return c.DownloadNZB(nzb)
}

// RestartDownload restarts a failed download with the same NZB
func (c *DownloadController) RestartDownload(jobID string) error {
	c.logger.WithField("job_id", jobID).Info("Restarting failed download")

	// Find NZB by job ID
	nzb, err := c.db.GetNZBByTorBoxJobID(jobID)
	if err != nil {
		return fmt.Errorf("NZB not found for job ID %s: %w", jobID, err)
	}

	c.logger.WithFields(logrus.Fields{
		"nzb_id":      nzb.ID,
		"title":       nzb.Title,
		"retry_count": nzb.RetryCount,
	}).Info("Found NZB to restart")

	// Check if we've exceeded max retries
	if nzb.RetryCount >= maxRetries {
		c.logger.WithFields(logrus.Fields{
			"nzb_id":      nzb.ID,
			"retry_count": nzb.RetryCount,
		}).Error("Max retries exceeded, trying next candidate")

		// Try next candidate instead
		return c.RetryWithNextCandidate(nzb.MediaID)
	}

	// Increment retry count
	nzb.RetryCount++

	// Download NZB file from indexer
	nzbData, err := c.newznabClient.DownloadNZB(nzb.Link)
	if err != nil {
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = fmt.Sprintf("restart failed - download NZB: %v", err)
		c.db.UpdateNZB(nzb)
		return fmt.Errorf("failed to download NZB for restart: %w", err)
	}

	// Create new TorBox job by uploading NZB file
	filename := nzb.Title + ".nzb"
	newJobID, _, err := c.torboxClient.CreateDownloadJob(nzbData, filename, nzb.Title)
	if err != nil {
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = fmt.Sprintf("restart failed - upload to TorBox: %v", err)
		c.db.UpdateNZB(nzb)
		return fmt.Errorf("failed to restart download: %w", err)
	}

	// Update NZB with new job ID
	nzb.TorBoxJobID = newJobID
	nzb.Status = models.NZBStatusDownloading
	nzb.FailureReason = "" // Clear previous failure reason

	if err := c.db.UpdateNZB(nzb); err != nil {
		c.logger.WithError(err).Error("Failed to update NZB after restart")
		return err
	}

	c.logger.WithFields(logrus.Fields{
		"nzb_id":      nzb.ID,
		"old_job_id":  jobID,
		"new_job_id":  newJobID,
		"retry_count": nzb.RetryCount,
	}).Info("Download restarted successfully")

	return nil
}

// HandleWebhookByName handles webhook callbacks from TorBox by download name
func (c *DownloadController) HandleWebhookByName(downloadName string, status string) error {
	c.logger.WithFields(logrus.Fields{
		"download_name": downloadName,
		"status":        status,
	}).Info("Processing webhook by download name")

	// Find NZB by title (download name)
	nzb, err := c.db.GetNZBByTitle(downloadName)
	if err != nil {
		return fmt.Errorf("NZB not found for download name %s: %w", downloadName, err)
	}

	// Use the existing webhook handler with the job_id
	return c.HandleWebhook(nzb.TorBoxJobID, status, "")
}

// HandleWebhookByHash handles webhook callbacks from TorBox by hash
func (c *DownloadController) HandleWebhookByHash(hash string, status string) error {
	c.logger.WithFields(logrus.Fields{
		"hash":   hash,
		"status": status,
	}).Info("Processing webhook by hash")

	// Find NZB by hash
	nzb, err := c.db.GetNZBByHash(hash)
	if err != nil {
		return fmt.Errorf("NZB not found for hash %s: %w", hash, err)
	}

	// Use the existing webhook handler with the job_id
	return c.HandleWebhook(nzb.TorBoxJobID, status, "")
}

// RestartDownloadByName restarts a failed download by download name
func (c *DownloadController) RestartDownloadByName(downloadName string) error {
	c.logger.WithField("download_name", downloadName).Info("Restarting failed download by name")

	// Find NZB by title (download name)
	nzb, err := c.db.GetNZBByTitle(downloadName)
	if err != nil {
		return fmt.Errorf("NZB not found for download name %s: %w", downloadName, err)
	}

	c.logger.WithFields(logrus.Fields{
		"nzb_id":      nzb.ID,
		"title":       nzb.Title,
		"retry_count": nzb.RetryCount,
	}).Info("Found NZB to restart")

	// Check if we've exceeded max retries
	if nzb.RetryCount >= maxRetries {
		c.logger.WithFields(logrus.Fields{
			"nzb_id":      nzb.ID,
			"retry_count": nzb.RetryCount,
		}).Error("Max retries exceeded, trying next candidate")

		// Try next candidate instead
		return c.RetryWithNextCandidate(nzb.MediaID)
	}

	// Increment retry count
	nzb.RetryCount++

	// Download NZB file from indexer
	nzbData, err := c.newznabClient.DownloadNZB(nzb.Link)
	if err != nil {
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = fmt.Sprintf("restart failed - download NZB: %v", err)
		c.db.UpdateNZB(nzb)
		return fmt.Errorf("failed to download NZB for restart: %w", err)
	}

	// Create new TorBox job by uploading NZB file
	filename := nzb.Title + ".nzb"
	newJobID, _, err := c.torboxClient.CreateDownloadJob(nzbData, filename, nzb.Title)
	if err != nil {
		nzb.Status = models.NZBStatusFailed
		nzb.FailureReason = fmt.Sprintf("restart failed - upload to TorBox: %v", err)
		c.db.UpdateNZB(nzb)
		return fmt.Errorf("failed to restart download: %w", err)
	}

	// Update NZB with new job ID
	nzb.TorBoxJobID = newJobID
	nzb.Status = models.NZBStatusDownloading
	nzb.FailureReason = "" // Clear previous failure reason

	if err := c.db.UpdateNZB(nzb); err != nil {
		c.logger.WithError(err).Error("Failed to update NZB after restart")
		return err
	}

	c.logger.WithFields(logrus.Fields{
		"nzb_id":      nzb.ID,
		"new_job_id":  newJobID,
		"retry_count": nzb.RetryCount,
	}).Info("Download restarted successfully")

	return nil
}

// CheckStuckDownloads checks for downloads that have been stuck for too long and retries them
func (c *DownloadController) CheckStuckDownloads(timeout time.Duration) error {
	// Get all downloading NZBs
	nzbs, err := c.db.GetNZBsByStatus(models.NZBStatusDownloading)
	if err != nil {
		return fmt.Errorf("failed to get downloading NZBs: %w", err)
	}

	if len(nzbs) == 0 {
		c.logger.Debug("No downloading NZBs to check")
		return nil
	}

	now := time.Now()
	stuckCount := 0

	for _, nzb := range nzbs {
		duration := now.Sub(nzb.UpdatedAt)

		if duration > timeout {
			stuckCount++
			c.logger.WithFields(logrus.Fields{
				"nzb_id":   nzb.ID,
				"title":    nzb.Title,
				"job_id":   nzb.TorBoxJobID,
				"duration": duration,
				"timeout":  timeout,
			}).Warn("Download timeout detected, deleting and retrying")

			// Delete from TorBox
			if nzb.TorBoxJobID != "" {
				if err := c.torboxClient.DeleteJob(nzb.TorBoxJobID); err != nil {
					c.logger.WithError(err).WithField("job_id", nzb.TorBoxJobID).Warn("Failed to delete stuck job from TorBox")
				} else {
					c.logger.WithField("job_id", nzb.TorBoxJobID).Info("Deleted stuck download from TorBox")
				}
			}

			// Mark as failed
			nzb.Status = models.NZBStatusFailed
			nzb.FailureReason = fmt.Sprintf("Download timeout after %v", duration)
			nzb.RetryCount++

			if err := c.db.UpdateNZB(nzb); err != nil {
				c.logger.WithError(err).Error("Failed to update stuck NZB")
				continue
			}

			// Retry with next candidate
			if nzb.RetryCount < maxRetries {
				if err := c.RetryWithNextCandidate(nzb.MediaID); err != nil {
					c.logger.WithError(err).Error("Failed to retry with next candidate")

					// Update media status to failed if no more candidates
					media, err := c.db.GetMediaByID(nzb.MediaID)
					if err == nil {
						media.Status = models.StatusFailed
						c.db.UpdateMedia(media)
					}
				}
			} else {
				c.logger.WithFields(logrus.Fields{
					"nzb_id":      nzb.ID,
					"retry_count": nzb.RetryCount,
				}).Error("Max retries reached for stuck download")

				// Update media status to failed
				media, err := c.db.GetMediaByID(nzb.MediaID)
				if err == nil {
					media.Status = models.StatusFailed
					c.db.UpdateMedia(media)
				}
			}
		}
	}

	if stuckCount > 0 {
		c.logger.WithField("count", stuckCount).Info("Processed stuck downloads")
	}

	return nil
}
