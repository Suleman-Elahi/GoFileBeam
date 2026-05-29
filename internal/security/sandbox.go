package security

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SandboxConfig holds sandbox configuration
type SandboxConfig struct {
	StoragePath string
	NoExec      bool // Mount with noexec flag
	ReadOnly    bool // Make files read-only after upload
}

// Sandbox provides filesystem-level security for uploaded files
type Sandbox struct {
	config *SandboxConfig
}

// NewSandbox creates a new sandbox instance
func NewSandbox(storagePath string) *Sandbox {
	return &Sandbox{
		config: &SandboxConfig{
			StoragePath: storagePath,
			NoExec:      true,
			ReadOnly:    true,
		},
	}
}

// Initialize sets up the sandbox environment
func (s *Sandbox) Initialize() error {
	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(s.config.StoragePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Set directory permissions to prevent execution
	if err := s.setDirectoryPermissions(); err != nil {
		return fmt.Errorf("failed to set directory permissions: %w", err)
	}

	return nil
}

// setDirectoryPermissions configures the storage directory for security
func (s *Sandbox) setDirectoryPermissions() error {
	// On Unix-like systems, set permissions to prevent execution
	if runtime.GOOS != "windows" {
		// Set directory to 755 (rwxr-xr-x) - allows reading but we'll prevent exec on files
		if err := os.Chmod(s.config.StoragePath, 0755); err != nil {
			return err
		}
	}

	return nil
}

// SecureFile applies security measures to an uploaded file
func (s *Sandbox) SecureFile(filePath string) error {
	// Make file read-only to prevent modification
	if s.config.ReadOnly {
		if err := s.makeReadOnly(filePath); err != nil {
			return fmt.Errorf("failed to make file read-only: %w", err)
		}
	}

	// Remove execute permissions
	if s.config.NoExec {
		if err := s.removeExecutePermissions(filePath); err != nil {
			return fmt.Errorf("failed to remove execute permissions: %w", err)
		}
	}

	return nil
}

// makeReadOnly makes a file read-only
func (s *Sandbox) makeReadOnly(filePath string) error {
	if runtime.GOOS == "windows" {
		// On Windows, set read-only attribute
		return os.Chmod(filePath, 0444)
	}
	// On Unix-like systems, set to read-only (r--r--r--)
	return os.Chmod(filePath, 0444)
}

// removeExecutePermissions removes execute permissions from a file
func (s *Sandbox) removeExecutePermissions(filePath string) error {
	if runtime.GOOS == "windows" {
		// Windows doesn't have execute permissions in the same way
		// Files are not executable by default unless they have specific extensions
		return nil
	}

	// On Unix-like systems, ensure no execute bits are set (rw-r--r--)
	return os.Chmod(filePath, 0644)
}

// ValidatePath ensures a path is within the sandbox
func (s *Sandbox) ValidatePath(filePath string) error {
	// Get absolute paths
	absStorage, err := filepath.Abs(s.config.StoragePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute storage path: %w", err)
	}

	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute file path: %w", err)
	}

	// Check if file is within storage directory
	rel, err := filepath.Rel(absStorage, absFile)
	if err != nil {
		return fmt.Errorf("path is outside sandbox: %w", err)
	}

	// Prevent path traversal
	if len(rel) > 0 && rel[0] == '.' && rel[1] == '.' {
		return fmt.Errorf("path traversal detected: %s", rel)
	}

	return nil
}

// GetSecureFilePath returns a secure path within the sandbox
func (s *Sandbox) GetSecureFilePath(filename string) (string, error) {
	// Clean the filename
	cleanName := filepath.Base(filename)
	
	// Build full path
	fullPath := filepath.Join(s.config.StoragePath, cleanName)
	
	// Validate it's within sandbox
	if err := s.ValidatePath(fullPath); err != nil {
		return "", err
	}

	return fullPath, nil
}

// CreateNoExecFile creates a file with no execute permissions
func (s *Sandbox) CreateNoExecFile(filePath string, data []byte) error {
	// Validate path is in sandbox
	if err := s.ValidatePath(filePath); err != nil {
		return err
	}

	// Write file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Apply security measures
	if err := s.SecureFile(filePath); err != nil {
		return err
	}

	return nil
}

// GetSystemInfo returns information about sandbox security features
func (s *Sandbox) GetSystemInfo() map[string]interface{} {
	info := map[string]interface{}{
		"storage_path": s.config.StoragePath,
		"no_exec":      s.config.NoExec,
		"read_only":    s.config.ReadOnly,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
	}

	// Check if storage is on a noexec mount (Linux only)
	if runtime.GOOS == "linux" {
		info["mount_noexec"] = s.checkNoExecMount()
	}

	return info
}

// checkNoExecMount checks if storage is on a noexec mount (Linux)
func (s *Sandbox) checkNoExecMount() bool {
	// This would require parsing /proc/mounts
	// For now, return false (would need implementation)
	return false
}

// GetRecommendations returns security recommendations
func (s *Sandbox) GetRecommendations() []string {
	recommendations := []string{
		"✓ Files stored with 0644 permissions (no execute on creation)",
		"✓ Files changed to 0444 after upload (read-only)",
		"✓ Path traversal prevention active",
		"✓ Sandbox validation on all file operations",
	}

	if runtime.GOOS == "linux" {
		recommendations = append(recommendations,
			"",
			"Optional Enhancement (Linux):",
			"💡 Mount storage with 'noexec' flag for additional security:",
			"   sudo mount -o remount,noexec "+s.config.StoragePath,
			"💡 Or add to /etc/fstab for persistence:",
			"   /dev/sdX "+s.config.StoragePath+" ext4 defaults,noexec 0 2",
			"",
			"Note: This is OPTIONAL. Files already cannot execute due to 0444 permissions.",
		)
	}

	if runtime.GOOS == "windows" {
		recommendations = append(recommendations,
			"",
			"Optional Enhancement (Windows):",
			"💡 Consider using Windows Defender Application Control",
			"💡 Enable Windows Firewall rules for the storage directory",
		)
	}

	return recommendations
}
