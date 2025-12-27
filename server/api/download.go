package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"p2p-drive/shared"
	"time"

	"github.com/google/uuid"
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

		// Find All Locations for this chunk
		var devices []string
		locRows, err := s.DB.Query(`
            SELECT d.id FROM devices d 
            JOIN chunk_locations cl ON cl.device_id = d.id 
            WHERE cl.chunk_id = ? AND d.online = 1`, chunkID)
		
		if err == nil {
			for locRows.Next() {
				var did string
				locRows.Scan(&did)
				devices = append(devices, did)
			}
			locRows.Close()
		}

		if len(devices) == 0 {
			http.Error(w, fmt.Sprintf("Chunk %d missing (no online peers)", seq), http.StatusServiceUnavailable)
			return
		}

		// Try to Retrieve from any available device
		var chunkData []byte
		success := false

		for _, deviceID := range devices {
            // Get Device Type (Optimization: fetch type in previous query or query here)
            // Let's query type here for safety or refactor previous query. 
            // Previous query just returned 'id'. Let's refactor previous query.
            var dType string
            s.DB.QueryRow("SELECT type FROM devices WHERE id = ?", deviceID).Scan(&dType)

            if dType == "gdrive" {
                data, err := s.GDrive.DownloadChunk(userID, chunkID)
                if err == nil {
                    chunkData = data
                    success = true
                    break
                } else {
                    fmt.Printf("GDrive Download Failed for chunk %s: %v\n", chunkID, err)
                }
            } else {
                // Request Chunk
                reqMsg := shared.RelayMessage{
                    Type:    shared.RelayTypeRetrieve,
                    Payload: []byte(chunkID),
                }
                reqBytes, _ := json.Marshal(reqMsg)
                
                // Try to inject (skip if buffer full)
                if !s.injectRelayMessage(deviceID, "inbox", reqBytes) {
                    continue
                }

                // Wait for Data
                data, err := s.waitForRelayData("server", "chunk-"+chunkID, 15*time.Second)
                if err == nil {
                    chunkData = data
                    success = true
                    break
                }
            }
		}

		if !success {
            fmt.Printf("Failed to retrieve chunk %d (%s) from any peer\n", seq, chunkID)
			http.Error(w, fmt.Sprintf("Failed to retrieve chunk %d from any peer", seq), http.StatusGatewayTimeout)
			return
		}

		w.Write(chunkData)
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
	var chunkIDs []string
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
                // Check if GDrive (could query DB or check ID prefix, ID prefix is faster/cheaper if consistent)
                // We set ID as "GDrive-"+UserID in gdrive.go
                // Let's rely on DB type check for correctness or just try/catch?
                // DB Type check is cleaner.
                var dType string
                s.DB.QueryRow("SELECT type FROM devices WHERE id = ?", deviceID).Scan(&dType)

                if dType == "gdrive" {
                    // Delete from GDrive
                    // We need the folderID? GDriveManager handles lookup.
                    // We need userID for the client.
                    s.GDrive.DeleteChunk(userID, chunkID)
                } else {
				    // Send Delete Command (Async/Best Effort) to Agent
				    msg := shared.RelayMessage{
					    Type:    shared.RelayTypeDelete,
					    Payload: []byte(chunkID),
				    }
				    bytes, _ := json.Marshal(msg)
				    s.injectRelayMessage(deviceID, "inbox", bytes)
                }
			}
		}
	}

	// 2.1 Collect Chunk IDs for Offline Sync
	// We need a list of ALL chunks belonging to this file, regardless of location
	chunkRows, err := s.DB.Query("SELECT id FROM chunks WHERE file_id = ?", fileID)
	if err == nil {
		defer chunkRows.Close()
		for chunkRows.Next() {
			var cid string
			chunkRows.Scan(&cid)
			chunkIDs = append(chunkIDs, cid)
		}
	}

	// 2.2 Record in deleted_files table
	chunkJSON, _ := json.Marshal(chunkIDs)
	s.DB.Exec("INSERT INTO deleted_files (id, file_id, chunk_ids, deleted_at) VALUES (?, ?, ?, ?)",
		uuid.New().String(), fileID, string(chunkJSON), time.Now().Format(time.RFC3339))


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
