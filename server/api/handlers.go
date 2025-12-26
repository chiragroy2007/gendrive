package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"p2p-drive/shared"

	"github.com/google/uuid"
)

type Server struct {
	DB *sql.DB
}

func NewServer(db *sql.DB) *Server {
	return &Server{DB: db}
}

func (s *Server) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req shared.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = uuid.New().String()
	}

	// Check if exists
	// Check if exists
	var exists int
	s.DB.QueryRow("SELECT 1 FROM devices WHERE id = ?", deviceID).Scan(&exists)
	if exists == 1 {
		// Already registered, just update details
		_, err := s.DB.Exec("UPDATE devices SET public_key=?, name=?, last_seen=?, online=?, ip=?, claim_token=? WHERE id=?",
			req.PublicKey, req.Name, time.Now().Format(time.RFC3339), true, r.RemoteAddr, req.ClaimToken, deviceID)
		if err != nil {
			log.Printf("Error updating device: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		_, err := s.DB.Exec("INSERT INTO devices (id, public_key, name, last_seen, online, ip, claim_token) VALUES (?, ?, ?, ?, ?, ?, ?)",
			deviceID, req.PublicKey, req.Name, time.Now().Format(time.RFC3339), true, r.RemoteAddr, req.ClaimToken)
		if err != nil {
			log.Printf("Error registering device: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	resp := shared.RegisterResponse{DeviceID: deviceID}
	json.NewEncoder(w).Encode(resp)

	// Trigger Rebalance
	var userID string
	s.DB.QueryRow("SELECT user_id FROM devices WHERE id = ?", deviceID).Scan(&userID)
	if userID != "" {
		s.TriggerRebalance(userID)
	}
}

func (s *Server) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req shared.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.DB.Exec("UPDATE devices SET last_seen = ?, online = ? WHERE id = ?", time.Now().Format(time.RFC3339), true, req.DeviceID)
	if err != nil {
		log.Printf("Error updating heartbeat: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetPeers(w http.ResponseWriter, r *http.Request) {
	// Return list of online peers (excluding self if requester ID provided? For now simply all online)
	// In a real app we'd filter by user, but this is single-user.
	rows, err := s.DB.Query("SELECT id, public_key, name, last_seen, ip, online FROM devices WHERE online = 1")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var devices []shared.Device
	for rows.Next() {
		var d shared.Device
		var lastSeenStr string
		if err := rows.Scan(&d.ID, &d.PublicKey, &d.Name, &lastSeenStr, &d.IP, &d.Online); err != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, lastSeenStr); err == nil {
			d.LastSeen = t
		}

		// Dynamic Online Check
		if time.Since(d.LastSeen) > 30*time.Second {
			d.Online = false
		}

		devices = append(devices, d)
	}

	json.NewEncoder(w).Encode(devices)
}
