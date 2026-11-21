package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	// Server configuration
	Port         string
	Host         string
	
	// Storage configuration
	StoragePath  string
	MaxStorageGB int64
	MaxFileSize  int64
	
	// Security
	EnableHTTPS  bool
	TLSCertPath  string
	TLSKeyPath   string
	
	// Rate limiting
	RateLimitPerMinute int
	// ChunkRateLimitPerMinute is the sustained per-minute budget for
	// POST /api/upload/chunk specifically. It is separate from
	// RateLimitPerMinute because chunked uploads legitimately issue many
	// requests in a short window; the burst size is derived in main() from
	// the maximum file size so a full-size upload always fits.
	ChunkRateLimitPerMinute int
	
	// Cleanup
	CleanupInterval time.Duration

	// Chunked upload configuration.
	//
	// GoFileBeam is commonly deployed behind Cloudflare, which enforces a
	// hard per-request body size limit regardless of app configuration
	// (100 MB on Free/Pro, 200 MB on Business, 500 MB on Enterprise, and
	// Enterprise-only beyond that). ChunkSize controls how large each
	// individual chunk request is allowed to be; keep it comfortably below
	// your actual Cloudflare (or other proxy) limit so no single request in
	// the chunked upload protocol is ever rejected at the edge. Any file at
	// or above ChunkUploadThreshold should use the chunked protocol
	// (/api/upload/init, /chunk, /complete) instead of the single-request
	// /api/upload endpoint.
	ChunkSize            int64
	ChunkUploadThreshold int64
	// ChunkSessionTimeout is how long an upload session may sit idle
	// (no chunk received) before it is considered abandoned and its
	// staged data is deleted.
	ChunkSessionTimeout time.Duration
	// ChunkConcurrency is how many chunk requests the client may have in
	// flight at once for a single upload. The server writes each chunk to
	// its own offset under a per-session mutex, so parallel arrival is
	// safe; concurrency is what keeps the upload pipe full instead of
	// paying a full round trip between every chunk. Capped at 6 by the
	// frontend to stay under the browser's per-host connection limit.
	ChunkConcurrency int
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Port:              "8080",
		Host:              "0.0.0.0",
		StoragePath:       "./uploads",
		MaxStorageGB:      1, // 1 GB default
		MaxFileSize:       100 * 1024 * 1024, // 100 MB per file
		EnableHTTPS:       false,
		RateLimitPerMinute: 60,
		// Sized so the maximum file's chunks (63 at defaults) plus
		// retries fit comfortably in one minute of budget.
		ChunkRateLimitPerMinute: 300,
		CleanupInterval:   time.Hour,

		// Defaults chosen to stay safely under Cloudflare's Free/Pro plan
		// limit (100 MB per request). 8 MB chunks keep individual chunk
		// requests small and fast to retry on failure; files >= 80 MB use
		// the chunked protocol so a single request body never approaches
		// the 100 MB edge limit even with multipart overhead.
		ChunkSize:            8 * 1024 * 1024,
		ChunkUploadThreshold: 80 * 1024 * 1024,
		ChunkSessionTimeout:  30 * time.Minute,
		ChunkConcurrency:     6,
	}
}

// LoadFromEnv loads configuration from environment variables and .env file
func (c *Config) LoadFromEnv() {
	// Try to load .env file if it exists
	_ = godotenv.Load()
	
	if port := os.Getenv("GOFILEBEAM_PORT"); port != "" {
		c.Port = port
	}
	
	if host := os.Getenv("GOFILEBEAM_HOST"); host != "" {
		c.Host = host
	}
	
	if storagePath := os.Getenv("GOFILEBEAM_STORAGE_PATH"); storagePath != "" {
		c.StoragePath = storagePath
	}
	
	if maxStorageGB := os.Getenv("GOFILEBEAM_MAX_STORAGE_GB"); maxStorageGB != "" {
		if val, err := strconv.ParseInt(maxStorageGB, 10, 64); err == nil && val > 0 {
			c.MaxStorageGB = val
		}
	}
	
	if maxFileSize := os.Getenv("GOFILEBEAM_MAX_FILE_SIZE_MB"); maxFileSize != "" {
		if val, err := strconv.ParseInt(maxFileSize, 10, 64); err == nil && val > 0 {
			c.MaxFileSize = val * 1024 * 1024
		}
	}
	
	if enableHTTPS := os.Getenv("GOFILEBEAM_ENABLE_HTTPS"); enableHTTPS != "" {
		c.EnableHTTPS = enableHTTPS == "true" || enableHTTPS == "1"
	}
	
	if certPath := os.Getenv("GOFILEBEAM_TLS_CERT_PATH"); certPath != "" {
		c.TLSCertPath = certPath
	}
	
	if keyPath := os.Getenv("GOFILEBEAM_TLS_KEY_PATH"); keyPath != "" {
		c.TLSKeyPath = keyPath
	}
	
	if rateLimit := os.Getenv("GOFILEBEAM_RATE_LIMIT_PER_MINUTE"); rateLimit != "" {
		if val, err := strconv.Atoi(rateLimit); err == nil && val > 0 {
			c.RateLimitPerMinute = val
		}
	}

	if chunkRateLimit := os.Getenv("GOFILEBEAM_CHUNK_RATE_LIMIT_PER_MINUTE"); chunkRateLimit != "" {
		if val, err := strconv.Atoi(chunkRateLimit); err == nil && val > 0 {
			c.ChunkRateLimitPerMinute = val
		}
	}
	
	if cleanupInterval := os.Getenv("GOFILEBEAM_CLEANUP_INTERVAL_MINUTES"); cleanupInterval != "" {
		if val, err := strconv.ParseInt(cleanupInterval, 10, 64); err == nil && val > 0 {
			c.CleanupInterval = time.Duration(val) * time.Minute
		}
	}

	if chunkSizeMB := os.Getenv("GOFILEBEAM_CHUNK_SIZE_MB"); chunkSizeMB != "" {
		if val, err := strconv.ParseInt(chunkSizeMB, 10, 64); err == nil && val > 0 {
			c.ChunkSize = val * 1024 * 1024
		}
	}

	if chunkThresholdMB := os.Getenv("GOFILEBEAM_CHUNK_UPLOAD_THRESHOLD_MB"); chunkThresholdMB != "" {
		if val, err := strconv.ParseInt(chunkThresholdMB, 10, 64); err == nil && val > 0 {
			c.ChunkUploadThreshold = val * 1024 * 1024
		}
	}

	if chunkSessionTimeoutMin := os.Getenv("GOFILEBEAM_CHUNK_SESSION_TIMEOUT_MINUTES"); chunkSessionTimeoutMin != "" {
		if val, err := strconv.ParseInt(chunkSessionTimeoutMin, 10, 64); err == nil && val > 0 {
			c.ChunkSessionTimeout = time.Duration(val) * time.Minute
		}
	}

	if chunkConcurrency := os.Getenv("GOFILEBEAM_CHUNK_CONCURRENCY"); chunkConcurrency != "" {
		if val, err := strconv.Atoi(chunkConcurrency); err == nil && val > 0 {
			// Hard ceiling: browsers cap ~6 connections per host, so more
			// than that just queues behind the browser rather than helping.
			if val > 6 {
				val = 6
			}
			c.ChunkConcurrency = val
		}
	}
}

// GetMaxStorageBytes returns the maximum storage in bytes
func (c *Config) GetMaxStorageBytes() int64 {
	return c.MaxStorageGB * 1024 * 1024 * 1024
}