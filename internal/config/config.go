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
	
	// Cleanup
	CleanupInterval time.Duration
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
		CleanupInterval:   time.Hour,
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
	
	if cleanupInterval := os.Getenv("GOFILEBEAM_CLEANUP_INTERVAL_MINUTES"); cleanupInterval != "" {
		if val, err := strconv.ParseInt(cleanupInterval, 10, 64); err == nil && val > 0 {
			c.CleanupInterval = time.Duration(val) * time.Minute
		}
	}
}

// GetMaxStorageBytes returns the maximum storage in bytes
func (c *Config) GetMaxStorageBytes() int64 {
	return c.MaxStorageGB * 1024 * 1024 * 1024
}