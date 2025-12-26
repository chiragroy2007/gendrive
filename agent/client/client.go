package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"p2p-drive/shared"
)

type Client struct {
	ServerURL string
	ID        string
	Client    *http.Client
}

func NewClient(serverURL, deviceID string) *Client {
	// Custom Dialer to force Google DNS (fixes Termux/Android DNS issues)
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return net.Dial("udp", "8.8.8.8:53")
			},
		},
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}

	return &Client{
		ServerURL: serverURL,
		ID:        deviceID,
		Client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) Register(publicKey, name, deviceID, claimToken string) (string, error) {
	req := shared.RegisterRequest{
		DeviceID:   deviceID,
		PublicKey:  publicKey,
		Name:       name,
		ClaimToken: claimToken,
	}
	body, _ := json.Marshal(req)

	resp, err := c.Client.Post(c.ServerURL+"/register", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	var res shared.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.DeviceID, nil
}

func (c *Client) SendHeartbeat() error {
	req := shared.HeartbeatRequest{DeviceID: c.ID}
	body, _ := json.Marshal(req)

	resp, err := c.Client.Post(c.ServerURL+"/heartbeat", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) StartHeartbeat(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if err := c.SendHeartbeat(); err != nil {
				fmt.Printf("Heartbeat error: %v\n", err)
			}
		}
	}()
}

func (c *Client) CreateFileMetadata(meta shared.FileMetadata) error {
	body, _ := json.Marshal(meta)
	resp, err := c.Client.Post(c.ServerURL+"/metadata", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metadata create failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) GetFileMetadata(id string) (*shared.FileMetadata, error) {
	resp, err := c.Client.Get(c.ServerURL + "/metadata?id=" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get metadata failed: %d", resp.StatusCode)
	}

	var meta shared.FileMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (c *Client) ReportChunkLocation(chunkID string) error {
	req := shared.ChunkLocationRequest{
		ChunkID:  chunkID,
		DeviceID: c.ID,
	}

	body, _ := json.Marshal(req)
	resp, err := c.Client.Post(c.ServerURL+"/chunk/location", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("report location failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) GetPeers() ([]shared.Device, error) {
	resp, err := c.Client.Get(c.ServerURL + "/peers")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var peers []shared.Device
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func (c *Client) RelaySend(to, session string, data []byte) error {
	url := fmt.Sprintf("%s/relay/send?to=%s&session=%s", c.ServerURL, to, session)
	resp, err := c.Client.Post(url, "application/octet-stream", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay send failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) GetDeletions(since time.Time) ([]shared.DeletionEvent, error) {
	url := fmt.Sprintf("%s/api/sync/deletions?since=%s", c.ServerURL, since.Format(time.RFC3339))
	resp, err := c.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get deletions failed: %d", resp.StatusCode)
	}

	var events []shared.DeletionEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}
	return events, nil
}
