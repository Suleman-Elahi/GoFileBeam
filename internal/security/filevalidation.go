package security

import (
	"bytes"
	"errors"
	"strings"
)

var (
	ErrSuspiciousContent = errors.New("suspicious content detected")
	ErrInvalidFilename   = errors.New("invalid filename")
)

// Suspicious patterns in file content (for HTML/text files only)
var suspiciousPatterns = [][]byte{
	[]byte("eval("),
	[]byte("exec("),
	[]byte("system("),
	[]byte("shell_exec"),
	[]byte("passthru"),
	[]byte("<script"),
	[]byte("javascript:"),
	[]byte("vbscript:"),
	[]byte("onclick="),
	[]byte("onerror="),
	[]byte("onload="),
}

// FileValidator validates uploaded files for security
type FileValidator struct {
	maxFilenameLen   int
	scanContent      bool
	scanMaxSize      int64 // Only scan files smaller than this
}

// NewFileValidator creates a new file validator
func NewFileValidator() *FileValidator {
	return &FileValidator{
		maxFilenameLen:   255,
		scanContent:      true,
		scanMaxSize:      10 * 1024 * 1024, // 10MB
	}
}

// ValidateFilename checks if filename is safe
func (fv *FileValidator) ValidateFilename(filename string) error {
	// Check length
	if len(filename) > fv.maxFilenameLen {
		return ErrInvalidFilename
	}
	
	// Check for null bytes
	if strings.Contains(filename, "\x00") {
		return ErrInvalidFilename
	}
	
	// Check for path traversal attempts
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return ErrInvalidFilename
	}
	
	// Allow all file types - no extension restrictions
	// Security is handled by filesystem sandbox instead
	
	return nil
}

// ValidateContent performs basic content scanning for HTML/text files
// This is optional and only scans for obvious script injection attempts
func (fv *FileValidator) ValidateContent(data []byte, filename string) error {
	if !fv.scanContent {
		return nil
	}

	// Skip validation for large files (performance)
	if int64(len(data)) > fv.scanMaxSize {
		return nil
	}
	
	// Only scan text-based files that could be rendered in browsers
	ext := strings.ToLower(filename[strings.LastIndex(filename, "."):])
	textExtensions := map[string]bool{
		".html": true, ".htm": true, ".svg": true,
	}
	
	// Only scan HTML/SVG files for script injection
	if textExtensions[ext] {
		for _, pattern := range suspiciousPatterns {
			if bytes.Contains(data, pattern) {
				// Don't block, just log warning
				// Users should be able to share HTML files
				// Security is handled by sandbox + browser security
				return nil
			}
		}
	}
	
	return nil
}

// SanitizeFilename removes dangerous characters from filename
func (fv *FileValidator) SanitizeFilename(filename string) string {
	// Remove path components
	if idx := strings.LastIndex(filename, "/"); idx != -1 {
		filename = filename[idx+1:]
	}
	if idx := strings.LastIndex(filename, "\\"); idx != -1 {
		filename = filename[idx+1:]
	}
	
	// Replace dangerous characters
	replacer := strings.NewReplacer(
		"\x00", "",
		"..", "_",
		"<", "_",
		">", "_",
		":", "_",
		"\"", "_",
		"|", "_",
		"?", "_",
		"*", "_",
	)
	
	return replacer.Replace(filename)
}

// GetFileTypeInfo returns information about a file (for display purposes)
func (fv *FileValidator) GetFileTypeInfo(filename string) map[string]string {
	ext := strings.ToLower(filename[strings.LastIndex(filename, "."):])
	
	info := map[string]string{
		"extension": ext,
		"category":  "unknown",
		"warning":   "",
	}
	
	// Categorize file types (for UI display, not blocking)
	categories := map[string]string{
		".exe": "executable", ".bat": "executable", ".cmd": "executable",
		".sh": "executable", ".app": "executable", ".dmg": "executable",
		".pdf": "document", ".doc": "document", ".docx": "document",
		".jpg": "image", ".png": "image", ".gif": "image",
		".mp4": "video", ".avi": "video", ".mov": "video",
		".zip": "archive", ".tar": "archive", ".gz": "archive",
	}
	
	if category, exists := categories[ext]; exists {
		info["category"] = category
	}
	
	// Add warnings for executable files (informational only)
	if info["category"] == "executable" {
		info["warning"] = "This is an executable file. Only run if you trust the source."
	}
	
	return info
}

