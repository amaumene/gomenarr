package nzbget

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/amaumene/gomenarr/internal/core/ports"
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/rs/zerolog/log"
)

type Client struct {
	cfg        config.NZBGetConfig
	httpClient *http.Client
}

func NewClient(cfg config.NZBGetConfig) *Client {
	// Configure HTTP transport with connection pooling for better performance
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}
}

func (c *Client) QueueDownload(ctx context.Context, nzbContent []byte, filename string, category string, priority int, params map[string]string) (int64, error) {
	// Base64 encode NZB content
	encoded := base64.StdEncoding.EncodeToString(nzbContent)

	// Build parameters array in the correct format: [["key", "value"], ["key2", "value2"]]
	// NZBGet expects PPParameters as an array of arrays, not array of strings
	paramsList := make([][]string, 0, len(params))
	for key, value := range params {
		paramsList = append(paramsList, []string{key, value})
	}

	// JSON-RPC request
	// append(NZBFilename, Content, Category, Priority, AddToTop, AddPaused, DupeKey, DupeScore, DupeMode, PPParameters)
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "append",
		"params":  []interface{}{filename, encoded, category, priority, false, false, "", 0, "ALL", paramsList},
		"id":      1,
	}

	// Debug logging to troubleshoot parameter issues
	log.Debug().
		Str("filename", filename).
		Str("category", category).
		Int("priority", priority).
		Interface("pp_params", paramsList).
		Msg("NZBGet: Queuing download")

	var response struct {
		Result int64 `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.rpc(ctx, request, &response); err != nil {
		return 0, err
	}

	if response.Error != nil {
		log.Error().
			Str("filename", filename).
			Str("error_message", response.Error.Message).
			Interface("params", paramsList).
			Msg("NZBGet: API error")
		return 0, fmt.Errorf("nzbget error: %s", response.Error.Message)
	}

	log.Info().
		Str("filename", filename).
		Int64("nzb_id", response.Result).
		Msg("NZBGet: Download queued successfully")

	return response.Result, nil
}

func (c *Client) GetQueue(ctx context.Context) ([]ports.DownloadQueueItem, error) {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "listgroups",
		"params":  []interface{}{},
		"id":      1,
	}

	var response struct {
		Result []struct {
			NZBID   int64  `json:"NZBID"`
			NZBName string `json:"NZBName"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.rpc(ctx, request, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("nzbget error: %s", response.Error.Message)
	}

	items := make([]ports.DownloadQueueItem, 0, len(response.Result))
	for _, item := range response.Result {
		items = append(items, ports.DownloadQueueItem{
			ID:    item.NZBID,
			Title: item.NZBName,
		})
	}

	return items, nil
}

func (c *Client) GetHistory(ctx context.Context) ([]ports.DownloadHistoryItem, error) {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "history",
		"params":  []interface{}{false},
		"id":      1,
	}

	var response struct {
		Result []struct {
			NZBID   int64  `json:"NZBID"`
			Name    string `json:"Name"`
			Status  string `json:"Status"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.rpc(ctx, request, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("nzbget error: %s", response.Error.Message)
	}

	items := make([]ports.DownloadHistoryItem, 0, len(response.Result))
	for _, item := range response.Result {
		items = append(items, ports.DownloadHistoryItem{
			ID:     item.NZBID,
			Title:  item.Name,
			Status: item.Status,
		})
	}

	return items, nil
}

func (c *Client) DeleteFromHistory(ctx context.Context, downloadID int64) error {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "editqueue",
		"params":  []interface{}{"HistoryDelete", "", []int64{downloadID}},
		"id":      1,
	}

	var response struct {
		Result bool `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.rpc(ctx, request, &response); err != nil {
		return err
	}

	if response.Error != nil {
		return fmt.Errorf("nzbget error: %s", response.Error.Message)
	}

	if !response.Result {
		return fmt.Errorf("failed to delete from history")
	}

	return nil
}

func (c *Client) rpc(ctx context.Context, request interface{}, response interface{}) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}

	// Debug: Log the full JSON-RPC request
	log.Debug().
		RawJSON("request", data).
		Str("url", c.cfg.URL+"/jsonrpc").
		Msg("NZBGet: Sending JSON-RPC request")

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.URL+"/jsonrpc", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("body", string(body)).
			Msg("NZBGet: HTTP error")
		return fmt.Errorf("nzbget HTTP error: %d %s", resp.StatusCode, string(body))
	}

	// Read the response body so we can log it and decode it
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Debug: Log the full JSON-RPC response
	log.Debug().
		RawJSON("response", body).
		Msg("NZBGet: Received JSON-RPC response")

	// Decode the response
	if err := json.Unmarshal(body, response); err != nil {
		return err
	}

	return nil
}
