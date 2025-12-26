package api

import (
	"encoding/json"
	"log"
	"time"

	"p2p-drive/shared"
)

func (s *Server) TriggerRebalance(userID string) {
	go func() {
		log.Printf("[Rebalance] Started for user %s", userID)

		// 1. Get Devices & Usage
		rows, err := s.DB.Query(`
            SELECT d.id, COALESCE(SUM(c.size), 0) 
            FROM devices d
            LEFT JOIN chunk_locations cl ON d.id = cl.device_id
            LEFT JOIN chunks c ON cl.chunk_id = c.id
            WHERE d.user_id = ? AND d.online = 1
            GROUP BY d.id`, userID)
		if err != nil {
			log.Printf("[Rebalance] Error getting usage: %v", err)
			return
		}
		defer rows.Close()

		type DeviceLoad struct {
			ID    string
			Usage int64
		}
		var loads []DeviceLoad
		var totalUsage int64

		for rows.Next() {
			var dl DeviceLoad
			rows.Scan(&dl.ID, &dl.Usage)
			loads = append(loads, dl)
			totalUsage += dl.Usage
		}

		if len(loads) < 2 || totalUsage == 0 {
			log.Println("[Rebalance] Not enough devices or data to rebalance.")
			return
		}

		avgLoad := totalUsage / int64(len(loads))
		thresholdHigh := int64(float64(avgLoad) * 1.2)
		thresholdLow := int64(float64(avgLoad) * 0.8)

		log.Printf("[Rebalance] Total: %d, Avg: %d, High: %d, Low: %d", totalUsage, avgLoad, thresholdHigh, thresholdLow)

		// 2. Identify Moves
		for _, source := range loads {
			if source.Usage > thresholdHigh {
				// Find target
				for _, target := range loads {
					if target.Usage < thresholdLow && target.ID != source.ID {
						// Calculate amount to move
						toMove := source.Usage - avgLoad
						log.Printf("[Rebalance] Moving %d bytes from %s to %s", toMove, source.ID, target.ID)

						s.moveChunks(source.ID, target.ID, toMove)

						// Update local tracking to avoid double-moving in this pass
						source.Usage -= toMove
						target.Usage += toMove
					}
				}
			}
		}
		log.Println("[Rebalance] Completed.")
	}()
}

func (s *Server) moveChunks(sourceID, targetID string, amount int64) {
	// Get chunks from source
	rows, err := s.DB.Query(`
        SELECT c.id, c.size FROM chunks c
        JOIN chunk_locations cl ON c.id = cl.chunk_id
        WHERE cl.device_id = ? LIMIT 50`, sourceID) // Process batch
	if err != nil {
		return
	}
	defer rows.Close()

	var moved int64 = 0
	for rows.Next() {
		if moved >= amount {
			break
		}

		var chunkID string
		var size int64
		rows.Scan(&chunkID, &size)

		// 1. Retrieve from Source
		// Send RETRIEVE to Source
		reqMsg := shared.RelayMessage{Type: shared.RelayTypeRetrieve, Payload: []byte(chunkID)}
		reqBytes, _ := json.Marshal(reqMsg)
		s.injectRelayMessage(sourceID, "inbox", reqBytes)

		// Wait for Data
		data, err := s.waitForRelayData("server", "chunk-"+chunkID, 10*time.Second)
		if err != nil {
			log.Printf("[Rebalance] Failed to retrieve chunk %s: %v", chunkID, err)
			continue
		}

		// 2. Store to Target
		// Send STORE to Target
		storeMsg := shared.RelayMessage{Type: shared.RelayTypeStore, Payload: data}
		storeBytes, _ := json.Marshal(storeMsg)
		s.injectRelayMessage(targetID, "inbox", storeBytes)

		// Wait/Assume success (UDP-like for now, but ideally we wait for ack)
		// For prototype, we update DB immediately.

		// 3. Update DB
		_, err = s.DB.Exec("INSERT OR IGNORE INTO chunk_locations (chunk_id, device_id) VALUES (?, ?)", chunkID, targetID)
		if err != nil {
			continue
		}

		_, err = s.DB.Exec("DELETE FROM chunk_locations WHERE chunk_id = ? AND device_id = ?", chunkID, sourceID)
		if err != nil {
			continue
		}

		// 4. Delete from Source
		delMsg := shared.RelayMessage{Type: shared.RelayTypeDelete, Payload: []byte(chunkID)}
		delBytes, _ := json.Marshal(delMsg)
		s.injectRelayMessage(sourceID, "inbox", delBytes)

		log.Printf("[Rebalance] Moved chunk %s (%d bytes)", chunkID, size)
		moved += size
	}
}
