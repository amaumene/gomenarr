package trakt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TokenStore defines the interface for storing and retrieving tokens
type TokenStore interface {
	GetToken() (*Token, error)
	SaveToken(token *Token) error
}

// Token represents a Trakt authentication token
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// FileTokenStore implements TokenStore using a JSON file
type FileTokenStore struct {
	filepath string
}

// NewFileTokenStore creates a new file-based token store
func NewFileTokenStore(filepath string) (*FileTokenStore, error) {
	return &FileTokenStore{filepath: filepath}, nil
}

// GetToken retrieves the token from the file
func (s *FileTokenStore) GetToken() (*Token, error) {
	data, err := os.ReadFile(s.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("token file not found")
		}
		return nil, err
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// SaveToken saves the token to the file
func (s *FileTokenStore) SaveToken(token *Token) error {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filepath, data, 0600)
}

// DeviceCodeResponse represents the response from device code request
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse represents the response from token request
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// GetToken retrieves the current token from the token store
func (c *Client) GetToken() (*Token, error) {
	return c.tokenStore.GetToken()
}

// Authenticate performs device authentication flow
func (c *Client) Authenticate(ctx context.Context) error {
	// Step 1: Request device code
	deviceCodeReq := map[string]string{
		"client_id": c.clientID,
	}

	var deviceResp DeviceCodeResponse
	if err := c.doRequest(ctx, "POST", "/oauth/device/code", deviceCodeReq, &deviceResp); err != nil {
		return fmt.Errorf("failed to get device code: %w", err)
	}

	// Step 2: Display user code and URL
	c.logger.Infof("Please visit %s and enter code: %s", deviceResp.VerificationURL, deviceResp.UserCode)
	fmt.Printf("\nPlease visit %s and enter code: %s\n\n", deviceResp.VerificationURL, deviceResp.UserCode)

	// Step 3: Poll for token
	interval := time.Duration(deviceResp.Interval) * time.Second
	timeout := time.Duration(deviceResp.ExpiresIn) * time.Second
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("authentication timeout")
			}

			tokenReq := map[string]string{
				"code":          deviceResp.DeviceCode,
				"client_id":     c.clientID,
				"client_secret": c.clientSecret,
			}

			var tokenResp TokenResponse
			err := c.doRequest(ctx, "POST", "/oauth/device/token", tokenReq, &tokenResp)
			if err != nil {
				// Continue polling on certain errors
				c.logger.Debug("Waiting for user authorization...")
				continue
			}

			// Success! Save token
			token := &Token{
				AccessToken:  tokenResp.AccessToken,
				RefreshToken: tokenResp.RefreshToken,
				ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
			}

			if err := c.tokenStore.SaveToken(token); err != nil {
				return fmt.Errorf("failed to save token: %w", err)
			}

			c.logger.Info("Authentication successful!")
			return nil
		}
	}
}

// RefreshToken refreshes the access token using the refresh token
func (c *Client) RefreshToken(ctx context.Context) error {
	token, err := c.tokenStore.GetToken()
	if err != nil {
		return fmt.Errorf("no token to refresh: %w", err)
	}

	refreshReq := map[string]string{
		"refresh_token": token.RefreshToken,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"grant_type":    "refresh_token",
	}

	var tokenResp TokenResponse
	if err := c.doRequest(ctx, "POST", "/oauth/token", refreshReq, &tokenResp); err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	newToken := &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if err := c.tokenStore.SaveToken(newToken); err != nil {
		return fmt.Errorf("failed to save refreshed token: %w", err)
	}

	c.logger.Info("Token refreshed successfully")
	return nil
}
