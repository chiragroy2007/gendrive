package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"p2p-drive/agent/bg"
	"p2p-drive/agent/client"
	"p2p-drive/agent/identity"
	"p2p-drive/agent/storage"
)

func main() {
	serverURL := flag.String("server", "http://localhost:8080", "Control Server URL")
	dataDir := flag.String("data", "./agent_data", "Data directory")
	name := flag.String("name", "Device-1", "Device Name")

	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatal(err)
	}

	// 1. Identity
	idPath := filepath.Join(*dataDir, "identity.json")
	id, err := identity.LoadOrGenerate(idPath)
	if err != nil {
		log.Fatalf("Failed to load/generate identity: %v", err)
	}
	log.Printf("Device ID: %s", id.DeviceID)

	// Generate Claim Token
	claimToken := generateToken()

	// 2. Client & Registration
	c := client.NewClient(*serverURL, id.DeviceID)
	registeredID, err := c.Register(string(id.PublicKey), *name, id.DeviceID, claimToken)
	if err != nil {
		log.Printf("Registration warning: %v", err)
	} else {
		if registeredID != id.DeviceID {
			log.Printf("WARNING: Server assigned different ID: %s", registeredID)
			id.DeviceID = registeredID
		}
	}

	// 3. Storage & Receiver
	store, err := storage.NewStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}

	receiver := bg.NewReceiver(c, store, id.DeviceID)
	receiver.Start()

	// 4. Heartbeat
	c.StartHeartbeat(5 * time.Second)

	// 5. Display Claim Info
	fmt.Println("========================================")
	fmt.Println("   GENDRIVE - DEVICE STARTED")
	fmt.Println("========================================")
	fmt.Printf(" Device ID:   %s\n", id.DeviceID)
	fmt.Printf(" Claim Token: %s\n", claimToken)
	fmt.Println("========================================")
	fmt.Println(" Go to your Dashboard to Add this Device.")
	fmt.Println("========================================")

	log.Println("Agent running (Headless). Press Ctrl+C to stop.")

	// Stats ticker
	go func() {
		for {
			time.Sleep(10 * time.Second) // Updates every 10s for demo
			usage, _ := store.GetTotalUsage()
			fmt.Printf("[%s] Storage Used: %.2f MB\n", time.Now().Format("15:04:05"), float64(usage)/1024/1024)
		}
	}()

	select {}
}

func generateToken() string {
	b := make([]byte, 6) // 12 chars hex
	rand.Read(b)
	return hex.EncodeToString(b)
}
