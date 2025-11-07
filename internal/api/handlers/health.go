package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	logger *logrus.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(logger *logrus.Logger) *HealthHandler {
	return &HealthHandler{logger: logger}
}

// ServeHTTP handles the health check endpoint
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]string{
		"status": "healthy",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
