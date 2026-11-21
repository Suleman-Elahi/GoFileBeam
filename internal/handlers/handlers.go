package handlers

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gofilebeam/internal/config"
	"gofilebeam/internal/security"
	staticpkg "gofilebeam/internal/static"
	"gofilebeam/internal/storage"
)

// Handler struct holds dependencies
type Handler struct {
	storage       *storage.Storage
	config        *config.Config
	sandbox       *security.Sandbox
	rateLimiter   *security.RateLimiter
	bruteForce    *security.BruteForceProtection
	fileValidator *security.FileValidator
	chunkManager  *storage.ChunkSessionManager
}

// NewHandler creates a new handler instance (legacy, without security)
func NewHandler(storageInstance *storage.Storage, config *config.Config) *Handler {
	chunkManager, err := storage.NewChunkSessionManager(storageInstance, config.ChunkSessionTimeout)
	if err != nil {
		log.Printf("Warning: chunked upload support disabled: %v", err)
	}
	return &Handler{
		storage:      storageInstance,
		config:       config,
		chunkManager: chunkManager,
	}
}

// NewHandlerWithSecurity creates a new handler instance with security components
func NewHandlerWithSecurity(storageInstance *storage.Storage, config *config.Config, sandbox *security.Sandbox, rateLimiter *security.RateLimiter, bruteForce *security.BruteForceProtection, fileValidator *security.FileValidator) *Handler {
	chunkManager, err := storage.NewChunkSessionManager(storageInstance, config.ChunkSessionTimeout)
	if err != nil {
		log.Printf("Warning: chunked upload support disabled: %v", err)
	}
	return &Handler{
		storage:       storageInstance,
		config:        config,
		sandbox:       sandbox,
		rateLimiter:   rateLimiter,
		bruteForce:    bruteForce,
		fileValidator: fileValidator,
		chunkManager:  chunkManager,
	}
}

// UploadRequest represents the upload request from the UI
type UploadRequest struct {
	Filename         string `json:"filename"`
	ExpirationOption string `json:"expiration_option"`
	Password         string `json:"password,omitempty"`
}

// UploadResponse represents the response after upload
type UploadResponse struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
	MaxDownloads int       `json:"max_downloads"`
}

// DownloadRequest represents the download request
type DownloadRequest struct {
	Password string `json:"password,omitempty"`
}

// StorageInfoResponse represents storage information
type StorageInfoResponse struct {
	UsedBytes    int64   `json:"used_bytes"`
	TotalBytes   int64   `json:"total_bytes"`
	FileCount    int     `json:"file_count"`
	UsagePercent float64 `json:"usage_percent"`
}

// --- Chunked upload protocol ---
//
// Large uploads are split by the client into fixed-size chunks and sent as
// a sequence of small, independent requests instead of one large request
// body. This exists specifically to work around reverse-proxy/CDN request
// body size limits (e.g. Cloudflare's 100 MB limit on Free/Pro plans) that
// sit in front of this app and cannot be configured away at the
// application level: no single chunk request ever needs to exceed
// config.ChunkSize, regardless of total file size.
//
// Flow:
//   1. POST /api/upload/init    -> create a session, get chunk_size/total_chunks
//   2. POST /api/upload/chunk   -> repeated per chunk, raw bytes body
//   3. POST /api/upload/complete -> assemble + store, returns normal UploadResponse
//   (or POST /api/upload/abort to cancel early and free staging disk space)

// ChunkInitRequest is the JSON body for /api/upload/init.
type ChunkInitRequest struct {
	Filename         string `json:"filename"`
	Size             int64  `json:"size"`
	ExpirationOption string `json:"expiration_option"`
	Password         string `json:"password,omitempty"`
}

// ChunkInitResponse tells the client how to split the file.
type ChunkInitResponse struct {
	SessionID   string `json:"session_id"`
	ChunkSize   int64  `json:"chunk_size"`
	TotalChunks int    `json:"total_chunks"`
}

// ChunkAckResponse is returned after each chunk upload.
type ChunkAckResponse struct {
	Received       int   `json:"received"`
	BytesReceived   int64 `json:"bytes_received"`
	TotalChunks     int   `json:"total_chunks"`
}

// ChunkCompleteRequest is the JSON body for /api/upload/complete and
// /api/upload/abort.
type ChunkCompleteRequest struct {
	SessionID string `json:"session_id"`
}

// InitChunkedUploadHandler creates a new chunked upload session.
func (h *Handler) InitChunkedUploadHandler(w http.ResponseWriter, r *http.Request) {
	if h.chunkManager == nil {
		http.Error(w, "Chunked uploads are not available", http.StatusServiceUnavailable)
		return
	}

	var req ChunkInitRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxUploadMemory)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Filename == "" {
		http.Error(w, "filename is required", http.StatusBadRequest)
		return
	}
	if req.Size <= 0 {
		http.Error(w, "size must be positive", http.StatusBadRequest)
		return
	}
	if req.Size > h.config.MaxFileSize {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}
	if h.fileValidator != nil {
		if err := h.fileValidator.ValidateFilename(req.Filename); err != nil {
			http.Error(w, "Invalid filename: "+req.Filename, http.StatusBadRequest)
			return
		}
	}

	expirationOption := expirationOptionOrDefault(req.ExpirationOption)

	sess, err := h.chunkManager.InitSession(req.Filename, req.Size, expirationOption, req.Password, h.config.ChunkSize)
	if err != nil {
		http.Error(w, "Failed to start upload session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChunkInitResponse{
		SessionID:   sess.ID,
		ChunkSize:   sess.ChunkSize,
		TotalChunks: sess.TotalChunks,
	})
}

// UploadChunkHandler receives a single chunk's raw bytes for an existing
// session. session_id and index are passed as query parameters so the
// request body is pure binary data (no multipart overhead per chunk).
func (h *Handler) UploadChunkHandler(w http.ResponseWriter, r *http.Request) {
	if h.chunkManager == nil {
		http.Error(w, "Chunked uploads are not available", http.StatusServiceUnavailable)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	indexStr := r.URL.Query().Get("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		http.Error(w, "invalid chunk index", http.StatusBadRequest)
		return
	}

	sess, err := h.chunkManager.GetSession(sessionID)
	if err != nil {
		writeChunkSessionError(w, err)
		return
	}

	// Bound how much we'll read for a single chunk: the declared chunk
	// size plus a small margin, so a client (malicious or buggy) can't
	// force an unbounded read for one "chunk" request.
	limit := sess.ChunkSize + 4096
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		http.Error(w, "Failed to read chunk: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if int64(len(data)) > limit {
		http.Error(w, "Chunk too large", http.StatusBadRequest)
		return
	}

	if err := h.chunkManager.WriteChunk(sess, index, data); err != nil {
		switch err {
		case storage.ErrInvalidChunkIndex, storage.ErrChunkSizeMismatch:
			http.Error(w, err.Error(), http.StatusBadRequest)
		case storage.ErrSessionAlreadyDone:
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "Failed to write chunk: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	received, bytesReceived := sess.Progress()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChunkAckResponse{
		Received:      received,
		BytesReceived: bytesReceived,
		TotalChunks:   sess.TotalChunks,
	})
}

// CompleteChunkedUploadHandler assembles all received chunks into the
// final stored file (streaming through the same encryption/quota pipeline
// as the single-request upload path) and returns the standard
// UploadResponse.
func (h *Handler) CompleteChunkedUploadHandler(w http.ResponseWriter, r *http.Request) {
	if h.chunkManager == nil {
		http.Error(w, "Chunked uploads are not available", http.StatusServiceUnavailable)
		return
	}

	var req ChunkCompleteRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxUploadMemory)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	metadata, err := h.chunkManager.CompleteSession(req.SessionID)
	if err != nil {
		switch err {
		case storage.ErrSessionNotFound, storage.ErrSessionExpired, storage.ErrSessionAlreadyDone:
			writeChunkSessionError(w, err)
		case storage.ErrIncompleteSession:
			http.Error(w, "Not all chunks have been uploaded", http.StatusBadRequest)
		default:
			writeStoreError(w, err)
		}
		return
	}

	h.finalizeUpload(w, r, metadata)
}

// AbortChunkedUploadHandler cancels an in-progress session early and frees
// its staging disk space immediately, instead of waiting for the idle
// reaper to do it.
func (h *Handler) AbortChunkedUploadHandler(w http.ResponseWriter, r *http.Request) {
	if h.chunkManager == nil {
		http.Error(w, "Chunked uploads are not available", http.StatusServiceUnavailable)
		return
	}

	var req ChunkCompleteRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxUploadMemory)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	if err := h.chunkManager.AbortSession(req.SessionID); err != nil {
		writeChunkSessionError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeChunkSessionError maps chunk session errors to HTTP responses.
func writeChunkSessionError(w http.ResponseWriter, err error) {
	switch err {
	case storage.ErrSessionNotFound:
		http.Error(w, "Upload session not found", http.StatusNotFound)
	case storage.ErrSessionExpired:
		http.Error(w, "Upload session expired", http.StatusGone)
	case storage.ErrSessionAlreadyDone:
		http.Error(w, "Upload session already completed", http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// maxUploadMemory bounds the in-memory parts of a multipart request (form
// field values like password/expiration_option). File parts are always
// streamed to disk via multipart.Reader and are never subject to this
// limit or buffered in memory.
const maxUploadMemory = 1 << 20 // 1 MB, just for non-file form fields

// UploadHandler handles file uploads. It streams multipart file parts
// directly to disk (encrypting on the fly if a password is supplied)
// instead of reading them fully into memory first, so memory usage per
// request is bounded by a small constant rather than by file size. This
// matters for concurrent-user throughput: previously every simultaneous
// upload held its entire file (and a duplicate copy during encryption) in
// RAM at once.
func (h *Handler) UploadHandler(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	var (
		expirationOption string
		password         string
		storedFiles      []*storage.FileMetadata
		singleOriginal   string
	)

	// tempFiles collects temp paths for multi-file uploads so they can be
	// zipped together on disk (not in memory) and cleaned up afterwards.
	var tempFiles []string
	defer func() {
		for _, p := range tempFiles {
			os.Remove(p)
		}
	}()

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		formName := part.FormName()

		switch formName {
		case "expiration_option":
			buf, _ := io.ReadAll(io.LimitReader(part, maxUploadMemory))
			expirationOption = string(buf)
			part.Close()
			continue
		case "password":
			buf, _ := io.ReadAll(io.LimitReader(part, maxUploadMemory))
			password = string(buf)
			part.Close()
			continue
		}

		if formName != "files" && formName != "file" {
			part.Close()
			continue
		}

		filename := part.FileName()
		if filename == "" {
			part.Close()
			continue
		}

		if h.fileValidator != nil {
			if err := h.fileValidator.ValidateFilename(filename); err != nil {
				part.Close()
				http.Error(w, "Invalid filename: "+filename, http.StatusBadRequest)
				return
			}
		}

		if formName == "file" {
			// Single-file legacy field: store directly without an
			// intermediate temp file.
			singleOriginal = filename
			metadata, serr := h.storage.StoreFileStream(filename, part, 0, expirationOptionOrDefault(expirationOption), password)
			part.Close()
			if serr != nil {
				writeStoreError(w, serr)
				return
			}
			storedFiles = append(storedFiles, metadata)
			continue
		}

		// "files" field: spill each part to a temp file on disk. For a
		// single file this is stored directly; for multiple files these
		// temp files are streamed into a ZIP afterwards, still without
		// buffering full file contents in memory.
		tempPath := filepath.Join(h.config.StoragePath, "upload-"+randomSuffix()+".tmp")
		out, cerr := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if cerr != nil {
			part.Close()
			http.Error(w, "Failed to stage upload: "+cerr.Error(), http.StatusInternalServerError)
			return
		}
		_, cerr = io.Copy(out, part)
		out.Close()
		part.Close()
		if cerr != nil {
			os.Remove(tempPath)
			http.Error(w, "Failed to stage upload: "+cerr.Error(), http.StatusInternalServerError)
			return
		}

		tempFiles = append(tempFiles, tempPath)
		if singleOriginal == "" {
			singleOriginal = filename
		} else {
			singleOriginal = "" // marks "multiple distinct names seen"
		}
	}

	if len(storedFiles) == 0 && len(tempFiles) == 0 {
		http.Error(w, "No file provided", http.StatusBadRequest)
		return
	}

	expirationOption = expirationOptionOrDefault(expirationOption)

	var metadata *storage.FileMetadata

	switch {
	case len(storedFiles) == 1 && len(tempFiles) == 0:
		metadata = storedFiles[0]

	case len(tempFiles) == 1:
		// Single file via "files" field: stream the staged temp file
		// straight into storage.
		f, ferr := os.Open(tempFiles[0])
		if ferr != nil {
			http.Error(w, "Failed to read staged upload: "+ferr.Error(), http.StatusInternalServerError)
			return
		}
		info, _ := f.Stat()
		var size int64
		if info != nil {
			size = info.Size()
		}
		// Recover original filename from the staged part; multipart
		// doesn't let us re-query it, so we rely on singleOriginal.
		m, serr := h.storage.StoreFileStream(originalNameOrDefault(singleOriginal), f, size, expirationOption, password)
		f.Close()
		if serr != nil {
			writeStoreError(w, serr)
			return
		}
		metadata = m

	case len(tempFiles) > 1:
		// Multiple files: stream them into a ZIP staged on disk, then
		// stream that ZIP into storage. Nothing here buffers a whole file
		// in memory; archive/zip still needs an io.Writer that doesn't
		// require seeking back for this use (we write a plain streamed zip
		// to a temp file, which does require the writer to support the
		// local/central-directory layout zip.Writer handles internally via
		// its own small buffers, not full file buffering).
		zipPath := filepath.Join(h.config.StoragePath, "upload-"+randomSuffix()+".zip")
		zf, zerr := os.OpenFile(zipPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if zerr != nil {
			http.Error(w, "Failed to create ZIP: "+zerr.Error(), http.StatusInternalServerError)
			return
		}
		defer os.Remove(zipPath)

		zipWriter := zip.NewWriter(zf)
		for i, tempPath := range tempFiles {
			tf, operr := os.Open(tempPath)
			if operr != nil {
				continue
			}
			name := fmt.Sprintf("file_%d", i+1)
			zipFile, cerr := zipWriter.Create(name)
			if cerr == nil {
				io.Copy(zipFile, tf)
			}
			tf.Close()
		}
		if err := zipWriter.Close(); err != nil {
			zf.Close()
			http.Error(w, "Failed to create ZIP: "+err.Error(), http.StatusInternalServerError)
			return
		}
		zf.Close()

		zr, zrerr := os.Open(zipPath)
		if zrerr != nil {
			http.Error(w, "Failed to read ZIP: "+zrerr.Error(), http.StatusInternalServerError)
			return
		}
		info, _ := zr.Stat()
		var size int64
		if info != nil {
			size = info.Size()
		}

		zipFilename := fmt.Sprintf("files_%s.zip", time.Now().Format("20060102_150405"))
		m, serr := h.storage.StoreFileStream(zipFilename, zr, size, expirationOption, password)
		zr.Close()
		if serr != nil {
			writeStoreError(w, serr)
			return
		}
		metadata = m
	}

	if metadata == nil {
		http.Error(w, "Failed to store file", http.StatusInternalServerError)
		return
	}

	h.finalizeUpload(w, r, metadata)
}

// finalizeUpload applies sandbox security to a newly stored file and
// writes the standard UploadResponse JSON body. Shared by the legacy
// single-request upload path and the chunked upload completion path so
// both produce an identical response shape.
func (h *Handler) finalizeUpload(w http.ResponseWriter, r *http.Request, metadata *storage.FileMetadata) {
	if h.sandbox != nil {
		filePath := filepath.Join(h.config.StoragePath, metadata.ID)
		if err := h.sandbox.SecureFile(filePath); err != nil {
			log.Printf("Warning: Failed to apply sandbox security to %s: %v", metadata.ID, err)
		} else {
			log.Printf("✓ Sandbox security applied to file: %s", metadata.ID)
		}
	}

	protocol := "http"
	if h.config.EnableHTTPS {
		protocol = "https"
	}

	host := r.Host
	if host == "" {
		host = fmt.Sprintf("%s:%s", h.config.Host, h.config.Port)
	}

	url := fmt.Sprintf("%s://%s/api/download/%s", protocol, host, metadata.ID)

	response := UploadResponse{
		ID:           metadata.ID,
		URL:          url,
		ExpiresAt:    metadata.ExpiresAt,
		MaxDownloads: metadata.MaxDownloads,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// expirationOptionOrDefault returns the given option, or the default if empty.
func expirationOptionOrDefault(opt string) string {
	if opt == "" {
		return "1 Download or 1 Day"
	}
	return opt
}

// originalNameOrDefault returns name, or a generic fallback if it was
// cleared because multiple distinct filenames were seen on a single-file
// code path (shouldn't normally happen).
func originalNameOrDefault(name string) string {
	if name == "" {
		return "upload"
	}
	return name
}

// randomSuffix returns a short random hex string used to name temp files
// uniquely per request without colliding under concurrent uploads.
func randomSuffix() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// writeStoreError maps storage errors to HTTP responses.
func writeStoreError(w http.ResponseWriter, err error) {
	switch err {
	case storage.ErrFileTooLarge:
		http.Error(w, "File too large", http.StatusBadRequest)
	case storage.ErrStorageFull:
		http.Error(w, "Storage quota exceeded", http.StatusInsufficientStorage)
	case storage.ErrInvalidExpiration:
		http.Error(w, "Invalid expiration option", http.StatusBadRequest)
	default:
		http.Error(w, "Failed to store file: "+err.Error(), http.StatusInternalServerError)
	}
}

// DownloadHandler handles file downloads
func (h *Handler) DownloadHandler(w http.ResponseWriter, r *http.Request) {
	// Extract file ID from URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	id := pathParts[3]

	// Get client IP for brute force protection
	clientIP := security.GetClientIP(r)

	// Check if IP is blocked for this file
	if h.bruteForce != nil && h.bruteForce.IsBlocked(id, clientIP) {
		log.Printf("Blocked download attempt from %s for file %s (brute force protection)", clientIP, id)
		http.Error(w, "Too many failed attempts. Please try again later.", http.StatusTooManyRequests)
		return
	}

	// Get password from query parameter or form
	password := r.URL.Query().Get("password")
	if password == "" {
		password = r.FormValue("password")
	}

	// Validate existence, expiry, download count, and password (including
	// brute-force bookkeeping) before sending any response headers, so
	// error cases still get a correct status code. Only after this
	// succeeds do we commit to streaming the body.
	metadata, err := h.storage.ValidateDownload(id, password)
	if err != nil {
		switch err {
		case storage.ErrFileNotFound:
			http.Error(w, "File not found", http.StatusNotFound)
		case storage.ErrFileExpired:
			http.Error(w, "File has expired", http.StatusGone)
		case storage.ErrInvalidPassword:
			if h.bruteForce != nil {
				if !h.bruteForce.RecordFailedAttempt(id, clientIP) {
					log.Printf("IP %s blocked for file %s after repeated failed attempts", clientIP, id)
					http.Error(w, "Too many failed attempts. Please try again later.", http.StatusTooManyRequests)
					return
				}
			}
			log.Printf("Invalid password attempt from %s for file %s", clientIP, id)
			http.Error(w, "Invalid password", http.StatusUnauthorized)
		default:
			http.Error(w, "Failed to retrieve file", http.StatusInternalServerError)
		}
		return
	}

	// Reset failed attempts now that the password (if any) has checked out.
	if h.bruteForce != nil {
		h.bruteForce.ResetAttempts(id, clientIP)
	}

	// Set headers for download. Note: for encrypted files the plaintext
	// size equals metadata.Size (the stored size tracks plaintext bytes
	// written during streaming encryption), so Content-Length is accurate
	// even though the on-disk file is larger due to GCM tags/framing.
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", metadata.OriginalName))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", metadata.Size))

	// Stream file data directly to the response writer. For encrypted
	// files this decrypts chunk-by-chunk on the fly; at no point is the
	// whole file held in memory. Validation was already done above, so
	// failures here are unexpected (e.g. disk I/O error) and can only be
	// logged since headers/partial body may already be sent.
	if _, err := h.storage.GetFileStream(id, password, w); err != nil {
		log.Printf("Download streaming failed for file %s from %s: %v", id, clientIP, err)
		return
	}

	log.Printf("✓ File %s downloaded by %s", id, clientIP)
}

// StorageInfoHandler returns storage usage information
func (h *Handler) StorageInfoHandler(w http.ResponseWriter, r *http.Request) {
	usedBytes, totalBytes, fileCount := h.storage.GetStorageInfo()

	usagePercent := 0.0
	if totalBytes > 0 {
		usagePercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	response := StorageInfoResponse{
		UsedBytes:    usedBytes,
		TotalBytes:   totalBytes,
		FileCount:    fileCount,
		UsagePercent: usagePercent,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HealthHandler returns health status
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// UploadConfigResponse exposes the upload-related limits and thresholds
// the client needs to decide whether to use the single-request or chunked
// upload path, without hardcoding them in the frontend.
type UploadConfigResponse struct {
	MaxFileSize          int64 `json:"max_file_size"`
	ChunkSize            int64 `json:"chunk_size"`
	ChunkUploadThreshold int64 `json:"chunk_upload_threshold"`
	ChunkedUploadEnabled bool  `json:"chunked_upload_enabled"`
	ChunkConcurrency     int   `json:"chunk_concurrency"`
}

// UploadConfigHandler returns the current upload configuration so the
// frontend can decide, per file, whether to use the simple single-request
// upload or split into chunks to stay under any reverse proxy / CDN body
// size limit sitting in front of this server.
func (h *Handler) UploadConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UploadConfigResponse{
		MaxFileSize:          h.config.MaxFileSize,
		ChunkSize:            h.config.ChunkSize,
		ChunkUploadThreshold: h.config.ChunkUploadThreshold,
		ChunkedUploadEnabled: h.chunkManager != nil,
		ChunkConcurrency:     h.config.ChunkConcurrency,
	})
}

// ServeUI serves the HTML UI from embedded files
func (h *Handler) ServeUI(w http.ResponseWriter, r *http.Request) {
	// Serve index.html for root path
	if r.URL.Path == "/" {
		h.serveEmbeddedFile(w, r, "files/index.html")
		return
	}

	// Check if it's a download page
	if strings.HasPrefix(r.URL.Path, "/download/") {
		h.serveEmbeddedFile(w, r, "files/pages/download.html")
		return
	}

	// Check if it's a static page
	staticPages := map[string]string{
		"/privacy":  "privacy.html",
		"/terms":    "terms.html",
		"/security": "security.html",
		"/help":     "help.html",
	}

	// Check if it's a page path
	for path, file := range staticPages {
		if r.URL.Path == path || r.URL.Path == path+"/" {
			h.serveEmbeddedFile(w, r, "files/pages/"+file)
			return
		}
	}

	// Serve static files
	staticPath := "files" + r.URL.Path
	if _, err := staticpkg.Files.Open(staticPath); err == nil {
		h.serveEmbeddedFile(w, r, staticPath)
		return
	}

	// Serve from pages directory
	pagesPath := "files/pages" + r.URL.Path
	if _, err := staticpkg.Files.Open(pagesPath); err == nil {
		h.serveEmbeddedFile(w, r, pagesPath)
		return
	}

	// 404 - Not Found
	http.NotFound(w, r)
}

// serveEmbeddedFile serves a file from the embedded filesystem
func (h *Handler) serveEmbeddedFile(w http.ResponseWriter, r *http.Request, path string) {
	data, err := staticpkg.Files.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type based on file extension
	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(path, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(path, ".json") {
		contentType = "application/json; charset=utf-8"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(path, ".ico") {
		contentType = "image/x-icon"
	} else if strings.HasSuffix(path, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") {
		contentType = "image/jpeg"
	}

	w.Header().Set("Content-Type", contentType)

	// HTML must always be revalidated: it is what carries the versioned
	// asset URLs (e.g. /js/main.js?v=...). If a stale index.html is served,
	// the browser keeps loading an old cached bundle and the chunked upload
	// path silently disappears. Static assets themselves are long-lived
	// because their URLs are versioned.
	if strings.HasSuffix(path, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
	}

	w.Write(data)
}

// SetupRoutes sets up all HTTP routes
func (h *Handler) SetupRoutes(mux *http.ServeMux) {
	// UI and static file routes
	mux.HandleFunc("/", h.ServeUI)

	// API routes
	mux.HandleFunc("/api/upload", h.UploadHandler)
	mux.HandleFunc("/api/upload/init", h.InitChunkedUploadHandler)
	mux.HandleFunc("/api/upload/chunk", h.UploadChunkHandler)
	mux.HandleFunc("/api/upload/complete", h.CompleteChunkedUploadHandler)
	mux.HandleFunc("/api/upload/abort", h.AbortChunkedUploadHandler)
	mux.HandleFunc("/api/download/", h.DownloadHandler)
	mux.HandleFunc("/api/storage", h.StorageInfoHandler)
	mux.HandleFunc("/api/upload/config", h.UploadConfigHandler)
	mux.HandleFunc("/api/health", h.HealthHandler)
}
