package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrSessionNotFound    = errors.New("upload session not found")
	ErrSessionExpired     = errors.New("upload session expired")
	ErrInvalidChunkIndex  = errors.New("invalid chunk index")
	ErrChunkSizeMismatch  = errors.New("chunk size does not match expected size")
	ErrIncompleteSession  = errors.New("not all chunks have been received")
	ErrSessionAlreadyDone = errors.New("upload session already completed")
)

// chunkStagingDir is the subdirectory (relative to the storage base path)
// where in-progress chunked upload sessions stage their data before being
// assembled into a final stored file.
const chunkStagingDir = ".chunks"

// ChunkSession tracks an in-progress chunked upload. A session is created
// by UploadHandler's /api/upload/init and lives until /api/upload/complete
// (or /abort) is called, or until it is reaped for being idle too long.
//
// Chunks are written directly into their final offset in a pre-sized
// staging file via WriteAt, so chunks may arrive out of order (bounded by
// the client's concurrency) without needing to buffer them separately.
type ChunkSession struct {
	ID               string
	Filename         string
	DeclaredSize     int64
	ExpirationOption string
	Password         string
	ChunkSize        int64
	TotalChunks      int

	stagingPath string

	mu           sync.Mutex
	file         *os.File
	received     map[int]bool
	receivedSize int64
	lastActivity time.Time
	done         bool
}

// ChunkSessionManager manages the lifecycle of chunked upload sessions:
// creation, chunk writes, completion (handing the assembled file off to
// Storage's normal streaming store pipeline), and reaping abandoned
// sessions so their staging files don't accumulate on disk forever.
type ChunkSessionManager struct {
	storage *Storage
	dir     string
	timeout time.Duration

	mu       sync.Mutex
	sessions map[string]*ChunkSession

	stop chan struct{}
}

// NewChunkSessionManager creates a session manager rooted at
// {storage.basePath}/.chunks and starts its background reaper.
func NewChunkSessionManager(s *Storage, timeout time.Duration) (*ChunkSessionManager, error) {
	dir := filepath.Join(s.basePath, chunkStagingDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create chunk staging directory: %w", err)
	}

	m := &ChunkSessionManager{
		storage:  s,
		dir:      dir,
		timeout:  timeout,
		sessions: make(map[string]*ChunkSession),
		stop:     make(chan struct{}),
	}

	// Clean any staging files left over from a previous process crash /
	// restart - they can't be resumed since session state was in memory.
	m.cleanupOrphanedStagingFiles()

	go m.startReaper()

	return m, nil
}

// Close stops the background reaper and removes all staging files for
// sessions that never completed.
func (m *ChunkSessionManager) Close() {
	close(m.stop)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sess := range m.sessions {
		sess.mu.Lock()
		if sess.file != nil {
			sess.file.Close()
		}
		sess.mu.Unlock()
	}
}

func (m *ChunkSessionManager) cleanupOrphanedStagingFiles() {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		os.Remove(filepath.Join(m.dir, entry.Name()))
	}
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// InitSession creates a new chunked upload session for a file of the given
// declared size, pre-allocating a staging file on disk sized to fit the
// whole upload. Returns the session along with the chunk size and total
// chunk count the client should use.
func (m *ChunkSessionManager) InitSession(filename string, declaredSize int64, expirationOption, password string, chunkSize int64) (*ChunkSession, error) {
	if declaredSize <= 0 {
		return nil, errors.New("declared size must be positive")
	}
	if chunkSize <= 0 {
		return nil, errors.New("chunk size must be positive")
	}

	id, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session id: %w", err)
	}

	totalChunks := int((declaredSize + chunkSize - 1) / chunkSize)
	stagingPath := filepath.Join(m.dir, id+".part")

	f, err := os.OpenFile(stagingPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging file: %w", err)
	}
	// Pre-size the file so WriteAt for any chunk index succeeds without
	// needing chunks to arrive in order.
	if err := f.Truncate(declaredSize); err != nil {
		f.Close()
		os.Remove(stagingPath)
		return nil, fmt.Errorf("failed to allocate staging file: %w", err)
	}

	sess := &ChunkSession{
		ID:               id,
		Filename:         filename,
		DeclaredSize:     declaredSize,
		ExpirationOption: expirationOption,
		Password:         password,
		ChunkSize:        chunkSize,
		TotalChunks:      totalChunks,
		stagingPath:      stagingPath,
		file:             f,
		received:         make(map[int]bool, totalChunks),
		lastActivity:     time.Now(),
	}

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	return sess, nil
}

// GetSession retrieves an active session by ID.
func (m *ChunkSessionManager) GetSession(id string) (*ChunkSession, error) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	expired := time.Since(sess.lastActivity) > m.timeout
	done := sess.done
	sess.mu.Unlock()

	if done {
		return nil, ErrSessionAlreadyDone
	}
	if expired {
		m.removeSession(id)
		return nil, ErrSessionExpired
	}

	return sess, nil
}

// WriteChunk writes chunk data at the correct offset for the given index.
func (m *ChunkSessionManager) WriteChunk(sess *ChunkSession, index int, data []byte) error {
	if index < 0 || index >= sess.TotalChunks {
		return ErrInvalidChunkIndex
	}

	expectedSize := sess.ChunkSize
	if index == sess.TotalChunks-1 {
		// Last chunk may be smaller than the standard chunk size.
		expectedSize = sess.DeclaredSize - int64(index)*sess.ChunkSize
	}
	if int64(len(data)) != expectedSize {
		return ErrChunkSizeMismatch
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.done {
		return ErrSessionAlreadyDone
	}

	offset := int64(index) * sess.ChunkSize
	if _, err := sess.file.WriteAt(data, offset); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	if !sess.received[index] {
		sess.received[index] = true
		sess.receivedSize += int64(len(data))
	}
	sess.lastActivity = time.Now()

	return nil
}

// Progress returns how many chunks (and bytes) have been received so far.
func (sess *ChunkSession) Progress() (chunksReceived int, bytesReceived int64) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return len(sess.received), sess.receivedSize
}

// CompleteSession verifies all chunks have been received, then streams the
// assembled staging file through Storage's normal StoreFileStream pipeline
// (which handles encryption, quota checks, and metadata). The staging file
// and in-memory session are only removed on success or on a terminal
// storage error; if chunks are still missing, the session is left intact
// so the client can finish sending them and call complete again.
func (m *ChunkSessionManager) CompleteSession(id string) (*FileMetadata, error) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	if sess.done {
		sess.mu.Unlock()
		return nil, ErrSessionAlreadyDone
	}
	allReceived := len(sess.received) == sess.TotalChunks
	if !allReceived {
		sess.lastActivity = time.Now()
		sess.mu.Unlock()
		return nil, ErrIncompleteSession
	}
	stagingPath := sess.stagingPath
	filename := sess.Filename
	declaredSize := sess.DeclaredSize
	expirationOption := sess.ExpirationOption
	password := sess.Password
	file := sess.file
	sess.done = true
	sess.mu.Unlock()

	defer m.removeSession(id)

	// Flush and rewind for reading; the file was opened O_RDWR so we can
	// reuse the same handle rather than reopening by path.
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to sync staged upload: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to seek staged upload: %w", err)
	}

	metadata, err := m.storage.StoreFileStream(filename, file, declaredSize, expirationOption, password)
	file.Close()
	os.Remove(stagingPath)

	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// AbortSession cancels an in-progress session and removes its staging
// data immediately.
func (m *ChunkSessionManager) AbortSession(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return ErrSessionNotFound
	}

	sess.mu.Lock()
	sess.done = true
	if sess.file != nil {
		sess.file.Close()
	}
	stagingPath := sess.stagingPath
	sess.mu.Unlock()

	os.Remove(stagingPath)
	m.removeSession(id)

	return nil
}

func (m *ChunkSessionManager) removeSession(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// startReaper periodically scans for sessions that have been idle longer
// than the configured timeout and removes their staging files, so an
// abandoned upload (client closed the tab mid-transfer, network dropped
// permanently, etc.) doesn't leak disk space indefinitely.
func (m *ChunkSessionManager) startReaper() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.reapExpired()
		}
	}
}

func (m *ChunkSessionManager) reapExpired() {
	now := time.Now()

	m.mu.Lock()
	var expired []string
	for id, sess := range m.sessions {
		sess.mu.Lock()
		idle := now.Sub(sess.lastActivity) > m.timeout
		done := sess.done
		sess.mu.Unlock()
		if idle && !done {
			expired = append(expired, id)
		}
	}
	m.mu.Unlock()

	for _, id := range expired {
		m.mu.Lock()
		sess, ok := m.sessions[id]
		m.mu.Unlock()
		if !ok {
			continue
		}

		sess.mu.Lock()
		sess.done = true
		if sess.file != nil {
			sess.file.Close()
		}
		stagingPath := sess.stagingPath
		sess.mu.Unlock()

		os.Remove(stagingPath)
		m.removeSession(id)
	}
}
