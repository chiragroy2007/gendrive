package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"p2p-drive/shared"
	"time"
)

func (s *Server) GetFiles(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	rows, err := s.DB.Query("SELECT id, path, size, updated_at FROM files WHERE user_id = ? ORDER BY updated_at DESC", userID)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var files []shared.FileMetadata
	for rows.Next() {
		var f shared.FileMetadata
		var updatedStr string
		rows.Scan(&f.ID, &f.Path, &f.Size, &updatedStr)
		f.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		files = append(files, f)
	}
	json.NewEncoder(w).Encode(files)
}

func (s *Server) DownloadFile(w http.ResponseWriter, r *http.Request) {
	fileID := r.URL.Query().Get("id")
	userID := r.Header.Get("X-User-ID")

	// Verify ownership & Get Metadata
	var path string
	var size int64
	err := s.DB.QueryRow("SELECT path, size FROM files WHERE id = ? AND user_id = ?", fileID, userID).Scan(&path, &size)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", path))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	// Get Chunks
	rows, err := s.DB.Query("SELECT id, sequence, size FROM chunks WHERE file_id = ? ORDER BY sequence ASC", fileID)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var chunkID string
		var seq int
		var cSize int64
		rows.Scan(&chunkID, &seq, &cSize)

		// Find Location
		var deviceID string
		// Pick first online device
		err := s.DB.QueryRow(`
            SELECT d.id FROM devices d 
            JOIN chunk_locations cl ON cl.device_id = d.id 
            WHERE cl.chunk_id = ? AND d.online = 1 LIMIT 1`, chunkID).Scan(&deviceID)

		if err != nil {
			http.Error(w, fmt.Sprintf("Chunk %d missing (no online peers)", seq), http.StatusServiceUnavailable)
			return
		}

		// Request Chunk from Agent
		// Command: RETRIEVE <chunkID>
		// We send this to Agent's Inbox
		reqMsg := shared.RelayMessage{
			Type:    shared.RelayTypeRetrieve,
			Payload: []byte(chunkID),
		}
		reqBytes, _ := json.Marshal(reqMsg)
		s.injectRelayMessage(deviceID, "inbox", reqBytes)

		// Wait for Data
		// Agent sends to "server", session "chunk-<chunkID>"
		data, err := s.waitForRelayData("server", "chunk-"+chunkID, 15*time.Second)
		if err != nil {
			http.Error(w, fmt.Sprintf("Timeout retrieving chunk %d", seq), http.StatusGatewayTimeout)
			return
		}

		w.Write(data)
	}
}

// Helper to wait for data on internal relay channel
func (s *Server) waitForRelayData(to, session string, timeout time.Duration) ([]byte, error) {
	key := to + "-" + session

	// Ensure channel exists
	relayLock.Lock()
	ch, ok := relayChannels[key]
	if !ok {
		ch = make(chan []byte, 1)
		relayChannels[key] = ch
	}
	relayLock.Unlock()

	// Cleanup ensures we don't leak channels forever
	defer func() {
		relayLock.Lock()
		delete(relayChannels, key)
		relayLock.Unlock()
	}()

	select {
	case data := <-ch:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func (s *Server) DeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("id")
	userID := r.Header.Get("X-User-ID")

	// 1. Verify Ownership & Get Info
	var path string
	err := s.DB.QueryRow("SELECT path FROM files WHERE id = ? AND user_id = ?", fileID, userID).Scan(&path)
	if err != nil {
		http.Error(w, "File not found or unauthorized", http.StatusNotFound)
		return
	}

	// 2. Identify Chunks and Locations to notify Agents
	// We want to tell agents to delete these chunks.
	rows, err := s.DB.Query(`
		SELECT cl.device_id, c.id 
		FROM chunks c 
		JOIN chunk_locations cl ON cl.chunk_id = c.id 
		WHERE c.file_id = ?`, fileID)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var deviceID, chunkID string
			if err := rows.Scan(&deviceID, &chunkID); err == nil {
				// Send Delete Command (Async/Best Effort)
				msg := shared.RelayMessage{
					Type:    shared.RelayTypeDelete,
					Payload: []byte(chunkID),
				}
				bytes, _ := json.Marshal(msg)
				s.injectRelayMessage(deviceID, "inbox", bytes)
			}
		}
	}

	// 3. Clean Database (Cascade should handle chunks/locations if set up,
	// but manual cleanup is safer if schema is unsure)
	// Assuming foreign keys or manual cleanup. Let's do manual for safety.

	// Delete Locations (via chunk subquery)
	s.DB.Exec("DELETE FROM chunk_locations WHERE chunk_id IN (SELECT id FROM chunks WHERE file_id = ?)", fileID)
	// Delete Chunks
	s.DB.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)
	// Delete File
	_, err = s.DB.Exec("DELETE FROM files WHERE id = ?", fileID)

	if err != nil {
		http.Error(w, "DB Error during delete", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted"}`))
}
