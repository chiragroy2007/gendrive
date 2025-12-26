package uiserver

import (
	"log"
	"net/http"

	"p2p-drive/agent/client"
	"p2p-drive/agent/files"
)

type UIServer struct {
	Client  *client.Client
	Manager *files.Manager
	Port    string
	WebDir  string
}

func New(client *client.Client, manager *files.Manager, port string, webDir string) *UIServer {
	return &UIServer{
		Client:  client,
		Manager: manager,
		Port:    port,
		WebDir:  webDir,
	}
}

func (s *UIServer) Start() {
	mux := http.NewServeMux()

	// API
	mux.HandleFunc("/api/upload", s.handleUpload)

	// Static
	fs := http.FileServer(http.Dir(s.WebDir))
	mux.Handle("/", fs)

	log.Printf("Agent UI starting on http://localhost:%s", s.Port)
	if err := http.ListenAndServe(":"+s.Port, mux); err != nil {
		log.Printf("Agent UI Error: %v", err)
	}
}

func (s *UIServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.FormValue("file_path")

	if filePath == "" {
		http.Error(w, "Missing file_path", http.StatusBadRequest)
		return
	}

	go func() {
		log.Printf("Starting distributed upload of %s", filePath)
		if err := s.Manager.UploadFile(filePath); err != nil {
			log.Printf("Upload failed: %v", err)
		} else {
			log.Printf("Upload success: %s", filePath)
		}
	}()

	w.Write([]byte("Distributed upload started. Check logs."))
}
