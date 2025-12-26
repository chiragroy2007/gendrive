package files

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"p2p-drive/agent/client"
	"p2p-drive/agent/transfer"
	"p2p-drive/shared"
)

type Manager struct {
	Client  *client.Client
	DataDir string
	ID      *shared.Device
}

func (m *Manager) UploadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	meta := shared.FileMetadata{
		Path: filepath.Base(path),
		Size: info.Size(),
	}

	// 1. Calculate Hash
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	meta.Hash = hex.EncodeToString(hash.Sum(nil))
	file.Seek(0, 0)

	// 2. Select Peers for Replication (Factor 2)
	peers, err := m.Client.GetPeers()
	if err != nil {
		return fmt.Errorf("failed to get peers: %v", err)
	}
	var candidates []shared.Device
	for _, p := range peers {
		if p.ID != m.ID.ID { // Don't upload to self via relay (we are Uploader)
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no peers available for storage")
	}

	// Shuffle and pick 2
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	targetPeers := candidates
	if len(targetPeers) > 2 {
		targetPeers = targetPeers[:2]
	}

	fmt.Printf("Replicating file to %d peers: %v\n", len(targetPeers), targetPeers)

	// 3. Chunk and Upload
	buffer := make([]byte, transfer.ChunkSize)
	sequence := 0
	key := []byte("0123456789ABCDEF0123456789ABCDEF")

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		chunkData := buffer[:n]
		encrypted, err := transfer.EncryptChunk(chunkData, key)
		if err != nil {
			return fmt.Errorf("encrypt error: %v", err)
		}

		chunkHash := sha256.Sum256(encrypted)
		targetSession := "inbox"

		// Wrap in STORE message
		msg := shared.RelayMessage{
			Type:    shared.RelayTypeStore,
			Payload: encrypted,
		}
		msgBytes, _ := json.Marshal(msg)

		// Send to ALL targets
		for _, peer := range targetPeers {
			if err := transfer.UploadChunk(m.Client.ServerURL, peer.ID, targetSession, msgBytes); err != nil {
				fmt.Printf("Warning: Failed to upload chunk to %s: %v\n", peer.ID, err)
				// Continue trying other peers?
			}
		}

		meta.Chunks = append(meta.Chunks, shared.Chunk{
			Sequence: sequence,
			Hash:     hex.EncodeToString(chunkHash[:]),
			Size:     int64(len(encrypted)),
		})
		sequence++
	}

	// 4. Send Metadata
	if err := m.Client.CreateFileMetadata(meta); err != nil {
		return fmt.Errorf("failed to publish metadata: %v", err)
	}

	fmt.Printf("File uploaded successfully: %s\n", meta.Path)
	return nil
}
