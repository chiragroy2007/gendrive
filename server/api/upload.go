package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
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
	err := r.ParseMultipartForm(100 << 20) // 100MB max memory
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
			// Log error but maybe continue
		}

		// Select Device (Round Robin or Random)
		device := devices[rand.Intn(len(devices))]

		// Record Location
		_, err = s.DB.Exec("INSERT OR IGNORE INTO chunk_locations (chunk_id, device_id) VALUES (?, ?)",
			chunkID, device.ID)

		// Distribute via Relay
		// We construct a specific RelayMessage for the Agent
		relayMsg := shared.RelayMessage{
			Type:    shared.RelayTypeStore,
			Payload: chunkData,
		}
		msgBytes, _ := json.Marshal(relayMsg)

		// Inject into Relay Channel
		s.injectRelayMessage(device.ID, "inbox", msgBytes)

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
	rows, err := s.DB.Query("SELECT id, name FROM devices WHERE user_id = ? AND online = 1", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []shared.Device
	for rows.Next() {
		var d shared.Device
		rows.Scan(&d.ID, &d.Name)
		devices = append(devices, d)
	}
	return devices, nil
}

// Ensure we access the global relayChannels from relay.go
func (s *Server) injectRelayMessage(to, session string, data []byte) {
	key := to + "-" + session
	relayLock.Lock()
	defer relayLock.Unlock()

	ch, ok := relayChannels[key]
	if !ok {
		// Create if not exists, but usually receiver creates it by polling first?
		// Our Receiver polls -> RelayRecv -> creates channel.
		// If receiver hasn't polled yet, we create it here and wait for them to poll.
		ch = make(chan []byte, 10) // Buffer 10 chunks
		relayChannels[key] = ch

		// Timeout cleanup (Copied from RelaySend logic)
		go func(k string) {
			time.Sleep(60 * time.Second) // Give agent time to poll
			relayLock.Lock()
			if _, exists := relayChannels[k]; exists {
				// Don't close if actively used?
				// Simple timeout for now.
				// Re-closing might panic if already closed, but we check existence.
				// close(relayChannels[k]) // Closing might cause send panic if concurrent.
				// For simple prototype, rely on GC/delete.
				// Or better: don't close, just delete.
				// If we delete, sender re-creates.
				delete(relayChannels, k)
			}
			relayLock.Unlock()
		}(key)
	}

	// Non-blocking send or timeout?
	select {
	case ch <- data:
	case <-time.After(5 * time.Second):
		// Drop packet if agent full
		fmt.Println("Dropped chunk for " + to)
	}
}
