package bg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"p2p-drive/agent/client"
	"p2p-drive/agent/storage"
	"p2p-drive/agent/transfer"
	"p2p-drive/shared"
)

type Receiver struct {
	Client *client.Client
	Store  *storage.Store
	MyID   string
}

func NewReceiver(c *client.Client, s *storage.Store, myID string) *Receiver {
	return &Receiver{Client: c, Store: s, MyID: myID}
}

func (r *Receiver) Start() {
	go r.loop()
}

func (r *Receiver) loop() {
	session := "inbox"

	for {
		// Poll Relay
		data, err := transfer.DownloadChunk(r.Client.ServerURL, r.MyID, session)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		// Decode Message
		var msg shared.RelayMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("Received invalid message: %v", err)
			continue
		}

		r.handleMessage(msg)
	}
}

func (r *Receiver) handleMessage(msg shared.RelayMessage) {
	if msg.Type == shared.RelayTypeStore {
		r.handleStore(msg.Payload)
	} else if msg.Type == shared.RelayTypeRetrieve {
		r.handleRetrieve(msg.Payload)
	} else if msg.Type == shared.RelayTypeDelete {
		r.handleDelete(msg.Payload)
	}
}

func (r *Receiver) handleDelete(data []byte) {
	chunkID := string(data)
	log.Printf("Deleting chunk: %s", chunkID)
	if err := r.Store.DeleteChunk(chunkID); err != nil {
		log.Printf("Failed to delete chunk %s: %v", chunkID, err)
	}
}

func (r *Receiver) handleRetrieve(data []byte) {
	chunkID := string(data)
	log.Printf("Retrieving chunk request: %s", chunkID)

	// 1. Get Chunk
	chunkData, err := r.Store.GetChunk(chunkID)
	if err != nil {
		log.Printf("Chunk not found: %s", chunkID)
		return
	}

	// 2. Send back to Server
	// Target: "server", Session: "chunk-{id}"
	// We use a new helper in Client or raw request?
	// Let's add RelaySend to Client.
	err = r.Client.RelaySend("server", "chunk-"+chunkID, chunkData)
	if err != nil {
		log.Printf("Failed to send chunk %s: %v", chunkID, err)
	}
}

func (r *Receiver) handleStore(data []byte) {
	// 1. Calculate Hash (chunkID)
	hash := sha256.Sum256(data)
	chunkID := hex.EncodeToString(hash[:])

	// 2. Save to Store
	if err := r.Store.SaveChunk(chunkID, data); err != nil {
		log.Printf("Failed to save chunk %s: %v", chunkID, err)
		return
	}
	log.Printf("Stored chunk: %s", chunkID)

	// 3. Report to Server
	if err := r.Client.ReportChunkLocation(chunkID); err != nil {
		log.Printf("Failed to report chunk location: %v", err)
	}
}
