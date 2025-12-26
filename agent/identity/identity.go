package identity

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"

	"github.com/google/uuid"
)

type Identity struct {
	DeviceID   string `json:"device_id"`
	PrivateKey []byte `json:"private_key"` // PEM encoded
	PublicKey  []byte `json:"public_key"`  // PEM encoded
}

func LoadOrGenerate(path string) (*Identity, error) {
	if _, err := os.Stat(path); err == nil {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		var id Identity
		if err := json.NewDecoder(file).Decode(&id); err != nil {
			return nil, err
		}
		return &id, nil
	}

	// Generate new
	id := &Identity{
		DeviceID: uuid.New().String(),
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	id.PrivateKey = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	})

	pubBytes := x509.MarshalPKCS1PublicKey(&privKey.PublicKey)
	id.PublicKey = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubBytes,
	})

	// Save
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(id); err != nil {
		return nil, err
	}

	return id, nil
}
