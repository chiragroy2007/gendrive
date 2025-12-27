package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"p2p-drive/shared"

	"github.com/google/uuid"
)

const ChunkSize = 1024 * 1024 // 1MB

func (s *Server) UploadFile(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse Multipart
	err := r.ParseMultipartForm(512 << 20) // 512MB max memory
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create File Metadata
	fileID := uuid.New().String()
	fileHash := sha256.New()
	var totalSize int64 = 0

	// Get User's Online Devices
	devices, err := s.getUserOnlineDevices(userID)
	if err != nil || len(devices) == 0 {
		http.Error(w, "No online devices found to store chunks", http.StatusServiceUnavailable)
		return
	}

	// DB Transaction? For now, straight inserts.
	_, err = s.DB.Exec("INSERT INTO files (id, user_id, path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		fileID, userID, filepath.Base(header.Filename), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}

	// Chunking Loop
	buffer := make([]byte, ChunkSize)
	sequence := 0

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			http.Error(w, "Read Error", http.StatusInternalServerError)
			return
		}
		if n == 0 {
			break
		}

		chunkData := buffer[:n]
		totalSize += int64(n)
		fileHash.Write(chunkData)

		// Hash Chunk
		sum := sha256.Sum256(chunkData)
		chunkID := hex.EncodeToString(sum[:])

		// Save Chunk Metadata
		_, err = s.DB.Exec("INSERT OR IGNORE INTO chunks (id, file_id, sequence, hash, size) VALUES (?, ?, ?, ?, ?)",
			chunkID, fileID, sequence, chunkID, n)
		if err != nil {
			// Log error
		}

		// Distribute
		sent := false
        // Determine target device by sequence
        // This ensures uniform distribution: 0->DevA, 1->DevB, 2->DevA...
        if len(devices) > 0 {
            targetIndex := sequence % len(devices)
            // We try the target device first. If it fails, we try others.
            // Create a sorted list starting with targetIndex
            sortedDevices := append(devices[targetIndex:], devices[:targetIndex]...)

            for _, device := range sortedDevices {
                if device.Type == "gdrive" {
                    // Upload directly to GDrive
                    err := s.GDrive.UploadChunk(userID, chunkID, chunkData)
                    if err == nil {
                        s.DB.Exec("INSERT OR IGNORE INTO chunk_locations (chunk_id, device_id) VALUES (?, ?)", chunkID, device.ID)
                        sent = true
                        break
                    } else {
                        fmt.Printf("GDrive Upload Failed for chunk %s: %v\n", chunkID, err)
                    }
                } else {
                    // Agent Relay
                    msgBytes, _ := json.Marshal(shared.RelayMessage{Type: shared.RelayTypeStore, Payload: chunkData})
                    if s.injectRelayMessage(device.ID, "inbox", msgBytes) {
                        _, err := s.waitForRelayData("server", "ack-"+chunkID, 30*time.Second)
                        if err == nil {
                            s.DB.Exec("INSERT OR IGNORE INTO chunk_locations (chunk_id, device_id) VALUES (?, ?)", chunkID, device.ID)
                            sent = true
                            break
                        } else {
                            fmt.Printf("Device %s failed to ACK %s\n", device.ID, chunkID)
                        }
                    }
                }
            }
        }

		if !sent {
			http.Error(w, "Failed to store chunk", http.StatusServiceUnavailable)
			return
		}


		sequence++
		if err == io.EOF {
			break
		}
	}

	// Update File Size/Hash
	fullHash := hex.EncodeToString(fileHash.Sum(nil))
	s.DB.Exec("UPDATE files SET size = ?, hash = ? WHERE id = ?", totalSize, fullHash, fileID)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("File uploaded and distributed"))
}

func (s *Server) getUserOnlineDevices(userID string) ([]shared.Device, error) {
	rows, err := s.DB.Query("SELECT id, name, type FROM devices WHERE user_id = ? AND online = 1", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []shared.Device
	for rows.Next() {
		var d shared.Device
		rows.Scan(&d.ID, &d.Name, &d.Type)
		devices = append(devices, d)
	}
	return devices, nil
}

func (s *Server) getDeviceLoads(devices []shared.Device) map[string]int {
	loads := make(map[string]int)
	// Initialize 0
	for _, d := range devices {
		loads[d.ID] = 0
	}
	
	rows, err := s.DB.Query("SELECT device_id, COUNT(*) FROM chunk_locations GROUP BY device_id")
	if err != nil {
		return loads
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err == nil {
			loads[id] = count
		}
	}
	return loads
}

// Ensure we access the global relayChannels from relay.go
// Ensure we access the global relayChannels from relay.go
func (s *Server) injectRelayMessage(to, session string, data []byte) bool {
	key := to + "-" + session
	relayLock.Lock()
	
	ch, ok := relayChannels[key]
	if !ok {
		// Create if not exists with larger buffer
		ch = make(chan []byte, 10) 
		relayChannels[key] = ch

		// Timeout cleanup 
		go func(k string) {
			time.Sleep(60 * time.Second) 
			relayLock.Lock()
			if _, exists := relayChannels[k]; exists {
				delete(relayChannels, k)
			}
			relayLock.Unlock()
		}(key)
	}
	relayLock.Unlock()

	// Try send with timeout
	select {
	case ch <- data:
		return true
	case <-time.After(15 * time.Second): // Increased to 15s for mobile latency
		fmt.Println("Dropped chunk for " + to + " (Buffer Full/Timeout)")
		return false
	}
}
