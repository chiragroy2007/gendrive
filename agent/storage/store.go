package storage

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type Store struct {
	Root string
}

func NewStore(dataDir string) (*Store, error) {
	chunksDir := filepath.Join(dataDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return nil, err
	}
	return &Store{Root: chunksDir}, nil
}

func (s *Store) SaveChunk(chunkID string, data []byte) error {
	path := filepath.Join(s.Root, chunkID)
	return ioutil.WriteFile(path, data, 0644)
}

func (s *Store) GetChunk(chunkID string) ([]byte, error) {
	path := filepath.Join(s.Root, chunkID)
	return ioutil.ReadFile(path)
}

func (s *Store) HasChunk(chunkID string) bool {
	path := filepath.Join(s.Root, chunkID)
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func (s *Store) DeleteChunk(chunkID string) error {
	path := filepath.Join(s.Root, chunkID)
	return os.Remove(path)
}

func (s *Store) GetTotalUsage() (int64, error) {
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
