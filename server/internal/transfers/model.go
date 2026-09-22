package transfers

import "time"

type Kind string

const (
	KindFile      Kind = "file"
	KindPhoto     Kind = "photo"
	KindLink      Kind = "link"
	KindText      Kind = "text"
	KindClipboard Kind = "clipboard"
)

type Status string

const (
	StatusCreated     Status = "created"
	StatusUploading   Status = "uploading"
	StatusReady       Status = "ready"
	StatusDownloading Status = "downloading"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

type Transfer struct {
	ID                  string    `json:"id"`
	SourceDeviceID      string    `json:"sourceDeviceId"`
	DestinationDeviceID string    `json:"destinationDeviceId"`
	Kind                Kind      `json:"kind"`
	Status              Status    `json:"status"`
	DisplayName         string    `json:"displayName,omitempty"`
	ContentType         string    `json:"contentType,omitempty"`
	SizeBytes           int64     `json:"sizeBytes"`
	SHA256              string    `json:"sha256"`
	UploadURL           string    `json:"uploadUrl,omitempty"`
	DownloadURL         string    `json:"downloadUrl,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type CreateInput struct {
	SourceDeviceID      string `json:"sourceDeviceId"`
	DestinationDeviceID string `json:"destinationDeviceId"`
	Kind                Kind   `json:"kind"`
	DisplayName         string `json:"displayName"`
	ContentType         string `json:"contentType"`
	SizeBytes           int64  `json:"sizeBytes"`
	SHA256              string `json:"sha256"`
}
