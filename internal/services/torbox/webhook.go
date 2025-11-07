package torbox

import (
	"fmt"
	"regexp"
	"time"
)

// WebhookPayload represents the webhook payload from TorBox
type WebhookPayload struct {
	Type      string           `json:"type"`
	Timestamp time.Time        `json:"timestamp"`
	Data      NotificationData `json:"data"`
}

// NotificationData contains the notification details
type NotificationData struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// ExtractDownloadName extracts the download name from the notification message
// Message format: "download Bosch.Legacy.S03E01.720p has completed"
func (p *WebhookPayload) ExtractDownloadName() (string, error) {
	const regexPattern = `download (.+?) has`
	re := regexp.MustCompile(regexPattern)
	match := re.FindStringSubmatch(p.Data.Message)
	if len(match) < 2 {
		return "", fmt.Errorf("failed to extract download name from message: %s", p.Data.Message)
	}
	return match[1], nil
}

// ExtractHash extracts the hash from the notification message
// Message format: "The NZB with hash 5048ac7b66712696b0c2d06b3e14066a failed to download..."
func (p *WebhookPayload) ExtractHash() (string, error) {
	const regexPattern = `hash ([a-f0-9]{32})`
	re := regexp.MustCompile(regexPattern)
	match := re.FindStringSubmatch(p.Data.Message)
	if len(match) < 2 {
		return "", fmt.Errorf("failed to extract hash from message: %s", p.Data.Message)
	}
	return match[1], nil
}

// GetStatus returns the download status based on the title
func (p *WebhookPayload) GetStatus() string {
	switch p.Data.Title {
	case "Usenet Download Completed":
		return "completed"
	case "Usenet Download Failed":
		return "failed"
	default:
		return "unknown"
	}
}

// ShouldRestart returns true if the download failed and should be restarted
func (p *WebhookPayload) ShouldRestart() bool {
	return p.Data.Title == "Usenet Download Failed"
}
