package api

import (
	"encoding/json"
	"net/http"
	"p2p-drive/shared"
	"time"
)

// ClaimDeviceRequest
type ClaimDeviceRequest struct {
	DeviceID   string `json:"device_id"`
	ClaimToken string `json:"claim_token"`
}

func (s *Server) ClaimDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ClaimDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Verify Token & Device
	res, err := s.DB.Exec("UPDATE devices SET user_id = ? WHERE id = ? AND claim_token = ?",
		userID, req.DeviceID, req.ClaimToken)

	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		http.Error(w, "Invalid Device ID or Claim Token", http.StatusForbidden)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetMyDevices(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	rows, err := s.DB.Query("SELECT id, name, last_seen, online, ip FROM devices WHERE user_id = ?", userID)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var devices []shared.Device
	for rows.Next() {
		var d shared.Device
		var lastSeenStr string
		rows.Scan(&d.ID, &d.Name, &lastSeenStr, &d.Online, &d.IP)
		d.LastSeen, _ = time.Parse(time.RFC3339, lastSeenStr)
		devices = append(devices, d)
	}
	json.NewEncoder(w).Encode(devices)
}
