package storage

import (
	"io/ioutil"
	"os"
	"path/filepath"
)


// ChunkStore defines the interface for storage backends
type ChunkStore interface {
	SaveChunk(chunkID string, data []byte) error
	GetChunk(chunkID string) ([]byte, error)
	HasChunk(chunkID string) bool
	DeleteChunk(chunkID string) error
	GetTotalUsage() (int64, error)
}

type DiskStore struct {
	Root string
}

func NewDiskStore(dataDir string) (*DiskStore, error) {
	chunksDir := filepath.Join(dataDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return nil, err
	}
	return &DiskStore{Root: chunksDir}, nil
}

func (s *DiskStore) SaveChunk(chunkID string, data []byte) error {
	path := filepath.Join(s.Root, chunkID)
	return ioutil.WriteFile(path, data, 0644)
}

func (s *DiskStore) GetChunk(chunkID string) ([]byte, error) {
	path := filepath.Join(s.Root, chunkID)
	return ioutil.ReadFile(path)
}

func (s *DiskStore) HasChunk(chunkID string) bool {
	path := filepath.Join(s.Root, chunkID)
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func (s *DiskStore) DeleteChunk(chunkID string) error {
	path := filepath.Join(s.Root, chunkID)
	return os.Remove(path)
}

func (s *DiskStore) GetTotalUsage() (int64, error) {
	var size int64
	err := filepath.Walk(s.Root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

