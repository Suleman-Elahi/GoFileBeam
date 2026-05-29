package security

import (
	"sync"
	"time"
)

// BruteForceProtection tracks failed password attempts
type BruteForceProtection struct {
	attempts map[string]*PasswordAttempts
	mu       sync.RWMutex
}

// PasswordAttempts tracks failed attempts for a file
type PasswordAttempts struct {
	count      int
	firstAttempt time.Time
	lastAttempt  time.Time
	blocked      bool
	blockedUntil time.Time
}

// NewBruteForceProtection creates a new brute force protection instance
func NewBruteForceProtection() *BruteForceProtection {
	bfp := &BruteForceProtection{
		attempts: make(map[string]*PasswordAttempts),
	}
	
	// Start cleanup goroutine
	go bfp.cleanup()
	
	return bfp
}

// RecordFailedAttempt records a failed password attempt
func (bfp *BruteForceProtection) RecordFailedAttempt(fileID, ip string) bool {
	bfp.mu.Lock()
	defer bfp.mu.Unlock()
	
	key := fileID + ":" + ip
	attempts, exists := bfp.attempts[key]
	
	if !exists {
		attempts = &PasswordAttempts{
			count:        1,
			firstAttempt: time.Now(),
			lastAttempt:  time.Now(),
		}
		bfp.attempts[key] = attempts
		return true // Allow
	}
	
	// Check if blocked
	if attempts.blocked {
		if time.Now().Before(attempts.blockedUntil) {
			return false // Still blocked
		}
		// Unblock and reset
		attempts.blocked = false
		attempts.count = 0
	}
	
	// Increment count
	attempts.count++
	attempts.lastAttempt = time.Now()
	
	// Block after 5 failed attempts within 10 minutes
	if attempts.count >= 5 {
		timeSinceFirst := time.Since(attempts.firstAttempt)
		if timeSinceFirst < 10*time.Minute {
			attempts.blocked = true
			attempts.blockedUntil = time.Now().Add(30 * time.Minute)
			return false // Blocked
		} else {
			// Reset if attempts spread over long time
			attempts.count = 1
			attempts.firstAttempt = time.Now()
		}
	}
	
	return true // Allow
}

// ResetAttempts resets attempts for a file/IP (on successful auth)
func (bfp *BruteForceProtection) ResetAttempts(fileID, ip string) {
	bfp.mu.Lock()
	defer bfp.mu.Unlock()
	
	key := fileID + ":" + ip
	delete(bfp.attempts, key)
}

// IsBlocked checks if an IP is blocked for a file
func (bfp *BruteForceProtection) IsBlocked(fileID, ip string) bool {
	bfp.mu.RLock()
	defer bfp.mu.RUnlock()
	
	key := fileID + ":" + ip
	attempts, exists := bfp.attempts[key]
	
	if !exists {
		return false
	}
	
	if attempts.blocked && time.Now().Before(attempts.blockedUntil) {
		return true
	}
	
	return false
}

// cleanup removes old attempt records
func (bfp *BruteForceProtection) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		bfp.mu.Lock()
		now := time.Now()
		for key, attempts := range bfp.attempts {
			// Remove records older than 1 hour (unless blocked)
			if !attempts.blocked && now.Sub(attempts.lastAttempt) > time.Hour {
				delete(bfp.attempts, key)
			}
			// Remove expired blocks
			if attempts.blocked && now.After(attempts.blockedUntil) {
				delete(bfp.attempts, key)
			}
		}
		bfp.mu.Unlock()
	}
}
