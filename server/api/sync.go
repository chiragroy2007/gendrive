package api

import (
	"encoding/json"
	"net/http"
	"time"

	"p2p-drive/shared"
)

func (s *Server) GetDeletions(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		// Default to last 24h if not specified? Or error? 
		// Let's default to a safe recent time if missing to avoid dumping everything
		// Or maybe allow empty to mean "everything ever" (risky).
		// Best practice: if empty, return bad request or very recent.
		// For this MVP, let's treat empty as "file not found" or just empty list to be safe.
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Missing 'since' parameter"))
		return
	}

	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		http.Error(w, "Invalid time format. Use RFC3339", http.StatusBadRequest)
		return
	}

	rows, err := s.DB.Query("SELECT file_id, chunk_ids, deleted_at FROM deleted_files WHERE deleted_at > ?", since.Format(time.RFC3339))
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []shared.DeletionEvent
	for rows.Next() {
		var evt shared.DeletionEvent
		var chunkJSON string
		var delStr string
		if err := rows.Scan(&evt.FileID, &chunkJSON, &delStr); err == nil {
			json.Unmarshal([]byte(chunkJSON), &evt.ChunkIDs)
			evt.DeletedAt, _ = time.Parse(time.RFC3339, delStr)
			events = append(events, evt)
		}
	}

	json.NewEncoder(w).Encode(events)
}
