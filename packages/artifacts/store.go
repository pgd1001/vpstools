package artifacts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Metadata struct {
	ID          string
	ContentType string
	Size        int64
	SHA256      string
}

type Store interface {
	Put(id, contentType string, data []byte) (Metadata, error)
	Get(id string) ([]byte, Metadata, error)
	Delete(id string) error
}

type LocalStore struct {
	root string
	aead cipher.AEAD
}

func NewLocalStore(root, key string) (*LocalStore, error) {
	if root == "" || root == "." {
		return nil, errors.New("artifact directory must be a dedicated directory")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}
	if key == "" {
		keyPath := filepath.Join(root, ".key")
		contents, err := os.ReadFile(keyPath)
		if errors.Is(err, os.ErrNotExist) {
			contents = make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, contents); err != nil {
				return nil, fmt.Errorf("generate artifact key: %w", err)
			}
			if err := writeAtomic(keyPath, contents, 0600); err != nil {
				return nil, fmt.Errorf("persist artifact key: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("read artifact key: %w", err)
		}
		key = base64.RawStdEncoding.EncodeToString(contents)
	}
	decoded, err := base64.RawStdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("ARTIFACT_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &LocalStore{root: root, aead: aead}, nil
}

func (s *LocalStore) Put(id, contentType string, data []byte) (Metadata, error) {
	if id == "" {
		return Metadata{}, errors.New("artifact id is required")
	}
	hash := sha256.Sum256(data)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Metadata{}, err
	}
	ciphertext := append(nonce, s.aead.Seal(nil, nonce, data, []byte(id))...)
	path := s.path(id)
	if err := writeAtomic(path, ciphertext, 0600); err != nil {
		return Metadata{}, err
	}
	return Metadata{ID: id, ContentType: contentType, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", hash)}, nil
}

func (s *LocalStore) Get(id string) ([]byte, Metadata, error) {
	contents, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, Metadata{}, err
	}
	if len(contents) < s.aead.NonceSize() {
		return nil, Metadata{}, errors.New("invalid encrypted artifact")
	}
	nonce, ciphertext := contents[:s.aead.NonceSize()], contents[s.aead.NonceSize():]
	data, err := s.aead.Open(nil, nonce, ciphertext, []byte(id))
	if err != nil {
		return nil, Metadata{}, errors.New("artifact authentication failed")
	}
	hash := sha256.Sum256(data)
	return data, Metadata{ID: id, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", hash)}, nil
}

func (s *LocalStore) Delete(id string) error { return os.Remove(s.path(id)) }

func (s *LocalStore) path(id string) string {
	return filepath.Join(s.root, filepath.Base(id)+".bin")
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".artifact-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
