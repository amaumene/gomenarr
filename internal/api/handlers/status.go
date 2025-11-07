package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/amaumene/gomenarr/internal/models"
	"github.com/sirupsen/logrus"
)

// StatusHandler handles status requests
type StatusHandler struct {
	db     *models.Database
	logger *logrus.Logger
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(db *models.Database, logger *logrus.Logger) *StatusHandler {
	return &StatusHandler{
		db:     db,
		logger: logger,
	}
}

// StatusResponse represents the status response
type StatusResponse struct {
	TotalMedias     int            `json:"total_medias"`
	Pending         int            `json:"pending"`
	Searching       int            `json:"searching"`
	Downloading     int            `json:"downloading"`
	Completed       int            `json:"completed"`
	Failed          int            `json:"failed"`
	MediasByType    map[string]int `json:"medias_by_type"`
	MediasBySource  map[string]int `json:"medias_by_source"`
}

// ServeHTTP handles the status endpoint
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	medias, err := h.db.GetAllMedias()
	if err != nil {
		h.logger.WithError(err).Error("Failed to get medias")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := StatusResponse{
		TotalMedias:    len(medias),
		MediasByType:   make(map[string]int),
		MediasBySource: make(map[string]int),
	}

	for _, media := range medias {
		// Count by status
		switch media.Status {
		case models.StatusPending:
			response.Pending++
		case models.StatusSearching:
			response.Searching++
		case models.StatusDownloading:
			response.Downloading++
		case models.StatusCompleted:
			response.Completed++
		case models.StatusFailed:
			response.Failed++
		}

		// Count by type
		response.MediasByType[string(media.MediaType)]++

		// Count by source
		response.MediasBySource[string(media.Source)]++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
