package shared

import "time"

// Device represents a registered device in the mesh.
type Device struct {
	ID        string    `json:"id"`
	PublicKey string    `json:"public_key"`
	Name      string    `json:"name"`
	LastSeen  time.Time `json:"last_seen"`
	IP        string    `json:"ip"` // Last known IP (for potential direct connect optimization later)
	Online    bool      `json:"online"`
    Type      string    `json:"type"` // "agent" or "gdrive"
}

// FileMetadata represents a file tracked by the system.
type FileMetadata struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Hash      string    `json:"hash"` // SHA-256 of the whole file
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Chunks    []Chunk   `json:"chunks,omitempty"`
}

// Chunk represents a piece of a file.
type Chunk struct {
	ID        string   `json:"id"`
	FileID    string   `json:"file_id"`
	Sequence  int      `json:"sequence"`
	Hash      string   `json:"hash"` // SHA-256 of the encrypted chunk
	Size      int64    `json:"size"`
	Locations []string `json:"locations,omitempty"`
}

// RegisterRequest is the payload for device registration.
type RegisterRequest struct {
	DeviceID   string `json:"device_id"` // Optional: Client can suggest ID
	PublicKey  string `json:"public_key"`
	Name       string `json:"name"`
	ClaimToken string `json:"claim_token"`
}

// RegisterResponse is the response for device registration.
type RegisterResponse struct {
	DeviceID string `json:"device_id"`
}

// HeartbeatRequest is the payload for keep-alive.
type HeartbeatRequest struct {
	DeviceID string `json:"device_id"`
}

// RelayProtocol
const (
	RelayTypeStore    = "STORE"
	RelayTypeRequest  = "REQUEST"
	RelayTypeRetrieve = "RETRIEVE"
	RelayTypeDelete   = "DELETE"
	RelayTypeData     = "DATA"
)

type RelayMessage struct {
	Type    string `json:"type"`
	Payload []byte `json:"payload"` // JSON or Raw bytes depending on type
}

// Chunk Location Reporting
type ChunkLocationRequest struct {
	ChunkID  string `json:"chunk_id"`
	DeviceID string `json:"device_id"`
}

// DeletionEvent represents a file deletion event for sync.
type DeletionEvent struct {
	FileID    string    `json:"file_id"`
	ChunkIDs  []string  `json:"chunk_ids"`
	DeletedAt time.Time `json:"deleted_at"`
}

// SortDevicesByLoad sorts devices by chunk count (Least Loaded First)
func SortDevicesByLoad(devices []Device, loads map[string]int) {
	// Simple bubble sort or similar since N is small
	for i := 0; i < len(devices); i++ {
		for j := i + 1; j < len(devices); j++ {
			if loads[devices[j].ID] < loads[devices[i].ID] {
				devices[i], devices[j] = devices[j], devices[i]
			}
		}
	}
}
