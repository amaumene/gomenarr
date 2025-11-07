package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/amaumene/gomenarr/internal/controllers"
	"github.com/amaumene/gomenarr/internal/services/torbox"
	"github.com/sirupsen/logrus"
)

// WebhookHandler handles TorBox webhook callbacks
type WebhookHandler struct {
	downloadCtrl *controllers.DownloadController
	logger       *logrus.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(downloadCtrl *controllers.DownloadController, logger *logrus.Logger) *WebhookHandler {
	return &WebhookHandler{
		downloadCtrl: downloadCtrl,
		logger:       logger,
	}
}

// ServeHTTP handles the webhook endpoint
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload torbox.WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.logger.WithError(err).Error("Failed to decode webhook payload")
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	status := payload.GetStatus()

	// Extract download name from the notification message
	downloadName, err := payload.ExtractDownloadName()
	if err != nil {
		// Fallback: Try to extract hash from the message
		h.logger.WithFields(logrus.Fields{
			"title":   payload.Data.Title,
			"message": payload.Data.Message,
		}).Debug("Could not extract download name, trying hash fallback")

		hash, hashErr := payload.ExtractHash()
		if hashErr != nil {
			// Neither download name nor hash could be extracted
			h.logger.WithFields(logrus.Fields{
				"type":      payload.Type,
				"timestamp": payload.Timestamp,
				"title":     payload.Data.Title,
				"message":   payload.Data.Message,
			}).Warn("Received TorBox webhook without extractable download name or hash")

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		// Handle webhook by hash
		h.logger.WithFields(logrus.Fields{
			"hash":   hash,
			"status": status,
			"title":  payload.Data.Title,
		}).Info("Received TorBox webhook (matched by hash)")

		if err := h.downloadCtrl.HandleWebhookByHash(hash, status); err != nil {
			h.logger.WithError(err).Error("Failed to handle webhook by hash")
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Handle webhook by download name (primary method)
	h.logger.WithFields(logrus.Fields{
		"download_name": downloadName,
		"status":        status,
		"title":         payload.Data.Title,
	}).Info("Received TorBox webhook (matched by name)")

	// Handle all webhook statuses (completed, failed, etc.) through the unified handler
	// The HandleWebhookByName method will delete from TorBox and switch to next candidate on failure
	if err := h.downloadCtrl.HandleWebhookByName(downloadName, status); err != nil {
		h.logger.WithError(err).Error("Failed to handle webhook by name")
		http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
