package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// streamChunkSize is the plaintext chunk size used for chunked AES-GCM
// streaming encryption/decryption. Each chunk is sealed/opened independently
// so files of arbitrary size can be processed without buffering the whole
// file in memory.
const streamChunkSize = 64 * 1024 // 64 KiB

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
	config   *config.Config
	basePath string

	mu       sync.RWMutex
	metadata map[string]*FileMetadata

	// totalSize is tracked incrementally (not recomputed from disk on every
	// request) and is also accessed atomically for fast reads from
	// GetStorageInfo without needing the main mutex.
	totalSize int64

	expirationOptions map[string]ExpirationOption

	// metadataDirty tracks whether metadata.json needs to be persisted.
	// Writes are coalesced by a background flusher instead of doing a full
	// synchronous rewrite of the metadata file on every upload/download.
	metadataDirty int32
	flushRequest  chan struct{}
	stopFlusher   chan struct{}
}

// NewStorage creates a new storage instance
func NewStorage(cfg *config.Config) (*Storage, error) {
	storage := &Storage{
		config:   cfg,
		basePath: cfg.StoragePath,
		metadata: make(map[string]*FileMetadata),
		expirationOptions: map[string]ExpirationOption{
			"1 Download or 1 Day":      {MaxDownloads: 1, MaxDays: 1},
			"10 Downloads or 7 Days":   {MaxDownloads: 10, MaxDays: 7},
			"100 Downloads or 30 Days": {MaxDownloads: 100, MaxDays: 30},
		},
		flushRequest: make(chan struct{}, 1),
		stopFlusher:  make(chan struct{}),
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storage.basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Load existing metadata
	if err := storage.loadMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	// Reconcile the cached total size with what's actually on disk once at
	// startup (cheap, happens once) so incremental tracking starts accurate.
	storage.reconcileFromDisk()

	// Start cleanup goroutine
	go storage.startCleanup()

	// Start metadata flusher goroutine (debounced, atomic writes)
	go storage.startMetadataFlusher()

	return storage, nil
}

// Close stops background goroutines and flushes any pending metadata.
func (s *Storage) Close() error {
	close(s.stopFlusher)
	if atomic.LoadInt32(&s.metadataDirty) != 0 {
		return s.saveMetadataNow()
	}
	return nil
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

	var total int64
	for _, file := range files {
		s.metadata[file.ID] = file
		total += file.Size
	}
	atomic.StoreInt64(&s.totalSize, total)

	return nil
}

// reconcileFromDisk recalculates actual storage usage from the filesystem
// once (used at startup) so that incremental in-memory tracking afterwards
// doesn't drift due to files that were added/removed outside the running
// process.
func (s *Storage) reconcileFromDisk() {
	s.mu.Lock()
	defer s.mu.Unlock()

	var actualSize int64
	toDelete := []string{}

	for id, meta := range s.metadata {
		filePath := filepath.Join(s.basePath, id)
		if info, err := os.Stat(filePath); err == nil {
			actualSize += info.Size()
			_ = meta
		} else if os.IsNotExist(err) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(s.metadata, id)
	}

	atomic.StoreInt64(&s.totalSize, actualSize)

	if len(toDelete) > 0 {
		s.markDirty()
	}
}

// markDirty flags metadata as needing a flush and nudges the background
// flusher. Non-blocking: if a flush is already pending, this is a no-op.
func (s *Storage) markDirty() {
	atomic.StoreInt32(&s.metadataDirty, 1)
	select {
	case s.flushRequest <- struct{}{}:
	default:
	}
}

// startMetadataFlusher runs in the background and periodically persists
// metadata to disk if it has changed, instead of doing a full synchronous
// rewrite on every single upload/download request. This bounds write
// amplification under high request concurrency to roughly one write per
// debounce interval rather than one write per request.
func (s *Storage) startMetadataFlusher() {
	const debounce = 500 * time.Millisecond
	const maxInterval = 5 * time.Second

	timer := time.NewTimer(maxInterval)
	defer timer.Stop()

	for {
		select {
		case <-s.stopFlusher:
			return
		case <-s.flushRequest:
			// Debounce: wait a short period to coalesce bursts of changes
			// (e.g. many uploads/downloads in quick succession) into a
			// single write, but never wait longer than maxInterval.
			debounceTimer := time.NewTimer(debounce)
			select {
			case <-debounceTimer.C:
			case <-s.stopFlusher:
				debounceTimer.Stop()
				return
			}
			if atomic.CompareAndSwapInt32(&s.metadataDirty, 1, 0) {
				if err := s.saveMetadataNow(); err != nil {
					fmt.Printf("Warning: failed to save metadata: %v\n", err)
					atomic.StoreInt32(&s.metadataDirty, 1)
				}
			}
			timer.Reset(maxInterval)
		case <-timer.C:
			if atomic.CompareAndSwapInt32(&s.metadataDirty, 1, 0) {
				if err := s.saveMetadataNow(); err != nil {
					fmt.Printf("Warning: failed to save metadata: %v\n", err)
					atomic.StoreInt32(&s.metadataDirty, 1)
				}
			}
			timer.Reset(maxInterval)
		}
	}
}

// saveMetadataNow saves metadata to disk atomically (write to temp file,
// then rename over the real path) so a crash mid-write can never leave
// metadata.json truncated or corrupted.
func (s *Storage) saveMetadataNow() error {
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
	tempPath := metadataPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata temp file: %w", err)
	}
	if err := os.Rename(tempPath, metadataPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename metadata temp file: %w", err)
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

// newGCMForPassword derives a key from password+salt and returns a ready
// AES-GCM AEAD.
func newGCMForPassword(password string, salt []byte) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(password + string(salt)))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return gcm, nil
}

// chunkNonce derives a unique 12-byte nonce for chunk index i from a random
// 4-byte nonce prefix generated per file. The low 8 bytes are the chunk
// counter, guaranteeing nonce uniqueness across all chunks of a file without
// needing a full 12 random bytes per chunk.
func chunkNonce(prefix [4]byte, index uint64) []byte {
	nonce := make([]byte, 12)
	copy(nonce[:4], prefix[:])
	binary.BigEndian.PutUint64(nonce[4:], index)
	return nonce
}

// streamEncrypt reads plaintext from r in fixed-size chunks and writes
// length-prefixed, independently sealed AES-GCM ciphertext chunks to w.
// This allows encrypting files of arbitrary size while holding at most one
// chunk (streamChunkSize bytes) in memory at a time.
//
// On-disk format written to w:
//
//	[4-byte nonce prefix]
//	repeated: [4-byte big-endian ciphertext length][ciphertext+tag]
func streamEncrypt(r io.Reader, w io.Writer, gcm cipher.AEAD) (int64, error) {
	var noncePrefix [4]byte
	if _, err := rand.Read(noncePrefix[:]); err != nil {
		return 0, fmt.Errorf("failed to generate nonce prefix: %w", err)
	}
	if _, err := w.Write(noncePrefix[:]); err != nil {
		return 0, fmt.Errorf("failed to write nonce prefix: %w", err)
	}

	buf := make([]byte, streamChunkSize)
	var chunkIndex uint64
	var total int64

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			nonce := chunkNonce(noncePrefix, chunkIndex)
			ciphertext := gcm.Seal(nil, nonce, buf[:n], nil)

			var lenPrefix [4]byte
			binary.BigEndian.PutUint32(lenPrefix[:], uint32(len(ciphertext)))
			if _, werr := w.Write(lenPrefix[:]); werr != nil {
				return total, fmt.Errorf("failed to write chunk length: %w", werr)
			}
			if _, werr := w.Write(ciphertext); werr != nil {
				return total, fmt.Errorf("failed to write chunk: %w", werr)
			}

			chunkIndex++
			total += int64(n)
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return total, fmt.Errorf("failed to read plaintext: %w", err)
		}
	}

	return total, nil
}

// streamDecrypt reads the format written by streamEncrypt from r, verifies
// and decrypts each chunk, and writes the recovered plaintext to w.
// Returns ErrInvalidPassword if authentication fails on any chunk (wrong
// password or corrupted/tampered data).
func streamDecrypt(r io.Reader, w io.Writer, gcm cipher.AEAD) error {
	var noncePrefix [4]byte
	if _, err := io.ReadFull(r, noncePrefix[:]); err != nil {
		return errors.New("invalid encrypted file format")
	}

	var chunkIndex uint64
	var lenBuf [4]byte

	for {
		_, err := io.ReadFull(r, lenBuf[:])
		if err == io.EOF {
			// Clean end of stream
			break
		}
		if err != nil {
			return errors.New("invalid encrypted file format")
		}

		chunkLen := binary.BigEndian.Uint32(lenBuf[:])
		// Sanity bound: a sealed chunk is at most streamChunkSize + GCM tag
		// overhead. Reject anything larger to avoid unbounded allocation
		// from a corrupted or malicious length prefix.
		if int(chunkLen) > streamChunkSize+64 {
			return errors.New("invalid encrypted chunk size")
		}

		ciphertext := make([]byte, chunkLen)
		if _, err := io.ReadFull(r, ciphertext); err != nil {
			return errors.New("truncated encrypted file")
		}

		nonce := chunkNonce(noncePrefix, chunkIndex)
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return ErrInvalidPassword
		}

		if _, err := w.Write(plaintext); err != nil {
			return fmt.Errorf("failed to write plaintext: %w", err)
		}

		chunkIndex++
	}

	return nil
}

// StoreFileStream stores a file by streaming from r directly to its final
// location on disk (encrypting on the fly if a password is supplied),
// without ever buffering the whole file in memory. declaredSize is the
// client-declared size (e.g. multipart header) used for an early quota/size
// check; the authoritative size used for metadata/quota accounting is the
// number of bytes actually written.
func (s *Storage) StoreFileStream(filename string, r io.Reader, declaredSize int64, expirationOption string, password string) (*FileMetadata, error) {
	if declaredSize > 0 && declaredSize > s.config.MaxFileSize {
		return nil, ErrFileTooLarge
	}

	if atomic.LoadInt64(&s.totalSize)+declaredSize > s.config.GetMaxStorageBytes() {
		return nil, ErrStorageFull
	}

	expOption, exists := s.expirationOptions[expirationOption]
	if !exists {
		return nil, ErrInvalidExpiration
	}

	id := s.generateID()
	filePath := filepath.Join(s.basePath, id)

	metadata := &FileMetadata{
		ID:           id,
		Filename:     id,
		OriginalName: filename,
		UploadedAt:   time.Now(),
		ExpiresAt:    time.Now().Add(time.Duration(expOption.MaxDays) * 24 * time.Hour),
		MaxDownloads: expOption.MaxDownloads,
		Downloads:    0,
		IsEncrypted:  password != "",
	}

	out, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	// limitedReader enforces the configured max file size while streaming,
	// so an oversized or mislabeled upload is aborted early instead of
	// filling the disk.
	limited := &io.LimitedReader{R: r, N: s.config.MaxFileSize + 1}

	var written int64
	if password != "" {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			out.Close()
			os.Remove(filePath)
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}

		saltHashHex := make([]byte, 16)
		copy(saltHashHex, salt)
		metadata.PasswordHash = hashPassword(password, string(salt))
		metadata.Salt = string(salt)

		gcm, err := newGCMForPassword(password, salt)
		if err != nil {
			out.Close()
			os.Remove(filePath)
			return nil, err
		}

		// Prefix the file with the salt, then the streamed ciphertext.
		if _, err := out.Write(salt); err != nil {
			out.Close()
			os.Remove(filePath)
			return nil, fmt.Errorf("failed to write salt: %w", err)
		}

		written, err = streamEncrypt(limited, out, gcm)
		if err != nil {
			out.Close()
			os.Remove(filePath)
			return nil, fmt.Errorf("failed to encrypt file: %w", err)
		}
	} else {
		written, err = io.Copy(out, limited)
		if err != nil {
			out.Close()
			os.Remove(filePath)
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
	}

	if err := out.Close(); err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to finalize file: %w", err)
	}

	// If the limited reader was exhausted exactly at the cap, there may be
	// more data than allowed; detect by trying to read one more byte.
	if written > s.config.MaxFileSize {
		os.Remove(filePath)
		return nil, ErrFileTooLarge
	}

	metadata.Size = written

	s.mu.Lock()
	if atomic.LoadInt64(&s.totalSize)+written-declaredSize > s.config.GetMaxStorageBytes() && declaredSize == 0 {
		// Re-check quota using the real size when the declared size wasn't
		// known up front (declaredSize == 0 means caller didn't supply one).
		s.mu.Unlock()
		os.Remove(filePath)
		return nil, ErrStorageFull
	}
	s.metadata[id] = metadata
	s.mu.Unlock()

	atomic.AddInt64(&s.totalSize, written)
	s.markDirty()

	return metadata, nil
}

// StoreFile stores a file already fully in memory. Retained for callers
// (tests, small in-memory buffers) that don't need streaming; internally
// delegates to StoreFileStream so there is a single code path for disk
// writes and encryption.
func (s *Storage) StoreFile(filename string, data []byte, expirationOption string, password string) (*FileMetadata, error) {
	return s.StoreFileStream(filename, bytesReader(data), int64(len(data)), expirationOption, password)
}

// bytesReader avoids importing bytes just for this helper's call sites.
func bytesReader(data []byte) io.Reader {
	return &sliceReader{data: data}
}

type sliceReader struct {
	data []byte
	pos  int
}

func (sr *sliceReader) Read(p []byte) (int, error) {
	if sr.pos >= len(sr.data) {
		return 0, io.EOF
	}
	n := copy(p, sr.data[sr.pos:])
	sr.pos += n
	return n, nil
}

// lookupForDownload validates a download request (existence, expiry,
// download count, password) and returns the metadata without reading any
// file data. Split out from GetFileStream so callers can fail fast before
// opening the file on disk.
func (s *Storage) lookupForDownload(id, password string) (*FileMetadata, error) {
	s.mu.RLock()
	metadata, exists := s.metadata[id]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrFileNotFound
	}

	if time.Now().After(metadata.ExpiresAt) {
		return nil, ErrFileExpired
	}

	if metadata.Downloads >= metadata.MaxDownloads {
		return nil, ErrFileExpired
	}

	if metadata.IsEncrypted {
		if password == "" {
			return nil, ErrInvalidPassword
		}
		expectedHash := hashPassword(password, metadata.Salt)
		if expectedHash != metadata.PasswordHash {
			return nil, ErrInvalidPassword
		}
	}

	return metadata, nil
}

// ValidateDownload validates a download request (existence, expiry,
// download count, password) without reading any file content, so the
// handler can return the correct HTTP status (404/410/401) and run
// brute-force bookkeeping before committing to streaming a response body.
func (s *Storage) ValidateDownload(id, password string) (*FileMetadata, error) {
	return s.lookupForDownload(id, password)
}

// GetFileStream streams a stored file (decrypting on the fly if necessary)
// directly to w, without ever buffering the whole file in memory. It
// returns the metadata so the caller can set response headers (filename,
// content-length) before/while streaming.
func (s *Storage) GetFileStream(id, password string, w io.Writer) (*FileMetadata, error) {
	metadata, err := s.lookupForDownload(id, password)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(s.basePath, id)
	in, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer in.Close()

	if metadata.IsEncrypted {
		salt := []byte(metadata.Salt)
		gcm, err := newGCMForPassword(password, salt)
		if err != nil {
			return nil, err
		}

		// Skip the salt prefix written at the start of the file.
		if _, err := io.CopyN(io.Discard, in, int64(len(salt))); err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		if err := streamDecrypt(in, w, gcm); err != nil {
			return nil, err
		}
	} else {
		if _, err := io.Copy(w, in); err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	s.mu.Lock()
	metadata.Downloads++
	s.mu.Unlock()
	s.markDirty()

	return metadata, nil
}

// GetFile retrieves a whole file into memory for download. Retained for
// callers (tests, small files) that need the data as a []byte; internally
// delegates to GetFileStream so there is a single code path for decryption.
func (s *Storage) GetFile(id, password string) ([]byte, *FileMetadata, error) {
	var buf writerBuf
	metadata, err := s.GetFileStream(id, password, &buf)
	if err != nil {
		return nil, nil, err
	}
	return buf.data, metadata, nil
}

// writerBuf is a minimal io.Writer that accumulates written bytes, used to
// adapt the streaming API back to a []byte for legacy callers.
type writerBuf struct {
	data []byte
}

func (b *writerBuf) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

// GetFileInfo returns metadata for a file without reading its content from
// disk, used by the download handler to set Content-Length before
// streaming the body.
func (s *Storage) GetFileInfo(id string) (*FileMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	metadata, exists := s.metadata[id]
	if !exists {
		return nil, ErrFileNotFound
	}
	return metadata, nil
}

// DeleteFile removes a file from storage
func (s *Storage) DeleteFile(id string) error {
	s.mu.Lock()
	metadata, exists := s.metadata[id]
	if !exists {
		s.mu.Unlock()
		return ErrFileNotFound
	}
	delete(s.metadata, id)
	s.mu.Unlock()

	filePath := filepath.Join(s.basePath, id)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file: %w", err)
	}

	atomic.AddInt64(&s.totalSize, -metadata.Size)
	s.markDirty()

	return nil
}

// GetStorageInfo returns storage usage information.
//
// Usage is tracked incrementally in memory (totalSize, updated on every
// store/delete/cleanup) rather than recomputed by walking the filesystem
// and calling os.Stat on every file on every request. This makes the call
// O(1) instead of O(n) and avoids taking a write lock for what used to be
// effectively a read-only reporting call, removing a global contention
// point that previously serialized with every upload/download under load.
func (s *Storage) GetStorageInfo() (usedBytes int64, totalBytes int64, fileCount int) {
	s.mu.RLock()
	count := len(s.metadata)
	s.mu.RUnlock()

	used := atomic.LoadInt64(&s.totalSize)
	return used, s.config.GetMaxStorageBytes(), count
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
	now := time.Now()
	toDelete := []string{}

	for id, metadata := range s.metadata {
		if now.After(metadata.ExpiresAt) || metadata.Downloads >= metadata.MaxDownloads {
			toDelete = append(toDelete, id)
		}
	}

	var freed int64
	for _, id := range toDelete {
		metadata := s.metadata[id]

		filePath := filepath.Join(s.basePath, id)
		os.Remove(filePath)

		freed += metadata.Size
		delete(s.metadata, id)
	}
	s.mu.Unlock()

	if len(toDelete) > 0 {
		atomic.AddInt64(&s.totalSize, -freed)
		s.markDirty()
	}
}
