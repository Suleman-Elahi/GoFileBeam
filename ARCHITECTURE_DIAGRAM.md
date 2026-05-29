# GoFileBeam - Complete Architecture Diagram

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                          CLIENT (Browser)                            │
│                                                                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │
│  │  Upload UI   │  │ Download UI  │  │ Static Pages │              │
│  │ (index.html) │  │(download.html)│  │(help, etc.)  │              │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘              │
│         │                  │                                          │
│         │  JavaScript (main.js)                                      │
│         │                  │                                          │
└─────────┼──────────────────┼──────────────────────────────────────────┘
          │                  │
          │ HTTP/HTTPS       │
          ▼                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      GOFILEBEAM SERVER                               │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    SECURITY MIDDLEWARE                       │   │
│  │                                                               │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │   │
│  │  │ Rate Limiter │  │Security Headers│ │  CORS/CSP   │      │   │
│  │  │ (per IP)     │  │ (XSS, Frame)  │  │  Headers    │      │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘      │   │
│  │                                                               │   │
│  │  • Token bucket algorithm                                    │   │
│  │  • Auto-blocking after violations                            │   │
│  │  • 60 requests/minute default                                │   │
│  └───────────────────────────┬─────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                      HTTP HANDLERS                           │   │
│  │                                                               │   │
│  │  ┌──────────────────┐  ┌──────────────────┐                │   │
│  │  │ Upload Handler   │  │ Download Handler │                │   │
│  │  │                  │  │                  │                │   │
│  │  │ • Validate files │  │ • Check blocked  │                │   │
│  │  │ • Create ZIP     │  │ • Verify password│                │   │
│  │  │ • Encrypt data   │  │ • Track attempts │                │   │
│  │  │ • Apply sandbox  │  │ • Decrypt data   │                │   │
│  │  └────────┬─────────┘  └────────┬─────────┘                │   │
│  │           │                     │                            │   │
│  └───────────┼─────────────────────┼────────────────────────────┘   │
│              │                     │                                 │
│              ▼                     ▼                                 │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                   SECURITY COMPONENTS                        │   │
│  │                                                               │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │   │
│  │  │File Validator│  │Brute Force   │  │   Sandbox    │      │   │
│  │  │              │  │  Protection  │  │              │      │   │
│  │  │• No blocking │  │• Track fails │  │• No execute  │      │   │
│  │  │• Sanitize    │  │• 5 attempts  │  │• Read-only   │      │   │
│  │  │• Validate    │  │• 30min block │  │• Path check  │      │   │
│  │  └──────────────┘  └──────────────┘  └──────┬───────┘      │   │
│  │                                              │               │   │
│  └──────────────────────────────────────────────┼───────────────┘   │
│                                                 │                   │
│                                                 ▼                   │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    STORAGE LAYER                             │   │
│  │                                                               │   │
│  │  ┌──────────────────┐  ┌──────────────────┐                │   │
│  │  │  File Storage    │  │   Encryption     │                │   │
│  │  │                  │  │                  │                │   │
│  │  │ • Store files    │  │ • AES-256-GCM    │                │   │
│  │  │ • Track metadata │  │ • Password-based │                │   │
│  │  │ • Expiration     │  │ • Secure keys    │                │   │
│  │  │ • Quota mgmt     │  │ • Decrypt on DL  │                │   │
│  │  └────────┬─────────┘  └──────────────────┘                │   │
│  │           │                                                  │   │
│  └───────────┼──────────────────────────────────────────────────┘   │
│              │                                                       │
└──────────────┼───────────────────────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        FILESYSTEM                                    │
│                                                                       │
│  uploads/                                                            │
│  ├── abc123def456... (encrypted file, 0444 permissions)             │
│  ├── xyz789ghi012... (encrypted file, 0444 permissions)             │
│  └── metadata.json  (file metadata, expiration, downloads)          │
│                                                                       │
│  Permissions: -r--r--r-- (read-only, no execute)                    │
│  Optional: noexec mount flag (Linux)                                │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Request Flow Diagrams

### Upload Flow

```
User uploads file
       │
       ▼
┌──────────────────┐
│ Security         │
│ Middleware       │ ──► Rate limit check
│                  │ ──► Security headers
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Upload Handler   │
└────────┬─────────┘
         │
         ├──► File Validator ──► Validate filename
         │                   ──► Sanitize path
         │
         ├──► Storage Layer ──► Store file
         │                  ──► Encrypt (if password)
         │                  ──► Generate ID
         │                  ──► Save metadata
         │
         └──► Sandbox ──────────► Set permissions (0444)
                                ► Remove execute bits
                                ► Validate path
         │
         ▼
Return download URL
```

### Download Flow

```
User requests file
       │
       ▼
┌──────────────────┐
│ Security         │
│ Middleware       │ ──► Rate limit check
│                  │ ──► Security headers
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Download Handler │
└────────┬─────────┘
         │
         ├──► Brute Force ────► Check if blocked
         │    Protection      ► Track attempts
         │
         ├──► Storage Layer ──► Load metadata
         │                   ──► Check expiration
         │                   ──► Verify password
         │                   ──► Decrypt file
         │                   ──► Update download count
         │
         └──► Brute Force ────► Reset on success
                               ► Block on 5 failures
         │
         ▼
Return file data
```

---

## Security Layers

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Security                                    │
│ • Rate limiting (60 req/min)                                 │
│ • IP-based blocking                                          │
│ • HTTPS/TLS (optional)                                       │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Application Security                                │
│ • Security headers (XSS, Frame, CSP)                         │
│ • CORS configuration                                         │
│ • Input validation                                           │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Authentication Security                             │
│ • Brute force protection                                     │
│ • Password encryption (AES-256-GCM)                          │
│ • Attempt tracking                                           │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 4: File Security                                       │
│ • Filename validation                                        │
│ • Path traversal prevention                                  │
│ • No file type restrictions                                  │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 5: Filesystem Security (SANDBOX)                       │
│ • No execute permissions (0644 → 0444)                       │
│ • Read-only after upload                                     │
│ • Path validation                                            │
│ • Optional noexec mount                                      │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 6: Data Security                                       │
│ • Automatic expiration                                       │
│ • Download count limits                                      │
│ • Storage quota enforcement                                  │
│ • Automatic cleanup                                          │
└─────────────────────────────────────────────────────────────┘
```

---

## Component Integration Map

```
main.go
  │
  ├──► config.LoadFromEnv() ──────────► .env file
  │
  ├──► security.NewSandbox() ─────────► sandbox.go
  │
  ├──► security.NewRateLimiter() ─────► ratelimit.go
  │
  ├──► security.NewBruteForce() ──────► bruteforce.go
  │
  ├──► security.NewFileValidator() ───► filevalidation.go
  │
  ├──► storage.NewStorage() ──────────► storage.go
  │
  └──► handlers.NewHandlerWithSecurity()
         │
         └──► handlers.go
                │
                ├──► UploadHandler
                │      │
                │      ├──► fileValidator.ValidateFilename()
                │      ├──► storage.StoreFile()
                │      └──► sandbox.SecureFile()
                │
                └──► DownloadHandler
                       │
                       ├──► bruteForce.IsBlocked()
                       ├──► storage.GetFile()
                       ├──► bruteForce.RecordFailedAttempt()
                       └──► bruteForce.ResetAttempts()
```

---

## File Type Handling

```
User uploads ANY file type
         │
         ▼
┌────────────────────┐
│ No Restrictions    │  ✅ .exe allowed
│                    │  ✅ .sh allowed
│                    │  ✅ .bat allowed
│                    │  ✅ .js allowed
│                    │  ✅ ALL types allowed
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│ Filename Sanitized │  • Remove path components
│                    │  • Remove dangerous chars
│                    │  • Validate length
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│ File Stored        │  • Encrypted if password
│                    │  • Unique ID generated
│                    │  • Metadata saved
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│ Sandbox Applied    │  • chmod 0444 (r--r--r--)
│                    │  • No execute bits
│                    │  • Read-only
│                    │  • Cannot be modified
│                    │  • Cannot be executed
└────────────────────┘
          │
          ▼
    File is safe!
```

---

## Security Decision Tree

```
                    File Upload Request
                           │
                           ▼
                  Rate limit exceeded?
                    ├─ YES ──► 429 Too Many Requests
                    │
                    └─ NO
                       │
                       ▼
                  Valid filename?
                    ├─ NO ──► 400 Bad Request
                    │
                    └─ YES
                       │
                       ▼
                  File too large?
                    ├─ YES ──► 400 Bad Request
                    │
                    └─ NO
                       │
                       ▼
                  Storage full?
                    ├─ YES ──► 507 Insufficient Storage
                    │
                    └─ NO
                       │
                       ▼
                  Store & Encrypt
                       │
                       ▼
                  Apply Sandbox
                       │
                       ▼
                  Return URL ──► 200 OK


                   Download Request
                           │
                           ▼
                  Rate limit exceeded?
                    ├─ YES ──► 429 Too Many Requests
                    │
                    └─ NO
                       │
                       ▼
                  IP blocked (brute force)?
                    ├─ YES ──► 429 Too Many Requests
                    │
                    └─ NO
                       │
                       ▼
                  File exists?
                    ├─ NO ──► 404 Not Found
                    │
                    └─ YES
                       │
                       ▼
                  File expired?
                    ├─ YES ──► 410 Gone
                    │
                    └─ NO
                       │
                       ▼
                  Password required?
                    ├─ NO ──► Return file ──► 200 OK
                    │
                    └─ YES
                       │
                       ▼
                  Password correct?
                    ├─ NO ──► Record attempt
                    │         │
                    │         └─► 5 attempts? ──► Block IP
                    │                           ──► 401 Unauthorized
                    │
                    └─ YES
                       │
                       ▼
                  Reset attempts
                       │
                       ▼
                  Decrypt & Return ──► 200 OK
```

---

## All Components Are Integrated! ✅

Every security component is:
- ✅ Created
- ✅ Initialized in main.go
- ✅ Passed to handlers
- ✅ Used in request processing
- ✅ Tested and working

**The system is complete and production-ready!**
