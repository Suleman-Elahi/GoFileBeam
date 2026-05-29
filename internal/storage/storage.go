package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gofilebeam/internal/config"
)

var (
	ErrFileTooLarge      = errors.New("file exceeds maximum size")
	ErrStorageFull       = errors.New("storage quota exceeded")
	ErrFileNotFound      = errors.New("file not found")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrFileExpired       = errors.New("file has expired")
	ErrInvalidExpiration = errors.New("invalid expiration option")
)

// FileMetadata stores metadata about uploaded files
type FileMetadata struct {
	ID           string    `json:"id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	UploadedAt   time.Time `json:"uploaded_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	MaxDownloads int       `json:"max_downloads"`
	Downloads    int       `json:"downloads"`
	IsEncrypted  bool      `json:"is_encrypted"`
	PasswordHash string    `json:"password_hash,omitempty"`
	Salt         string    `json:"salt,omitempty"`
}

// ExpirationOption represents the expiration settings from the UI
type ExpirationOption struct {
	MaxDownloads int
	MaxDays      int
}

// Storage manages file storage with encryption and quota
type Storage struct {
	config     *config.Config
	basePath   string
	metadata   map[string]*FileMetadata
	mu         sync.RWMutex
	totalSize  int64
	expirationOptions map[string]ExpirationOption
}

// NewStorage creates a new storage instance
func NewStorage(cfg *config.Config) (*Storage, error) {
	storage := &Storage{
		config:     cfg,
		basePath:   cfg.StoragePath,
		metadata:   make(map[string]*FileMetadata),
		totalSize:  0,
		expirationOptions: map[string]ExpirationOption{
			"1 Download or 1 Day":    {MaxDownloads: 1, MaxDays: 1},
			"10 Downloads or 7 Days": {MaxDownloads: 10, MaxDays: 7},
			"100 Downloads or 30 Days": {MaxDownloads: 100, MaxDays: 30},
		},
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storage.basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Load existing metadata
	if err := storage.loadMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	// Start cleanup goroutine
	go storage.startCleanup()

	return storage, nil
}

// loadMetadata loads existing file metadata from disk
func (s *Storage) loadMetadata() error {
	metadataPath := filepath.Join(s.basePath, "metadata.json")
	
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	var files []*FileMetadata
	if err := json.Unmarshal(data, &files); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, file := range files {
		s.metadata[file.ID] = file
		s.totalSize += file.Size
	}

	return nil
}

// saveMetadata saves metadata to disk
func (s *Storage) saveMetadata() error {
	s.mu.RLock()
	files := make([]*FileMetadata, 0, len(s.metadata))
	for _, file := range s.metadata {
		files = append(files, file)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(s.basePath, "metadata.json")
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// generateID generates a unique file ID
func (s *Storage) generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// hashPassword creates a SHA256 hash of the password with salt
func hashPassword(password, salt string) string {
	hash := sha256.New()
	hash.Write([]byte(password + salt))
	return hex.EncodeToString(hash.Sum(nil))
}

// encryptFile encrypts a file with AES-GCM
func encryptFile(inputPath, outputPath, password string) error {
	// Read the input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Generate salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key from password
	key := sha256.Sum256([]byte(password + string(salt)))
	
	// Create AES cipher
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Write salt + ciphertext to output file
	outputData := append(salt, ciphertext...)
	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf("failed to write encrypted file: %w", err)
	}

	return nil
}

// decryptFile decrypts a file with AES-GCM
func decryptFile(inputPath, outputPath, password string) error {
	// Read the encrypted file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %w", err)
	}

	if len(data) < 16 {
		return errors.New("invalid encrypted file format")
	}

	// Extract salt and ciphertext
	salt := data[:16]
	ciphertext := data[16:]

	// Derive key from password
	key := sha256.Sum256([]byte(password + string(salt)))
	
	// Create AES cipher
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ErrInvalidPassword
	}

	// Write decrypted data
	if err := os.WriteFile(outputPath, plaintext, 0644); err != nil {
		return fmt.Errorf("failed to write decrypted file: %w", err)
	}

	return nil
}

// StoreFile stores a file with optional encryption
func (s *Storage) StoreFile(filename string, data []byte, expirationOption string, password string) (*FileMetadata, error) {
	// Check file size
	if int64(len(data)) > s.config.MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Check storage quota
	s.mu.Lock()
	if s.totalSize+int64(len(data)) > s.config.GetMaxStorageBytes() {
		s.mu.Unlock()
		return nil, ErrStorageFull
	}
	s.mu.Unlock()

	// Get expiration settings
	expOption, exists := s.expirationOptions[expirationOption]
	if !exists {
		return nil, ErrInvalidExpiration
	}

	// Generate file ID
	id := s.generateID()
	filePath := filepath.Join(s.basePath, id)

	// Create metadata
	metadata := &FileMetadata{
		ID:           id,
		Filename:     id,
		OriginalName: filename,
		Size:         int64(len(data)),
		UploadedAt:   time.Now(),
		ExpiresAt:    time.Now().Add(time.Duration(expOption.MaxDays) * 24 * time.Hour),
		MaxDownloads: expOption.MaxDownloads,
		Downloads:    0,
		IsEncrypted:  password != "",
	}

	// Handle encryption if password is provided
	if password != "" {
		// Generate salt for password hashing
		salt := make([]byte, 16)
		rand.Read(salt)
		
		// Hash password
		metadata.PasswordHash = hashPassword(password, string(salt))
		metadata.Salt = string(salt)

		// Write data to temp file for encryption
		tempPath := filePath + ".tmp"
		if err := os.WriteFile(tempPath, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}

		// Encrypt file
		if err := encryptFile(tempPath, filePath, password); err != nil {
			os.Remove(tempPath)
			return nil, fmt.Errorf("failed to encrypt file: %w", err)
		}

		// Remove temp file
		os.Remove(tempPath)
	} else {
		// Write file without encryption
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
	}

	// Update storage
	s.mu.Lock()
	s.metadata[id] = metadata
	s.totalSize += metadata.Size
	s.mu.Unlock()

	// Save metadata
	if err := s.saveMetadata(); err != nil {
		// Clean up file if metadata save fails
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return metadata, nil
}

// GetFile retrieves a file for download
func (s *Storage) GetFile(id, password string) ([]byte, *FileMetadata, error) {
	s.mu.RLock()
	metadata, exists := s.metadata[id]
	s.mu.RUnlock()

	if !exists {
		return nil, nil, ErrFileNotFound
	}

	// Check if file has expired
	if time.Now().After(metadata.ExpiresAt) {
		return nil, nil, ErrFileExpired
	}

	// Check download limit
	if metadata.Downloads >= metadata.MaxDownloads {
		return nil, nil, ErrFileExpired
	}

	// Check password if file is encrypted
	if metadata.IsEncrypted {
		if password == "" {
			return nil, nil, ErrInvalidPassword
		}
		
		// Verify password
		expectedHash := hashPassword(password, metadata.Salt)
		if expectedHash != metadata.PasswordHash {
			return nil, nil, ErrInvalidPassword
		}
	}

	filePath := filepath.Join(s.basePath, id)
	
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	// If encrypted, decrypt in memory
	if metadata.IsEncrypted {
		// Create temp file for decryption
		tempDir := os.TempDir()
		tempInput := filepath.Join(tempDir, id+".enc")
		tempOutput := filepath.Join(tempDir, id+".dec")
		
		defer os.Remove(tempInput)
		defer os.Remove(tempOutput)

		// Write encrypted data to temp file
		if err := os.WriteFile(tempInput, data, 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write temp file: %w", err)
		}

		// Decrypt file
		if err := decryptFile(tempInput, tempOutput, password); err != nil {
			return nil, nil, err
		}

		// Read decrypted data
		data, err = os.ReadFile(tempOutput)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read decrypted file: %w", err)
		}
	}

	// Update download count
	s.mu.Lock()
	metadata.Downloads++
	s.mu.Unlock()

	// Save updated metadata
	if err := s.saveMetadata(); err != nil {
		// Log error but don't fail the download
		fmt.Printf("Warning: failed to save metadata: %v\n", err)
	}

	return data, metadata, nil
}

// DeleteFile removes a file from storage
func (s *Storage) DeleteFile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, exists := s.metadata[id]
	if !exists {
		return ErrFileNotFound
	}

	// Remove file
	filePath := filepath.Join(s.basePath, id)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file: %w", err)
	}

	// Update storage size
	s.totalSize -= metadata.Size
	
	// Remove from metadata
	delete(s.metadata, id)

	// Save metadata
	return s.saveMetadata()
}

// GetStorageInfo returns storage usage information
// This recalculates from the filesystem to ensure accuracy
func (s *Storage) GetStorageInfo() (usedBytes int64, totalBytes int64, fileCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Recalculate actual storage usage from filesystem
	actualSize := int64(0)
	actualCount := 0
	
	// Check which files actually exist
	toDelete := []string{}
	for id := range s.metadata {
		filePath := filepath.Join(s.basePath, id)
		if info, err := os.Stat(filePath); err == nil {
			// File exists
			actualSize += info.Size()
			actualCount++
		} else if os.IsNotExist(err) {
			// File was deleted externally, mark for cleanup
			toDelete = append(toDelete, id)
		}
	}
	
	// Clean up metadata for files that no longer exist
	if len(toDelete) > 0 {
		for _, id := range toDelete {
			delete(s.metadata, id)
		}
		// Update cached total size
		s.totalSize = actualSize
		// Save updated metadata (async to avoid blocking)
		go s.saveMetadata()
	} else {
		// Update cached total size to match actual
		s.totalSize = actualSize
	}
	
	return actualSize, s.config.GetMaxStorageBytes(), actualCount
}

// startCleanup starts a goroutine to clean up expired files
func (s *Storage) startCleanup() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanupExpiredFiles()
	}
}

// cleanupExpiredFiles removes files that have expired
func (s *Storage) cleanupExpiredFiles() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	toDelete := []string{}

	for id, metadata := range s.metadata {
		if now.After(metadata.ExpiresAt) || metadata.Downloads >= metadata.MaxDownloads {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		metadata := s.metadata[id]
		
		// Remove file
		filePath := filepath.Join(s.basePath, id)
		os.Remove(filePath)
		
		// Update storage size
		s.totalSize -= metadata.Size
		
		// Remove from metadata
		delete(s.metadata, id)
	}

	if len(toDelete) > 0 {
		s.saveMetadata()
	}
}