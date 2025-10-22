package domain

// NotificationStatus represents the status of a download
type NotificationStatus string

const (
	NotificationStatusSuccess NotificationStatus = "SUCCESS"
	NotificationStatusFailure NotificationStatus = "FAILURE"
)

// Notification represents a webhook notification from NZBGet
type Notification struct {
	Status     NotificationStatus
	Name       string
	Path       string
	DownloadID int64
	TraktID    int64
}
