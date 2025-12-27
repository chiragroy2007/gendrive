package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type GDriveStore struct {
	Service   *drive.Service
	FolderID  string
}

func NewGDriveStore(credentialsPath string) (*GDriveStore, error) {
	ctx := context.Background()
	
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %v", err)
	}

	// Ideally we want full token flow, but for now headless server-to-server or
	// assuming user provides a token.json is tricky.
	// We'll use the "Credentials from a file" helper which usually handles service accounts,
	// BUT the user request says "download credentials.json (OAuth2)".
	// This usually requires a browser flow or copy-paste flow.
	// For "headless" operation, we'll assume the user has a "token.json" or we
	// prompt them to auth on first run?
	// To keep it simple as per plan: "It should authenticate via a user-provided credentials.json".
	// We'll use a simple config setup.

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %v", err)
	}
	
	// Check for token.json
	tokenFile := "token.json"
	client := getClient(config, tokenFile)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Drive client: %v", err)
	}

	// Create or Find 'GenDrive_Store' folder
	folderID, err := getOrCreateFolder(srv, "GenDrive_Store")
	if err != nil {
		return nil, err
	}

	return &GDriveStore{Service: srv, FolderID: folderID}, nil
}

func (s *GDriveStore) SaveChunk(chunkID string, data []byte) error {
	// Check if exists first? Or just overwrite.
	// Drive allows duplicates. We should check.
	// Use 'name = target and parents in folder'
	
	// To simplify, delete if exists
	s.DeleteChunk(chunkID)

	f := &drive.File{
		Name:    chunkID,
		Parents: []string{s.FolderID},
	}
	
	// Create temporary file for upload content or use pipe?
	// Pipe is better but harder with retries.
	// We'll write to temp file then upload
	tmp, err := os.CreateTemp("", "chunk")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	// Reset pointer
	tmp.Seek(0, 0)
	
	_, err = s.Service.Files.Create(f).Media(tmp).Do()
	return err
}

func (s *GDriveStore) GetChunk(chunkID string) ([]byte, error) {
	fileID, err := s.findFileID(chunkID)
	if err != nil {
		return nil, err
	}
	if fileID == "" {
		return nil, os.ErrNotExist
	}

	resp, err := s.Service.Files.Get(fileID).Download()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (s *GDriveStore) HasChunk(chunkID string) bool {
	id, _ := s.findFileID(chunkID)
	return id != ""
}

func (s *GDriveStore) DeleteChunk(chunkID string) error {
	fileID, err := s.findFileID(chunkID)
	if err != nil {
		return err
	}
	if fileID == "" {
		return nil // Already gone
	}
	
	return s.Service.Files.Delete(fileID).Do()
}

func (s *GDriveStore) GetTotalUsage() (int64, error) {
	about, err := s.Service.About.Get().Fields("storageQuota").Do()
	if err != nil {
		return 0, err
	}
	return about.StorageQuota.Usage, nil
}

// Helpers

func (s *GDriveStore) findFileID(name string) (string, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", name, s.FolderID)
	r, err := s.Service.Files.List().Q(q).Fields("files(id)").Do()
	if err != nil {
		return "", err
	}
	if len(r.Files) == 0 {
		return "", nil
	}
	return r.Files[0].Id, nil
}

func getOrCreateFolder(srv *drive.Service, name string) (string, error) {
	q := fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false", name)
	r, err := srv.Files.List().Q(q).Fields("files(id)").Do()
	if err != nil {
		return "", err
	}
	
	if len(r.Files) > 0 {
		return r.Files[0].Id, nil
	}
	
	// Create
	f := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
	}
	file, err := srv.Files.Create(f).Fields("id").Do()
	if err != nil {
		return "", err
	}
	return file.Id, nil
}

// Auth Helper (Simplified from official Go Quickstart)

func getClient(config *oauth2.Config, tokenFile string) *http.Client {
	tok, err := tokenFromFile(tokenFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokenFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the authorization code: \n%v\n", authURL)
	
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}
	
	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
