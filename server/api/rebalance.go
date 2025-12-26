package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"p2p-drive/shared"
)

// RebalanceHandler triggers the rebalancing process manually
func (s *Server) RebalanceHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	go s.TriggerRebalance(userID) 
	w.Write([]byte("Rebalancing started in background"))
}

func (s *Server) TriggerRebalance(userID string) {
	log.Printf("Starting Cluster Rebalance for user %s...", userID)
	// 1. Get all online devices
	devices, err := s.getUserOnlineDevices(userID)
	if err != nil || len(devices) < 2 {
		log.Println("Not enough devices to rebalance")
		return
	}

	// 2. Get all chunk locations
	// Map: ChunkID -> []DeviceID
	rows, err := s.DB.Query("SELECT chunk_id, device_id FROM chunk_locations")
	if err != nil {
		log.Println("DB Error:", err)
		return
	}
	defer rows.Close()

	chunkLocs := make(map[string][]string)
	deviceLoad := make(map[string]int) // DeviceID -> ChunkCount

	for _, d := range devices {
		deviceLoad[d.ID] = 0 // Init
	}

	for rows.Next() {
		var c, d string
		rows.Scan(&c, &d)
		chunkLocs[c] = append(chunkLocs[c], d)
		if _, ok := deviceLoad[d]; ok {
			deviceLoad[d]++
		}
	}

	// 3. Calculate Target Load
	totalChunks := 0
	for _, count := range deviceLoad {
		totalChunks += count
	}
	targetLoad := totalChunks / len(devices)
	fmt.Printf("Total Chunks: %d, Devices: %d, Target: %d\n", totalChunks, len(devices), targetLoad)

	// 4. Move Chunks
	// Simple greedy approach: Take from overloaded, give to underloaded
	for devID, count := range deviceLoad {
		if count > targetLoad {
			toMove := count - targetLoad
			// Find chunks on this device to move
			moved := 0
			// Iterate all chunks to find ones on this device (inefficient but works for now)
			for chunkID, locs := range chunkLocs {
				if moved >= toMove {
					break
				}
				// Check if this chunk is ON this device
				hasIt := false
				for _, d := range locs {
					if d == devID {
						hasIt = true
						break
					}
				}

				if hasIt {
					// Find the best target device (lowest load)
					var targetDev string
					minLoad := 999999
					
					for d, c := range deviceLoad {
						if c < minLoad {
							minLoad = c
							targetDev = d
						}
					}

					// Only move if target has less than source (prevent ping-pong if equal)
					// And only if target is significantly better or we are overloaded
					if targetDev != "" && minLoad < count {
						log.Printf("Moving chunk %s from %s (load %d) to %s (load %d)", chunkID, devID, count, targetDev, minLoad)
						if s.MoveChunk(chunkID, devID, targetDev) {
							// Update local state
							deviceLoad[devID]--
							deviceLoad[targetDev]++
							moved++
						}
					}
				}
			}
		}
	}
	log.Println("Rebalance Complete")
}

func (s *Server) MoveChunk(chunkID, sourceDev, targetDev string) bool {
	// 1. Request Retrieve from Source
	reqMsg := shared.RelayMessage{
		Type:    shared.RelayTypeRetrieve,
		Payload: []byte(chunkID),
	}
	reqBytes, _ := json.Marshal(reqMsg)
	s.injectRelayMessage(sourceDev, "inbox", reqBytes)

	// 2. Wait for Source to send back to "server" on session "chunk-{id}"
	key := "server-chunk-" + chunkID
	
	// Create channel in map to listen
	relayLock.Lock()
	ch := make(chan []byte, 1)
	relayChannels[key] = ch
	relayLock.Unlock()

	defer func() {
		relayLock.Lock()
		delete(relayChannels, key)
		relayLock.Unlock()
	}()

	var chunkData []byte
	select {
	case data := <-ch:
		chunkData = data
	case <-time.After(10 * time.Second):
		log.Printf("Timeout receiving chunk %s from %s", chunkID, sourceDev)
		return false
	}

	// 3. Send Store to Target
	storeMsg := shared.RelayMessage{
		Type:    shared.RelayTypeStore,
		Payload: chunkData,
	}
	storeBytes, _ := json.Marshal(storeMsg)
	s.injectRelayMessage(targetDev, "inbox", storeBytes)

	// 4. Update DB
	// We assume success for now, or could wait for ack?
	// Realistically, we should wait for "Location Reported" but that's async.
	// We'll update DB optimistically or wait?
	// Let's just update DB.
	s.DB.Exec("INSERT OR IGNORE INTO chunk_locations (chunk_id, device_id) VALUES (?, ?)", chunkID, targetDev)
	
	// 5. Delete from Source
	delMsg := shared.RelayMessage{
		Type:    shared.RelayTypeDelete,
		Payload: []byte(chunkID),
	}
	delBytes, _ := json.Marshal(delMsg)
	s.injectRelayMessage(sourceDev, "inbox", delBytes)
	
	s.DB.Exec("DELETE FROM chunk_locations WHERE chunk_id = ? AND device_id = ?", chunkID, sourceDev)

	return true
}
