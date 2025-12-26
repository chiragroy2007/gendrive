package api

import (
	"encoding/json"
	"net/http"
	"time"

	"p2p-drive/shared"

	"github.com/google/uuid"
)

func (s *Server) CreateFileMetadata(w http.ResponseWriter, r *http.Request) {
	var meta shared.FileMetadata
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if meta.ID == "" {
		meta.ID = uuid.New().String()
	}
	meta.CreatedAt = time.Now()
	meta.UpdatedAt = time.Now()

	tx, err := s.DB.Begin()
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}

	// Insert File
	_, err = tx.Exec("INSERT INTO files (id, path, size, hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		meta.ID, meta.Path, meta.Size, meta.Hash, meta.CreatedAt.Format(time.RFC3339), meta.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		tx.Rollback()
		http.Error(w, "Failed to insert file", http.StatusInternalServerError)
		return
	}

	// Insert Chunks
	for _, chunk := range meta.Chunks {
		if chunk.ID == "" {
			chunk.ID = uuid.New().String()
		}
		_, err = tx.Exec("INSERT INTO chunks (id, file_id, sequence, hash, size) VALUES (?, ?, ?, ?, ?)",
			chunk.ID, meta.ID, chunk.Sequence, chunk.Hash, chunk.Size)
		if err != nil {
			tx.Rollback()
			http.Error(w, "Failed to insert chunk", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Commit failed", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(meta)
}

func (s *Server) GetFileMetadata(w http.ResponseWriter, r *http.Request) {
	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		// List all files
		rows, err := s.DB.Query("SELECT id, path, size, hash, created_at FROM files")
		if err != nil {
			http.Error(w, "DB Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var files []shared.FileMetadata
		for rows.Next() {
			var f shared.FileMetadata
			var createdStr string
			rows.Scan(&f.ID, &f.Path, &f.Size, &f.Hash, &createdStr)
			if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
				f.CreatedAt = t
			}
			files = append(files, f)
		}
		json.NewEncoder(w).Encode(files)
		return
	}

	// Get specific file with chunks
	var f shared.FileMetadata
	var createdStr, updatedStr string
	err := s.DB.QueryRow("SELECT id, path, size, hash, created_at, updated_at FROM files WHERE id = ?", fileID).
		Scan(&f.ID, &f.Path, &f.Size, &f.Hash, &createdStr, &updatedStr)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
		f.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedStr); err == nil {
		f.UpdatedAt = t
	}

	rows, err := s.DB.Query("SELECT id, sequence, hash, size FROM chunks WHERE file_id = ? ORDER BY sequence", fileID)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var c shared.Chunk
		c.FileID = fileID
		rows.Scan(&c.ID, &c.Sequence, &c.Hash, &c.Size)

		// Fetch locations for this chunk
		locRows, err := s.DB.Query("SELECT device_id FROM chunk_locations WHERE chunk_id = ?", c.ID)
		if err == nil {
			for locRows.Next() {
				var devID string
				locRows.Scan(&devID)
				c.Locations = append(c.Locations, devID)
			}
			locRows.Close()
		}

		f.Chunks = append(f.Chunks, c)
	}

	json.NewEncoder(w).Encode(f)
}

func (s *Server) RegisterChunkLocation(w http.ResponseWriter, r *http.Request) {
	var req shared.ChunkLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Upsert equivalent (IGNORE on conflict)
	_, err := s.DB.Exec("INSERT OR IGNORE INTO chunk_locations (chunk_id, device_id) VALUES (?, ?)", req.ChunkID, req.DeviceID)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
