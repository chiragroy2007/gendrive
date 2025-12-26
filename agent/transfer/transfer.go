package transfer

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
)

const ChunkSize = 8 * 1024 * 1024 // 8MB

// EncryptChunk encrypts data using AES-GCM.
func EncryptChunk(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

// DecryptChunk decrypts data.
func DecryptChunk(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// UploadChunk sends a chunk to a peer via Relay.
func UploadChunk(client *http.Client, serverURL string, peerDeviceID string, sessionID string, data []byte) error {
	// POST /relay/send?to=PEER&session=SESSION
	url := fmt.Sprintf("%s/relay/send?to=%s&session=%s", serverURL, peerDeviceID, sessionID)
	
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay send failed: %d", resp.StatusCode)
	}
	return nil
}

// DownloadChunk requests a chunk from a peer via Relay.
func DownloadChunk(client *http.Client, serverURL string, myDeviceID string, sessionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/relay/recv?me=%s&session=%s", serverURL, myDeviceID, sessionID)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay recv failed: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
