package api

import (
	"io"
	"net/http"
	"sync"
	"time"
)

// Simple in-memory relay map: Token -> Channel
// Agent A posts to /relay/send?to=AgentB -> Server stores in map
// Agent B requests /relay/recv?from=AgentA -> Server streams from map
// Or better: Agent B long-polls or we use a temporary buffer.
// Given constraints: "transfer chunks via a simple relay through the control server"
// "The control server must never store file data"
// So we must pipe directly or use short-lived buffer in memory.
// Let's use a channel map.

var relayChannels = make(map[string]chan []byte)
var relayLock sync.Mutex

func (s *Server) RelaySend(w http.ResponseWriter, r *http.Request) {
	// Receiver Device ID
	to := r.URL.Query().Get("to")
	// Sender Device ID (authenticated? For now assume trusted or in header)
	// In real app, we check Auth header.

	if to == "" {
		http.Error(w, "Missing 'to' param", http.StatusBadRequest)
		return
	}

	// Read body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// We need a unique session ID or just pipe by device pair?
	// If multiple concurrent transfers, we need session ID.
	// Let's expect a "session" param.
	session := r.URL.Query().Get("session")
	if session == "" {
		http.Error(w, "Missing 'session' param", http.StatusBadRequest)
		return
	}

	key := to + "-" + session

	relayLock.Lock()
	ch, ok := relayChannels[key]
	if !ok {
		// Create channel if receiver is waiting?
		// Or fail if receiver not ready?
		// Better: Buffered channel, allow sender to push.
		ch = make(chan []byte, 1)
		relayChannels[key] = ch

		// Timeout to cleanup if no receiver
		go func(k string) {
			time.Sleep(30 * time.Second)
			relayLock.Lock()
			if _, exists := relayChannels[k]; exists {
				close(relayChannels[k])
				delete(relayChannels, k)
			}
			relayLock.Unlock()
		}(key)
	}
	relayLock.Unlock()

	select {
	case ch <- data:
		w.WriteHeader(http.StatusOK)
	case <-time.After(10 * time.Second):
		http.Error(w, "Timeout waiting for receiver", http.StatusGatewayTimeout)
	}
}

func (s *Server) RelayRecv(w http.ResponseWriter, r *http.Request) {
	// Current Device ID (me)
	// In real app, derived from Auth.
	to := r.URL.Query().Get("me")
	session := r.URL.Query().Get("session")

	if to == "" || session == "" {
		http.Error(w, "Missing params", http.StatusBadRequest)
		return
	}

	key := to + "-" + session

	relayLock.Lock()
	ch, ok := relayChannels[key]
	if !ok {
		ch = make(chan []byte, 10)
		relayChannels[key] = ch
	}
	relayLock.Unlock()

	// Wait for data
	select {
	case data, open := <-ch:
		if !open {
			http.Error(w, "Channel closed", http.StatusGone)
			return
		}
		w.Write(data)

		// Cleanup after one chunk?
		// "Chunks into fixed-size ... transferring chunks".
		// Assuming one request per chunk.
		relayLock.Lock()
		delete(relayChannels, key)
		relayLock.Unlock()

	case <-time.After(30 * time.Second):
		http.Error(w, "Timeout waiting for sender", http.StatusGatewayTimeout)
	}
}
