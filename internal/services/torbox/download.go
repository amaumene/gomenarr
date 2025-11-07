package torbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
)

const torboxAPIBase = "https://api.torbox.app/v1/api"

// CreateDownloadJobRequest represents a download job creation request
type CreateDownloadJobRequest struct {
	Link string `json:"link"` // NZB download link
	Name string `json:"name,omitempty"`
}

// CreateDownloadJobResponse represents the response from creating a download job
type CreateDownloadJobResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
	Detail  string  `json:"detail"` // e.g., "Found cached usenet download. Using cached download."
	Data    struct {
		Hash             string `json:"hash"`
		UsenetDownloadID int    `json:"usenetdownload_id"`
		AuthID           string `json:"auth_id"`
	} `json:"data"`
}

// UsenetDownloadFile represents a file within a usenet download
type UsenetDownloadFile struct {
	ID           int    `json:"id"`
	MD5          string `json:"md5"`
	Hash         string `json:"hash"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Zipped       bool   `json:"zipped"`
	S3Path       string `json:"s3_path"`
	Infected     bool   `json:"infected"`
	MimeType     string `json:"mimetype"`
	ShortName    string `json:"short_name"`
	AbsolutePath string `json:"absolute_path"`
}

// UsenetDownload represents a usenet download from TorBox
type UsenetDownload struct {
	ID               int                  `json:"id"`
	CreatedAt        string               `json:"created_at"`
	UpdatedAt        string               `json:"updated_at"`
	AuthID           string               `json:"auth_id"`
	Name             string               `json:"name"`
	Hash             string               `json:"hash"`
	DownloadState    string               `json:"download_state"`
	DownloadSpeed    int                  `json:"download_speed"`
	OriginalURL      string               `json:"original_url"`
	ETA              int                  `json:"eta"`
	Progress         float64              `json:"progress"`
	Size             int64                `json:"size"`
	DownloadID       string               `json:"download_id"`
	Files            []UsenetDownloadFile `json:"files"`
	Active           bool                 `json:"active"`
	Cached           bool                 `json:"cached"`           // TRUE if file is cached and ready
	DownloadPresent  bool                 `json:"download_present"` // TRUE if download is available
	DownloadFinished bool                 `json:"download_finished"`
	ExpiresAt        *string              `json:"expires_at"`
}

// UsenetListResponse represents the response from listing usenet downloads
type UsenetListResponse struct {
	Success bool             `json:"success"`
	Error   *string          `json:"error"`
	Detail  string           `json:"detail"`
	Data    []UsenetDownload `json:"data"`
}

// CreateDownloadJob creates a new download job in TorBox by uploading NZB file
// Returns the job ID and the full response (for checking cached status)
func (c *Client) CreateDownloadJob(nzbData []byte, filename string, name string) (string, *CreateDownloadJobResponse, error) {
	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field (the actual NZB file)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(nzbData); err != nil {
		return "", nil, fmt.Errorf("failed to write NZB data: %w", err)
	}

	// Add name field (helps TorBox identify the download in webhooks)
	if name != "" {
		if err := writer.WriteField("name", name); err != nil {
			return "", nil, fmt.Errorf("failed to add name field: %w", err)
		}
	}

	// Close the writer to finalize the multipart form
	if err := writer.Close(); err != nil {
		return "", nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// DEBUG: Log the request
	c.logger.WithFields(map[string]interface{}{
		"name":      name,
		"filename":  filename,
		"size_kb":   len(nzbData) / 1024,
		"size_bytes": len(nzbData),
	}).Debug("Uploading NZB file to TorBox API")

	req, err := http.NewRequest("POST", torboxAPIBase+"/usenet/createusenetdownload", &buf)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// DEBUG: Log the raw response
	c.logger.WithFields(map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(bodyBytes),
	}).Debug("TorBox API response")

	var result CreateDownloadJobResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return "", nil, fmt.Errorf("job creation failed: %s", result.Detail)
	}

	// Convert usenetdownload_id to string for consistent job_id handling
	jobID := fmt.Sprintf("%d", result.Data.UsenetDownloadID)
	c.logger.WithFields(map[string]interface{}{
		"job_id": jobID,
		"detail": result.Detail, // Log detail to see if cached
	}).Info("Created TorBox download job")
	return jobID, &result, nil
}

// JobStatusResponse represents the response from job status query
type JobStatusResponse struct {
	Success bool   `json:"success"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// GetJobStatus retrieves the status of a download job
func (c *Client) GetJobStatus(jobID string) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/usenet/mylist/%s", torboxAPIBase, jobID), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result JobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Status, nil
}

// ControlUsenetDownload controls a usenet download (delete, pause, etc.)
func (c *Client) ControlUsenetDownload(usenetID int, operation string) error {
	url, err := url.Parse(torboxAPIBase + "/usenet/controlusenetdownload")
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// Create request body
	data := map[string]interface{}{
		"usenet_id": usenetID,
		"operation": operation,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.WithFields(map[string]interface{}{
		"usenet_id": usenetID,
		"operation": operation,
	}).Info("Controlled TorBox usenet download")
	return nil
}

// DeleteJob deletes a download job by ID
func (c *Client) DeleteJob(jobID string) error {
	// Convert jobID string to int
	usenetID, err := strconv.Atoi(jobID)
	if err != nil {
		return fmt.Errorf("invalid job ID: %w", err)
	}

	return c.ControlUsenetDownload(usenetID, "delete")
}

// ListUsenetDownloads retrieves all usenet downloads from TorBox
func (c *Client) ListUsenetDownloads() ([]UsenetDownload, error) {
	req, err := http.NewRequest("GET", torboxAPIBase+"/usenet/mylist", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result UsenetListResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("failed to list downloads: %s", result.Detail)
	}

	return result.Data, nil
}

// FindDownloadByID finds a specific usenet download by its ID
func (c *Client) FindDownloadByID(downloadID int) (*UsenetDownload, error) {
	downloads, err := c.ListUsenetDownloads()
	if err != nil {
		return nil, fmt.Errorf("failed to list downloads: %w", err)
	}

	for _, download := range downloads {
		if download.ID == downloadID {
			return &download, nil
		}
	}

	return nil, fmt.Errorf("download with ID %d not found", downloadID)
}
