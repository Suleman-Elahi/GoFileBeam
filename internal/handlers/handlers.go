package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
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
}

// NewHandler creates a new handler instance (legacy, without security)
func NewHandler(storage *storage.Storage, config *config.Config) *Handler {
	return &Handler{
		storage: storage,
		config:  config,
	}
}

// NewHandlerWithSecurity creates a new handler instance with security components
func NewHandlerWithSecurity(storage *storage.Storage, config *config.Config, sandbox *security.Sandbox, rateLimiter *security.RateLimiter, bruteForce *security.BruteForceProtection, fileValidator *security.FileValidator) *Handler {
	return &Handler{
		storage:       storage,
		config:        config,
		sandbox:       sandbox,
		rateLimiter:   rateLimiter,
		bruteForce:    bruteForce,
		fileValidator: fileValidator,
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

// UploadHandler handles file uploads
func (h *Handler) UploadHandler(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(h.config.MaxFileSize); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get expiration option and password
	expirationOption := r.FormValue("expiration_option")
	password := r.FormValue("password")

	// Validate expiration option
	if expirationOption == "" {
		expirationOption = "1 Download or 1 Day"
	}

	// Collect all uploaded files
	var fileHeaders []*multipart.FileHeader
	
	// Check if we have files in "files" field
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if headers, ok := r.MultipartForm.File["files"]; ok && len(headers) > 0 {
			fileHeaders = headers
		}
	}

	// If no files found with "files", try "file"
	if len(fileHeaders) == 0 {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided: "+err.Error(), http.StatusBadRequest)
			return
		}
		file.Close()
		fileHeaders = []*multipart.FileHeader{header}
	}

	// Validate all filenames if validator is available
	if h.fileValidator != nil {
		for _, header := range fileHeaders {
			if err := h.fileValidator.ValidateFilename(header.Filename); err != nil {
				http.Error(w, "Invalid filename: "+header.Filename, http.StatusBadRequest)
				return
			}
		}
	}

	// Handle single file upload
	if len(fileHeaders) == 1 {
		fileHeader := fileHeaders[0]
		
		// Open the file
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, "Failed to open file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		// Read file data
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Store file (encryption happens in storage layer if password provided)
		metadata, err := h.storage.StoreFile(fileHeader.Filename, data, expirationOption, password)
		if err != nil {
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
			return
		}

		// Apply sandbox security to the stored file
		if h.sandbox != nil {
			filePath := filepath.Join(h.config.StoragePath, metadata.ID)
			if err := h.sandbox.SecureFile(filePath); err != nil {
				log.Printf("Warning: Failed to apply sandbox security to %s: %v", metadata.ID, err)
				// Don't fail the upload, just log the warning
			} else {
				log.Printf("✓ Sandbox security applied to file: %s", metadata.ID)
			}
		}

		// Build download URL
		protocol := "http"
		if h.config.EnableHTTPS {
			protocol = "https"
		}
		
		host := r.Host
		if host == "" {
			host = fmt.Sprintf("%s:%s", h.config.Host, h.config.Port)
		}
		
		url := fmt.Sprintf("%s://%s/api/download/%s", protocol, host, metadata.ID)

		// Prepare response
		response := UploadResponse{
			ID:           metadata.ID,
			URL:          url,
			ExpiresAt:    metadata.ExpiresAt,
			MaxDownloads: metadata.MaxDownloads,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Handle multiple files - create ZIP
	// Create a buffer to write ZIP to
	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)

	// Add each file to the ZIP
	for _, fileHeader := range fileHeaders {
		// Open the file
		file, err := fileHeader.Open()
		if err != nil {
			continue
		}

		// Create a file in the ZIP
		zipFile, err := zipWriter.Create(fileHeader.Filename)
		if err != nil {
			file.Close()
			continue
		}

		// Copy file content to ZIP
		_, err = io.Copy(zipFile, file)
		file.Close()
		if err != nil {
			continue
		}
	}

	// Close the ZIP writer
	if err := zipWriter.Close(); err != nil {
		http.Error(w, "Failed to create ZIP: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get ZIP data
	zipData := zipBuffer.Bytes()

	// Generate ZIP filename with timestamp
	zipFilename := fmt.Sprintf("files_%s.zip", time.Now().Format("20060102_150405"))

	// Store the ZIP file (encryption happens here if password provided)
	metadata, err := h.storage.StoreFile(zipFilename, zipData, expirationOption, password)
	if err != nil {
		switch err {
		case storage.ErrFileTooLarge:
			http.Error(w, "ZIP file too large", http.StatusBadRequest)
		case storage.ErrStorageFull:
			http.Error(w, "Storage quota exceeded", http.StatusInsufficientStorage)
		case storage.ErrInvalidExpiration:
			http.Error(w, "Invalid expiration option", http.StatusBadRequest)
		default:
			http.Error(w, "Failed to store ZIP: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Apply sandbox security to the stored ZIP file
	if h.sandbox != nil {
		filePath := filepath.Join(h.config.StoragePath, metadata.ID)
		if err := h.sandbox.SecureFile(filePath); err != nil {
			log.Printf("Warning: Failed to apply sandbox security to %s: %v", metadata.ID, err)
			// Don't fail the upload, just log the warning
		} else {
			log.Printf("✓ Sandbox security applied to ZIP file: %s", metadata.ID)
		}
	}

	// Build download URL
	protocol := "http"
	if h.config.EnableHTTPS {
		protocol = "https"
	}
	
	host := r.Host
	if host == "" {
		host = fmt.Sprintf("%s:%s", h.config.Host, h.config.Port)
	}
	
	url := fmt.Sprintf("%s://%s/api/download/%s", protocol, host, metadata.ID)

	// Prepare response
	response := UploadResponse{
		ID:           metadata.ID,
		URL:          url,
		ExpiresAt:    metadata.ExpiresAt,
		MaxDownloads: metadata.MaxDownloads,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

	// Get file
	data, metadata, err := h.storage.GetFile(id, password)
	if err != nil {
		switch err {
		case storage.ErrFileNotFound:
			http.Error(w, "File not found", http.StatusNotFound)
		case storage.ErrFileExpired:
			http.Error(w, "File has expired", http.StatusGone)
		case storage.ErrInvalidPassword:
			// Record failed password attempt
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

	// Reset failed attempts on successful download
	if h.bruteForce != nil {
		h.bruteForce.ResetAttempts(id, clientIP)
	}

	log.Printf("✓ File %s downloaded by %s", id, clientIP)

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", metadata.OriginalName))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write file data
	w.Write(data)
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
	w.Write(data)
}

// SetupRoutes sets up all HTTP routes
func (h *Handler) SetupRoutes(mux *http.ServeMux) {
	// UI and static file routes
	mux.HandleFunc("/", h.ServeUI)

	// API routes
	mux.HandleFunc("/api/upload", h.UploadHandler)
	mux.HandleFunc("/api/download/", h.DownloadHandler)
	mux.HandleFunc("/api/storage", h.StorageInfoHandler)
	mux.HandleFunc("/api/health", h.HealthHandler)
}
