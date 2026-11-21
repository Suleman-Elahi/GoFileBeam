package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gofilebeam/internal/config"
	"gofilebeam/internal/handlers"
	"gofilebeam/internal/security"
	"gofilebeam/internal/storage"
)

func main() {
	// Load configuration
	cfg := config.DefaultConfig()
	cfg.LoadFromEnv()

	// Initialize security components
	log.Println("Initializing security components...")
	
	// Create sandbox
	sandbox := security.NewSandbox(cfg.StoragePath)
	if err := sandbox.Initialize(); err != nil {
		log.Fatalf("Failed to initialize sandbox: %v", err)
	}
	log.Println("✓ Sandbox initialized")
	
	// Create rate limiter
	rateLimiter := security.NewRateLimiter(cfg.RateLimitPerMinute)
	log.Printf("✓ Rate limiter initialized (%d req/min)", cfg.RateLimitPerMinute)

	// Chunk uploads get their own bucket: a single large file is a burst
	// of many chunk requests from one IP, and that burst must fit in the
	// bucket or the upload dies with a 429 partway through. Burst covers
	// the maximum supported file's chunk count with headroom for retries;
	// the sustained rate still caps how fast any one IP can push data.
	maxChunks := int((cfg.MaxFileSize + cfg.ChunkSize - 1) / cfg.ChunkSize)
	chunkBurst := maxChunks*2 + 30
	chunkLimiter := security.NewRateLimiterWithBurst(cfg.ChunkRateLimitPerMinute, chunkBurst, 0)
	log.Printf("✓ Chunk upload rate limiter initialized (%d req/min, burst %d, no IP block)",
		cfg.ChunkRateLimitPerMinute, chunkBurst)
	
	// Create brute force protection
	bruteForce := security.NewBruteForceProtection()
	log.Println("✓ Brute force protection initialized")
	
	// Create file validator
	fileValidator := security.NewFileValidator()
	log.Println("✓ File validator initialized")

	// Create storage
	storageInstance, err := storage.NewStorage(cfg)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	log.Println("✓ Storage initialized")

	// Create handlers with security components
	handler := handlers.NewHandlerWithSecurity(storageInstance, cfg, sandbox, rateLimiter, bruteForce, fileValidator)
	log.Println("✓ Handlers initialized")

	// Create HTTP server
	mux := http.NewServeMux()
	handler.SetupRoutes(mux)

	// Add middleware with security
	handlerWithMiddleware := addSecurityMiddleware(mux, cfg, rateLimiter, chunkLimiter)

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:           handlerWithMiddleware,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Print security status
	printSecurityStatus(sandbox, cfg)

	// Start server in goroutine
	go func() {
		log.Printf("Starting GoFileBeam server on %s:%s", cfg.Host, cfg.Port)
		log.Printf("Web interface: http://%s:%s", cfg.Host, cfg.Port)
		if cfg.EnableHTTPS && cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
			log.Println("✓ HTTPS enabled")
			if err := server.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start HTTPS server: %v", err)
			}
		} else {
			log.Println("⚠ Running in HTTP mode (use HTTPS in production)")
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start HTTP server: %v", err)
			}
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown server
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// addSecurityMiddleware adds security middleware to the handler.
//
// Two limiters are used, because a chunked upload of a large file is many
// requests from one IP in quick succession and must not be throttled by
// the same budget that protects the rest of the API:
//
//   - generalLimiter: page loads, init/complete/abort, downloads, etc.
//     Keeps the original per-IP budget (default 60/min) so casual abuse
//     of the API surface stays limited.
//   - chunkLimiter: only POST /api/upload/chunk. Sized so the maximum
//     supported file (500 MB / 8 MB = 63 chunks) plus retries fits in the
//     burst, and it never escalates to an IP block — hitting it just means
//     "slow down briefly", not "locked out for 15 minutes mid-upload".
func addSecurityMiddleware(handler http.Handler, cfg *config.Config, rateLimiter, chunkLimiter *security.RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get client IP
		clientIP := security.GetClientIP(r)

		// Rate limiting: chunk data goes to its own bucket.
		limiter := rateLimiter
		if r.Method == http.MethodPost && r.URL.Path == "/api/upload/chunk" {
			limiter = chunkLimiter
		}
		if !limiter.Allow(clientIP) {
			log.Printf("Rate limit exceeded for IP: %s on %s", clientIP, r.URL.Path)
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}
		
		// Add CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Add security headers.
		//
		// Ownership: these headers are set here, in the application, and
		// are the single source of truth for this deployment. If you are
		// running GoFileBeam behind a reverse proxy (nginx, Caddy, etc.) or
		// Cloudflare, do NOT also set X-Content-Type-Options,
		// X-Frame-Options, X-XSS-Protection, or Content-Security-Policy in
		// the proxy config. Duplicate headers from two layers were
		// observed in production (e.g. "X-Frame-Options: DENY" from the
		// app and "X-Frame-Options: SAMEORIGIN" from nginx sent together),
		// which is both wasteful and risks sending conflicting policies to
		// the browser. See docs/nginx-recommended.conf for the reverse
		// proxy config that intentionally omits these headers.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.tailwindcss.com https://fonts.googleapis.com https://fonts.gstatic.com; img-src 'self' data:; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com;")
		
		// Add HSTS header if HTTPS is enabled
		if cfg.EnableHTTPS {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Log request
		log.Printf("%s %s %s", clientIP, r.Method, r.URL.Path)

		// Call the actual handler
		handler.ServeHTTP(w, r)
	})
}

// printSecurityStatus prints the security configuration status
func printSecurityStatus(sandbox *security.Sandbox, cfg *config.Config) {
	log.Println("")
	log.Println("═══════════════════════════════════════")
	log.Println("  GoFileBeam Security Status")
	log.Println("═══════════════════════════════════════")
	
	// Sandbox info
	info := sandbox.GetSystemInfo()
	log.Printf("Storage Path: %s", info["storage_path"])
	log.Printf("File Permissions: 0444 (read-only, no execute)")
	log.Printf("Platform: %s/%s", info["os"], info["arch"])
	
	// Security features
	log.Println("")
	log.Println("Security Features:")
	log.Println("  ✓ Rate limiting enabled")
	log.Println("  ✓ Brute force protection enabled")
	log.Println("  ✓ File validation enabled")
	log.Println("  ✓ Sandbox protection enabled")
	log.Println("  ✓ Path traversal prevention")
	log.Println("  ✓ Security headers enabled")
	log.Println("  ✓ Automatic file expiration")
	log.Println("  ✓ Storage quota management")
	
	if cfg.EnableHTTPS {
		log.Println("  ✓ HTTPS/TLS enabled")
	} else {
		log.Println("  ⚠ HTTPS disabled (enable for production)")
	}
	
	// Recommendations
	log.Println("")
	log.Println("Recommendations:")
	recommendations := sandbox.GetRecommendations()
	for _, rec := range recommendations {
		log.Println("  " + rec)
	}
	
	log.Println("═══════════════════════════════════════")
	log.Println("")
}