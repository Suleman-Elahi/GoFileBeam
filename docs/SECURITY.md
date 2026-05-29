# GoFileBeam Security Features

## 🛡️ Multi-Layered Security Architecture

GoFileBeam implements comprehensive security measures to prevent abuse, spam, DDoS attacks, and phishing attempts.

---

## 🚦 1. Rate Limiting

### **Purpose**: Prevent DDoS and abuse

### **Implementation**:
- **Token Bucket Algorithm** per IP address
- Configurable requests per minute (default: 60)
- Gradual token refill over time
- Automatic IP blocking after repeated violations

### **Features**:
```
✅ Per-IP rate limiting
✅ Automatic token refill
✅ Progressive blocking (5 violations = 15 min block)
✅ Automatic cleanup of old records
✅ Handles proxy headers (X-Forwarded-For, X-Real-IP)
```

### **Configuration**:
```bash
GOFILEBEAM_RATE_LIMIT_PER_MINUTE=60
```

### **Behavior**:
- **Normal usage**: Tokens refill gradually
- **Burst traffic**: Allowed up to rate limit
- **Sustained abuse**: IP blocked for 15 minutes
- **Repeated abuse**: Longer blocks

---

## 🔐 2. Brute Force Protection

### **Purpose**: Prevent password guessing attacks

### **Implementation**:
- Track failed password attempts per file + IP
- Block after 5 failed attempts within 10 minutes
- 30-minute block duration
- Automatic reset on successful authentication

### **Features**:
```
✅ Per-file + IP tracking
✅ Time-window based detection
✅ Automatic blocking
✅ Automatic unblocking after timeout
✅ Reset on successful auth
```

### **Behavior**:
```
Attempt 1-4: Allowed
Attempt 5 (within 10 min): IP blocked for 30 minutes
Successful auth: Counter reset
After 10 min: Counter reset if < 5 attempts
```

### **Protection Against**:
- Dictionary attacks
- Brute force attacks
- Credential stuffing
- Automated password guessing

---

## 📁 3. File Validation & Sandbox Security

### **Purpose**: Prevent malicious file execution while allowing all file types

### **Our Approach**: **No File Type Restrictions**

GoFileBeam allows users to share **any file type** (.exe, .sh, .bat, scripts, etc.) because:
- Blocking file types is ineffective (easily bypassed by renaming)
- Limits legitimate use cases
- Security is handled by **filesystem sandbox** instead

### **Implementation**:

#### **A. Filename Validation**
```
✅ Length limit (255 characters)
✅ Null byte detection
✅ Path traversal prevention (../, /, \)
✅ Dangerous character sanitization
```

#### **B. Filesystem Sandbox** (Primary Security)
```
✅ Files stored with 0644 permissions (no execute on creation)
✅ Files changed to 0444 after upload (read-only, no execute)
✅ Path validation (files cannot escape uploads/ directory)
✅ Optional: noexec mount flag (Linux)
```

**How it works:**
1. File uploaded → stored with `0644` (rw-r--r--)
2. Sandbox applied → changed to `0444` (r--r--r--)
3. Execute bits removed → file **cannot run on server**
4. Write bits removed → file **cannot be modified**

**Example:**
```bash
$ ls -l uploads/
-r--r--r-- 1 user user 1024 May 29 18:00 malware.exe
-r--r--r-- 1 user user 2048 May 29 18:01 script.sh

$ ./uploads/malware.exe
bash: Permission denied  ✓ Blocked by filesystem!
```

#### **C. Optional: noexec Mount (Linux)**

For extra security, mount uploads directory with `noexec` flag:
```bash
sudo mount -o remount,noexec ./uploads
```

This prevents **any** file execution, even if permissions change.

### **Why This Approach?**

**Traditional approach (blocking file types):**
- ❌ Blocks .exe, .sh, .bat, etc.
- ❌ Easily bypassed (rename .exe to .txt)
- ❌ Limits legitimate use cases
- ❌ Doesn't prevent actual execution

**GoFileBeam approach (filesystem sandbox):**
- ✅ Allow all file types
- ✅ Prevent execution via filesystem permissions
- ✅ Cannot be bypassed
- ✅ Users can share anything safely

### **Configuration**:
```bash
# Sandbox is always enabled
# Files automatically secured after upload
# No configuration needed
```

---

## 🌐 4. HTTP Security Headers

### **Purpose**: Protect against common web attacks

### **Headers Implemented**:
```http
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Content-Security-Policy: default-src 'self'
Strict-Transport-Security: max-age=31536000
```

### **Protection Against**:
- MIME type sniffing attacks
- Clickjacking
- Cross-site scripting (XSS)
- Man-in-the-middle attacks
- Content injection

---

## 🔒 5. HTTPS/TLS

### **Purpose**: Encrypt data in transit

### **Features**:
```
✅ TLS 1.3 support
✅ Strong cipher suites
✅ Certificate validation
✅ HSTS header
```

### **Configuration**:
```bash
GOFILEBEAM_ENABLE_HTTPS=true
GOFILEBEAM_TLS_CERT_PATH=/path/to/cert.pem
GOFILEBEAM_TLS_KEY_PATH=/path/to/key.pem
```

---

## 📊 6. Storage Quota Management

### **Purpose**: Prevent storage exhaustion attacks

### **Features**:
```
✅ Configurable total storage limit
✅ Per-file size limits
✅ Real-time usage tracking
✅ Automatic rejection when full
```

### **Configuration**:
```bash
GOFILEBEAM_MAX_STORAGE_GB=1
GOFILEBEAM_MAX_FILE_SIZE_MB=100
```

---

## ⏰ 7. Automatic File Expiration

### **Purpose**: Prevent long-term storage abuse

### **Features**:
```
✅ Time-based expiration
✅ Download-count based expiration
✅ Automatic cleanup
✅ Secure deletion
```

### **Options**:
- 1 Download or 1 Day
- 10 Downloads or 7 Days
- 100 Downloads or 30 Days

---

## 🚨 8. Abuse Detection & Prevention

### **A. Upload Abuse Prevention**
```
✅ Rate limiting per IP
✅ File size limits
✅ Storage quota enforcement
✅ Dangerous file type blocking
✅ Content validation
```

### **B. Download Abuse Prevention**
```
✅ Rate limiting per IP
✅ Brute force protection
✅ Download count limits
✅ Time-based expiration
```

### **C. Spam Prevention**
```
✅ No public file listing
✅ No search functionality
✅ Unique file IDs (not guessable)
✅ No user accounts (no spam targets)
```

### **D. DDoS Mitigation**
```
✅ Rate limiting
✅ Connection limits
✅ Request timeouts
✅ Automatic IP blocking
```

---

## 🎣 9. Phishing Prevention

### **A. File-Based Phishing**
```
✅ Dangerous file type blocking
✅ Double extension detection
✅ Content scanning for phishing patterns
✅ HTML/JavaScript validation
```

### **B. Link-Based Phishing**
```
✅ No URL shortening
✅ Clear file IDs in URLs
✅ Download page shows file info
✅ Password protection available
```

### **C. Social Engineering Protection**
```
✅ Clear UI warnings for dangerous files
✅ File type indicators
✅ Expiration information displayed
✅ Download confirmation
```

---

## 🔍 10. Logging & Monitoring

### **What is Logged**:
```
✅ Upload attempts (IP, timestamp, file size)
✅ Download attempts (IP, timestamp, file ID)
✅ Failed password attempts
✅ Rate limit violations
✅ Blocked IPs
✅ File deletions
```

### **What is NOT Logged**:
```
❌ File contents
❌ Passwords
❌ Decrypted data
❌ User personal information
```

### **Log Rotation**:
- Automatic cleanup of old logs
- Configurable retention period
- Privacy-preserving logging

---

## 🛠️ Implementation Guide

### **1. Enable All Security Features**

```go
// In main.go
import "gofilebeam/internal/security"

// Create security components
rateLimiter := security.NewRateLimiter(cfg.RateLimitPerMinute)
bruteForce := security.NewBruteForceProtection()
fileValidator := security.NewFileValidator()

// Add to handlers
handler := handlers.NewHandler(storage, cfg, rateLimiter, bruteForce, fileValidator)
```

### **2. Add Rate Limiting Middleware**

```go
func rateLimitMiddleware(rl *security.RateLimiter, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := security.GetClientIP(r)
        
        if !rl.Allow(ip) {
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### **3. Add File Validation**

```go
// In upload handler
if err := fileValidator.ValidateFilename(filename); err != nil {
    http.Error(w, "Invalid filename", http.StatusBadRequest)
    return
}

if err := fileValidator.ValidateContent(data, filename); err != nil {
    http.Error(w, "Suspicious file content", http.StatusBadRequest)
    return
}
```

### **4. Add Brute Force Protection**

```go
// In download handler
if bruteForce.IsBlocked(fileID, ip) {
    http.Error(w, "Too many failed attempts", http.StatusTooManyRequests)
    return
}

// On failed password
if !passwordValid {
    if !bruteForce.RecordFailedAttempt(fileID, ip) {
        http.Error(w, "Blocked due to too many failed attempts", http.StatusTooManyRequests)
        return
    }
    http.Error(w, "Invalid password", http.StatusUnauthorized)
    return
}

// On successful auth
bruteForce.ResetAttempts(fileID, ip)
```

---

## 📈 Security Metrics

### **Track These Metrics**:
```
- Upload rate per IP
- Failed password attempts
- Blocked IPs count
- Storage usage
- File type distribution
- Average file size
- Download patterns
```

### **Alert On**:
```
⚠️ Sustained high upload rate from single IP
⚠️ Many failed password attempts
⚠️ Unusual file types
⚠️ Storage quota approaching limit
⚠️ Spike in blocked IPs
```

---

## 🔧 Advanced Configuration

### **Strict Mode** (Maximum Security)
```bash
GOFILEBEAM_MAX_FILE_SIZE_MB=10
GOFILEBEAM_RATE_LIMIT_PER_MINUTE=10
GOFILEBEAM_MAX_STORAGE_GB=0.5
GOFILEBEAM_ENABLE_HTTPS=true
# All file types allowed - security via sandbox
```

### **Relaxed Mode** (More Permissive)
```bash
GOFILEBEAM_MAX_FILE_SIZE_MB=500
GOFILEBEAM_RATE_LIMIT_PER_MINUTE=120
GOFILEBEAM_MAX_STORAGE_GB=10
# All file types allowed - security via sandbox
```

---

## 🚀 Production Deployment Checklist

### **Before Going Live**:
- [ ] Enable HTTPS with valid certificate
- [ ] Configure appropriate rate limits
- [ ] Set reasonable storage quotas
- [ ] Enable file validation
- [ ] Configure logging
- [ ] Set up monitoring
- [ ] Test brute force protection
- [ ] Review security headers
- [ ] Configure firewall rules
- [ ] Set up backup system
- [ ] Document incident response plan
- [ ] Test DDoS mitigation
- [ ] Review file type restrictions
- [ ] Enable automatic cleanup
- [ ] Configure alerting

---

## 🆘 Incident Response

### **If Under Attack**:

1. **DDoS Attack**:
   ```bash
   # Reduce rate limit
   GOFILEBEAM_RATE_LIMIT_PER_MINUTE=10
   
   # Check blocked IPs
   grep "Rate limit exceeded" /var/log/gofilebeam.log
   
   # Add firewall rules for top offenders
   ```

2. **Brute Force Attack**:
   ```bash
   # Check failed attempts
   grep "Invalid password" /var/log/gofilebeam.log
   
   # Identify targeted files
   # Consider deleting compromised files
   ```

3. **Storage Abuse**:
   ```bash
   # Reduce storage limit
   GOFILEBEAM_MAX_STORAGE_GB=0.5
   
   # Reduce file size limit
   GOFILEBEAM_MAX_FILE_SIZE_MB=10
   
   # Clean up old files
   ```

4. **Malware Upload**:
   ```bash
   # Sandbox is always active - files cannot execute
   
   # Optionally scan uploads directory
   clamscan -r /var/gofilebeam/uploads
   
   # Delete suspicious files if needed
   ```

---

## 📚 Additional Resources

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE Top 25](https://cwe.mitre.org/top25/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [Go Security Best Practices](https://golang.org/doc/security/)

---

**GoFileBeam** - Secure by design, protected by default.
