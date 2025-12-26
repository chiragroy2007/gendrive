package main

import (
	"log"
	"net/http"
	"os"

	"p2p-drive/server/api"
	"p2p-drive/server/db"
)

func main() {
	// Ensure data directory exists
	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatal(err)
	}

	database := db.InitDB("./data/p2p.db")
	server := api.NewServer(database)
	authHandler := api.NewAuthHandler(database)

	// Public Auth
	http.HandleFunc("/api/signup", authHandler.Signup)
	http.HandleFunc("/api/login", authHandler.Login)

	// Protected Routes Wrapper
	auth := authHandler.Middleware

	http.HandleFunc("/api/me", auth(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-ID")
		var email string
		database.QueryRow("SELECT email FROM users WHERE id = ?", userID).Scan(&email)
		w.Write([]byte(`{"id":"` + userID + `", "email":"` + email + `"}`))
	}))

	http.HandleFunc("/api/devices/claim", auth(server.ClaimDevice))
	http.HandleFunc("/api/devices", auth(server.GetMyDevices))
	http.HandleFunc("/api/devices/delete", auth(server.DeleteDevice))
	http.HandleFunc("/api/upload", auth(server.UploadFile))
	http.HandleFunc("/api/files", auth(server.GetFiles))
	http.HandleFunc("/api/download", auth(server.DownloadFile))
	http.HandleFunc("/api/delete", auth(server.DeleteFile))

	// Agent Download
	http.HandleFunc("/agent.exe", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../agent/agent.exe")
	})
	http.HandleFunc("/agent-android", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../agent/agent-android")
	})

	// Public / Agent API
	http.HandleFunc("/register", server.RegisterDevice)
	http.HandleFunc("/heartbeat", server.Heartbeat)
	http.HandleFunc("/peers", server.GetPeers)
	http.HandleFunc("/relay/send", server.RelaySend)
	http.HandleFunc("/relay/recv", server.RelayRecv)
	http.HandleFunc("/chunk/location", server.RegisterChunkLocation)

	http.HandleFunc("/metadata", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			server.CreateFileMetadata(w, r)
		} else {
			server.GetFileMetadata(w, r)
		}
	})
	
	http.HandleFunc("/api/sync/deletions", server.GetDeletions)
	http.HandleFunc("/api/admin/rebalance", auth(server.RebalanceHandler))

	// Static
	fs := http.FileServer(http.Dir("../web"))
	http.Handle("/", fs)

	log.Println("GenDrive Server starting on :8085...")
	if err := http.ListenAndServe(":8085", nil); err != nil {
		log.Fatal(err)
	}
}
