package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
    "bytes"
    "io"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type GDriveManager struct {
	DB     *sql.DB
	Config *oauth2.Config
    FolderID string // Cache
}

func NewGDriveManager(db *sql.DB) *GDriveManager {
	// Read credentials.json (server-side, not agent)
	// User must put credentials.json in server root.
	b, err := os.ReadFile("credentials.json")
	var config *oauth2.Config
	if err == nil {
		config, err = google.ConfigFromJSON(b, drive.DriveFileScope)
		if err != nil {
			log.Printf("GDrive: Failed to parse credentials.json: %v", err)
		}
	} else {
		log.Println("GDrive: No credentials.json found. GDrive integration disabled.")
	}

	return &GDriveManager{DB: db, Config: config}
}

// 1. Auth Flow

func (m *GDriveManager) HandleAuth(w http.ResponseWriter, r *http.Request) {
	if m.Config == nil {
		http.Error(w, "Server not configured for Google Drive (missing credentials.json)", http.StatusNotImplemented)
		return
	}

    // Set dynamic redirect based on Host correctly
    // (Simplification: assuming http and host are accessible and allowed in Console)
    scheme := "http"
    if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
        scheme = "https"
    }
    redirectURL := fmt.Sprintf("%s://%s/api/gdrive/callback", scheme, r.Host)
    m.Config.RedirectURL = redirectURL

	url := m.Config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (m *GDriveManager) HandleCallback(w http.ResponseWriter, r *http.Request) {
    userID := r.Header.Get("X-User-ID") // Requires cookie/auth middleware
    // Actually callback comes from Google, so it won't have the session cookie headers if same-site strict?
    // Wait, session cookie is usually sent. But X-User-ID is added by middleware.
    // So we must wrap this endpoint with Auth middleware.

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No code", http.StatusBadRequest)
		return
	}

    // Restore Redirect URL for exchange
    scheme := "http"
    if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
        scheme = "https"
    }
    m.Config.RedirectURL = fmt.Sprintf("%s://%s/api/gdrive/callback", scheme, r.Host)


	token, err := m.Config.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	// Store Token
	m.saveToken(userID, token)

	// Register Virtual Device
	deviceID := "GDrive-" + userID
	name := "Google Drive (Cloud)"
    
    // Check if exists
    var exists int
    m.DB.QueryRow("SELECT 1 FROM devices WHERE id = ?", deviceID).Scan(&exists)
    
    now := time.Now().Format(time.RFC3339)
    if exists == 0 {
	    // Create as 'gdrive' type, always online
	    _, err = m.DB.Exec("INSERT INTO devices (id, user_id, public_key, name, last_seen, online, type, ip) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		    deviceID, userID, "N/A", name, now, 1, "gdrive", "localhost")
    } else {
        // Update to ensure online
        m.DB.Exec("UPDATE devices SET online=1, last_seen=? WHERE id=?", now, deviceID)
    }

    // Proactively create folder "GenDrive Data"
    go func() {
        client := m.Config.Client(context.Background(), token)
        srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
        if err == nil {
            m.getOrCreateFolder(srv, "GenDrive Data")
        }
    }()

	http.Redirect(w, r, "/#devices", http.StatusTemporaryRedirect)
}

// 2. Token Mgmt

func (m *GDriveManager) saveToken(userID string, t *oauth2.Token) {
	// Upsert
	_, err := m.DB.Exec(`INSERT INTO gdrive_tokens (user_id, access_token, refresh_token, token_type, expiry) 
		VALUES (?, ?, ?, ?, ?) 
		ON CONFLICT(user_id) DO UPDATE SET 
		access_token=excluded.access_token, 
		refresh_token=excluded.refresh_token, 
		expiry=excluded.expiry`,
		userID, t.AccessToken, t.RefreshToken, t.TokenType, t.Expiry)
	if err != nil {
		log.Printf("Failed to save gdrive token: %v", err)
	}
}

func (m *GDriveManager) getToken(userID string) (*oauth2.Token, error) {
	var t oauth2.Token

	err := m.DB.QueryRow("SELECT access_token, refresh_token, token_type, expiry FROM gdrive_tokens WHERE user_id = ?", userID).
		Scan(&t.AccessToken, &t.RefreshToken, &t.TokenType, &t.Expiry)
	
	if err != nil {
		return nil, err
	}
    // If RefreshToken empty (sometimes update doesn't return it?), we might need to preserve old one?
    // But we upserted it.
    
	return &t, nil
}

func (m *GDriveManager) getClient(userID string) (*http.Client, error) {
	token, err := m.getToken(userID)
	if err != nil {
		return nil, err
	}
	return m.Config.Client(context.Background(), token), nil
}


// 3. Operations (Upload/Download)

func (m *GDriveManager) UploadChunk(userID, chunkID string, data []byte) error {
	client, err := m.getClient(userID)
	if err != nil {
		return err
	}
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	// Folder
	folderID, err := m.getOrCreateFolder(srv, "GenDrive Data")
	if err != nil {
		return err
	}
    
    // Check if exists?
    // Just upload/overwrite
    // Delete existing if any to be safe (no duplicates)
    m.deleteFile(srv, chunkID, folderID)

	f := &drive.File{
		Name:    chunkID,
		Parents: []string{folderID},
	}
    
    // Pipe data
    // We need an io.Reader. 
    // bytes.NewReader(data)
    _, err = srv.Files.Create(f).Media(getReader(data)).Do()
    return err
}

func (m *GDriveManager) DownloadChunk(userID, chunkID string) ([]byte, error) {
    client, err := m.getClient(userID)
	if err != nil {
		return nil, err
	}
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
    
    folderID, err := m.getOrCreateFolder(srv, "GenDrive Data")

    fileID, err := m.findFileID(srv, chunkID, folderID)
    if err != nil || fileID == "" {
        return nil, os.ErrNotExist
    }

    resp, err := srv.Files.Get(fileID).Download()
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    // Use io.ReadAll
    return io.ReadAll(resp.Body)
}

// Helpers

func (m *GDriveManager) DeleteChunk(userID, chunkID string) error {
    client, err := m.getClient(userID)
	if err != nil {
		return err
	}
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return err
	}
    
    // We assume standard folder
    folderID, err := m.getOrCreateFolder(srv, "GenDrive Data")
    if err != nil { return err }

    m.deleteFile(srv, chunkID, folderID)
    return nil
}

func getReader(data []byte) *bytes.Reader {
    return bytes.NewReader(data)
}
// (Standard io.ReadAll is better, importing io in top)

func (m *GDriveManager) getOrCreateFolder(srv *drive.Service, name string) (string, error) {
    if m.FolderID != "" {
        return m.FolderID, nil
    }

	q := fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false", name)
	r, err := srv.Files.List().Q(q).Fields("files(id)").Do()
	if err != nil {
		return "", err
	}
	if len(r.Files) > 0 {
        m.FolderID = r.Files[0].Id
		return m.FolderID, nil
	}
	f := &drive.File{Name: name, MimeType: "application/vnd.google-apps.folder"}
	file, err := srv.Files.Create(f).Fields("id").Do()
	if err != nil { return "", err }
    m.FolderID = file.Id
	return file.Id, nil
}

func (m *GDriveManager) findFileID(srv *drive.Service, name, folderID string) (string, error) {
    q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", name, folderID)
	r, err := srv.Files.List().Q(q).Fields("files(id)").Do()
    if err != nil { return "", err}
    if len(r.Files) == 0 { return "", nil }
    return r.Files[0].Id, nil
}

func (m *GDriveManager) deleteFile(srv *drive.Service, name, folderID string) {
    id, _ := m.findFileID(srv, name, folderID)
    if id != "" {
        srv.Files.Delete(id).Do()
    }
}
