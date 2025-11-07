package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/amaumene/gomenarr/internal/config"
	"github.com/sirupsen/logrus"
)

const (
	baseURL = "https://api.trakt.tv"
	apiVersion = "2"
)

// Client handles communication with Trakt API
type Client struct {
	clientID     string
	clientSecret string
	tokenStore   TokenStore
	httpClient   *http.Client
	logger       *logrus.Logger
}

// NewClient creates a new Trakt API client
func NewClient(cfg *config.Config, logger *logrus.Logger) (*Client, error) {
	tokenStore, err := NewFileTokenStore(cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create token store: %w", err)
	}

	return &Client{
		clientID:     cfg.TraktClientID,
		clientSecret: cfg.TraktClientSecret,
		tokenStore:   tokenStore,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		logger:       logger,
	}, nil
}

// doRequest performs an authenticated HTTP request to Trakt API
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	// Check and refresh token if needed
	if err := c.ensureValidToken(ctx); err != nil {
		return fmt.Errorf("failed to ensure valid token: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	fullURL := baseURL + path
	c.logger.WithFields(logrus.Fields{
		"method": method,
		"url":    fullURL,
	}).Debug("Making Trakt API request")

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", apiVersion)
	req.Header.Set("trakt-api-key", c.clientID)

	// Add authorization if we have a token
	token, err := c.tokenStore.GetToken()
	if err == nil && token != nil {
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// ensureValidToken checks if the current token is valid and refreshes if needed
func (c *Client) ensureValidToken(ctx context.Context) error {
	token, err := c.tokenStore.GetToken()
	if err != nil {
		c.logger.Debug("No valid token found, authentication required")
		return nil
	}

	// Check if token expires within 24 hours
	if time.Until(token.ExpiresAt) < 24*time.Hour {
		c.logger.Info("Token expires soon, refreshing...")
		return c.RefreshToken(ctx)
	}

	return nil
}
