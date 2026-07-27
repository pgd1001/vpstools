package artifacts

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	Check() error
}

// S3Config configures an S3-compatible object store. Endpoint may point at
// AWS S3, MinIO, Cloudflare R2, or another path-style S3-compatible service.
// EncryptionKey is an optional base64-encoded 32-byte client-side key. The
// server-side encryption fields add the corresponding S3 request headers.
type S3Config struct {
	Endpoint string
	Bucket   string
	Region   string
	Prefix   string

	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	EncryptionKey        string
	ServerSideEncryption string
	SSEKMSKeyID          string
	Timeout              time.Duration
	MaxRetries           int
	RetryBackoff         time.Duration
	HTTPClient           *http.Client
}

type S3Store struct {
	endpoint, bucket, region, prefix   string
	accessKey, secretKey, sessionToken string
	client                             *http.Client
	maxRetries                         int
	backoff                            time.Duration
	aead                               cipher.AEAD
	sse, kmsKeyID                      string
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("S3 endpoint must be an absolute http or https URL")
	}
	if strings.TrimSpace(cfg.Bucket) == "" || strings.ContainsAny(cfg.Bucket, "/\\") {
		return nil, errors.New("S3 bucket is required and must not contain a slash")
	}
	if (cfg.AccessKeyID == "") != (cfg.SecretAccessKey == "") {
		return nil, errors.New("S3 access key and secret key must be provided together")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout < 0 || cfg.Timeout > 5*time.Minute {
		return nil, errors.New("S3 timeout must be between 1 nanosecond and 5 minutes")
	}
	if cfg.MaxRetries < 0 || cfg.MaxRetries > 5 {
		return nil, errors.New("S3 max retries must be between 0 and 5")
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = 100 * time.Millisecond
	}
	if cfg.RetryBackoff < 0 || cfg.RetryBackoff > 10*time.Second {
		return nil, errors.New("S3 retry backoff must be between 0 and 10 seconds")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	if client.Timeout == 0 {
		client.Timeout = cfg.Timeout
	}

	store := &S3Store{endpoint: endpoint, bucket: cfg.Bucket, region: cfg.Region,
		prefix: strings.Trim(cfg.Prefix, "/"), accessKey: cfg.AccessKeyID,
		secretKey: cfg.SecretAccessKey, sessionToken: cfg.SessionToken, client: client,
		maxRetries: cfg.MaxRetries, backoff: cfg.RetryBackoff, sse: cfg.ServerSideEncryption,
		kmsKeyID: cfg.SSEKMSKeyID}
	if cfg.EncryptionKey != "" {
		key, err := base64.RawStdEncoding.DecodeString(cfg.EncryptionKey)
		if err != nil {
			key, err = base64.StdEncoding.DecodeString(cfg.EncryptionKey)
		}
		if err != nil || len(key) != 32 {
			return nil, errors.New("S3 encryption key must be a base64-encoded 32-byte key")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		store.aead, err = cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
	}
	if store.sse != "" && store.sse != "AES256" && store.sse != "aws:kms" {
		return nil, errors.New("S3 server-side encryption must be AES256 or aws:kms")
	}
	if store.sse == "aws:kms" && store.kmsKeyID == "" {
		return nil, errors.New("S3 KMS key ID is required for aws:kms encryption")
	}
	return store, nil
}

func (s *S3Store) Put(id, contentType string, data []byte) (Metadata, error) {
	if err := validateArtifactID(id); err != nil {
		return Metadata{}, err
	}
	hash := sha256.Sum256(data)
	checksum := fmt.Sprintf("%x", hash)
	payload := data
	if s.aead != nil {
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return Metadata{}, err
		}
		payload = append(nonce, s.aead.Seal(nil, nonce, data, []byte(id))...)
	}
	path := s.objectPath(id)
	req, err := http.NewRequest(http.MethodPut, path, bytes.NewReader(payload))
	if err != nil {
		return Metadata{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Amz-Meta-Sha256", checksum)
	req.Header.Set("X-Amz-Meta-Content-Length", fmt.Sprintf("%d", len(data)))
	s.setEncryptionHeaders(req)
	if err := s.do(req, payload); err != nil {
		return Metadata{}, fmt.Errorf("put artifact %q: %w", id, err)
	}
	return Metadata{ID: id, ContentType: contentType, Size: int64(len(data)), SHA256: checksum}, nil
}

func (s *S3Store) Get(id string) ([]byte, Metadata, error) {
	if err := validateArtifactID(id); err != nil {
		return nil, Metadata{}, err
	}
	req, err := http.NewRequest(http.MethodGet, s.objectPath(id), nil)
	if err != nil {
		return nil, Metadata{}, err
	}
	resp, err := s.doResponse(req, nil)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("get artifact %q: %w", id, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Metadata{}, err
	}
	data := payload
	if s.aead != nil {
		if len(payload) < s.aead.NonceSize() {
			return nil, Metadata{}, errors.New("invalid encrypted artifact")
		}
		data, err = s.aead.Open(nil, payload[:s.aead.NonceSize()], payload[s.aead.NonceSize():], []byte(id))
		if err != nil {
			return nil, Metadata{}, errors.New("artifact authentication failed")
		}
	}
	hash := sha256.Sum256(data)
	checksum := fmt.Sprintf("%x", hash)
	if expected := resp.Header.Get("X-Amz-Meta-Sha256"); expected != "" && !strings.EqualFold(expected, checksum) {
		return nil, Metadata{}, fmt.Errorf("artifact checksum mismatch: got %s, want %s", checksum, expected)
	}
	contentType := resp.Header.Get("Content-Type")
	return data, Metadata{ID: id, ContentType: contentType, Size: int64(len(data)), SHA256: checksum}, nil
}

func (s *S3Store) Delete(id string) error {
	if err := validateArtifactID(id); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, s.objectPath(id), nil)
	if err != nil {
		return err
	}
	if err := s.do(req, nil); err != nil {
		return fmt.Errorf("delete artifact %q: %w", id, err)
	}
	return nil
}

// Check validates configuration and verifies that the configured bucket is reachable.
func (s *S3Store) Check() error {
	req, err := http.NewRequest(http.MethodHead, s.bucketPath(), nil)
	if err != nil {
		return err
	}
	if err := s.do(req, nil); err != nil {
		return fmt.Errorf("check S3 bucket: %w", err)
	}
	return nil
}

func (s *S3Store) objectPath(id string) string {
	key := s.prefix
	if key != "" {
		key += "/"
	}
	key += id
	return s.endpoint + "/" + s.bucket + "/" + strings.TrimLeft(key, "/")
}
func (s *S3Store) bucketPath() string { return s.endpoint + "/" + s.bucket }
func (s *S3Store) setEncryptionHeaders(req *http.Request) {
	if s.sse != "" {
		req.Header.Set("X-Amz-Server-Side-Encryption", s.sse)
	}
	if s.kmsKeyID != "" {
		req.Header.Set("X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id", s.kmsKeyID)
	}
}

func (s *S3Store) do(req *http.Request, body []byte) error {
	_, err := s.doResponse(req, body)
	return err
}
func (s *S3Store) doResponse(req *http.Request, body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}
		if err := s.sign(req, body); err != nil {
			return nil, err
		}
		resp, err := s.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		retry := err != nil || resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests
		if !retry || attempt >= s.maxRetries {
			if resp != nil {
				defer resp.Body.Close()
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				return nil, fmt.Errorf("S3 request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
			}
			return nil, err
		}
		if resp != nil {
			resp.Body.Close()
		}
		delay := s.backoff * time.Duration(1<<attempt)
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}
		time.Sleep(delay)
	}
}

func validateArtifactID(id string) error {
	if id == "" {
		return errors.New("artifact id is required")
	}
	if strings.HasPrefix(id, "/") || strings.ContainsAny(id, "\\\x00\r\n?#%") {
		return errors.New("invalid artifact id")
	}
	for _, part := range strings.Split(id, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("invalid artifact id")
		}
	}
	return nil
}

func (s *S3Store) sign(req *http.Request, body []byte) error {
	if s.accessKey == "" {
		return nil
	}
	now := time.Now().UTC()
	payloadHash := sha256.Sum256(body)
	payloadHex := hex.EncodeToString(payloadHash[:])
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Content-Sha256", payloadHex)
	req.Header.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	if s.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", s.sessionToken)
	}

	keys := make([]string, 0, len(req.Header)+1)
	values := map[string]string{"host": req.URL.Host}
	for key, list := range req.Header {
		lower := strings.ToLower(key)
		if lower == "authorization" {
			continue
		}
		keys = append(keys, lower)
		values[lower] = strings.Join(list, ",")
	}
	keys = append(keys, "host")
	sort.Strings(keys)
	unique := keys[:0]
	for _, key := range keys {
		if len(unique) == 0 || unique[len(unique)-1] != key {
			unique = append(unique, key)
		}
	}
	keys = unique
	var canonicalHeaders strings.Builder
	for _, key := range keys {
		canonicalHeaders.WriteString(key)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.TrimSpace(values[key]))
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(keys, ";")
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := req.URL.Query().Encode()
	canonicalRequest := strings.Join([]string{req.Method, canonicalURI, canonicalQuery, canonicalHeaders.String(), signedHeaders, payloadHex}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	date := now.Format("20060102")
	scope := date + "/" + s.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + now.Format("20060102T150405Z") + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	kDate := hmacSum([]byte("AWS4"+s.secretKey), []byte(date))
	kRegion := hmacSum(kDate, []byte(s.region))
	kService := hmacSum(kRegion, []byte("s3"))
	kSigning := hmacSum(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSum(kSigning, []byte(stringToSign)))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return nil
}

func hmacSum(key, value []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(value)
	return mac.Sum(nil)
}

var _ Store = (*S3Store)(nil)

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

// Check verifies that the backing directory is accessible and that one
// existing encrypted artefact, when present, can be authenticated and read.
func (s *LocalStore) Check() error {
	info, err := os.Stat(s.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("artifact path is not a directory")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bin") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".bin")
		if _, _, err := s.Get(id); err != nil {
			return fmt.Errorf("read encrypted artifact %s: %w", id, err)
		}
		break
	}
	return nil
}

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
