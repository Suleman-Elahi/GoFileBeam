# GoFileBeam Quick Start Guide

## 🚀 Getting Started

### 1. Build the Application
```bash
cd /home/ilfs/Public/GoFileBeam
go build -o gofilebeam ./cmd/gofilebeam
```

### 2. Configure (Optional)
Edit the `.env` file to customize settings:
```bash
# Server Configuration
GOFILEBEAM_PORT=8098
GOFILEBEAM_HOST=0.0.0.0

# Storage Configuration
GOFILEBEAM_STORAGE_PATH=./uploads
GOFILEBEAM_MAX_STORAGE_GB=1
GOFILEBEAM_MAX_FILE_SIZE_MB=100
```

### 3. Run the Service
```bash
./gofilebeam
```

The service will start on `http://localhost:8098` (or your configured port).

### 4. Access the Web Interface
Open your browser and navigate to:
```
http://localhost:8098
```

## 📤 Uploading Files

1. **Drag and drop** files onto the upload area, or click "Select Files"
2. Choose an **expiration option**:
   - 1 Download or 1 Day
   - 10 Downloads or 7 Days
   - 100 Downloads or 30 Days
3. (Optional) Add a **password** for encryption
4. Click **Upload**
5. Copy the generated **share link**

## 📥 Downloading Files

1. Click on the download link provided by the sender
2. If password-protected, enter the password
3. File downloads automatically

## 🔧 Using the Makefile

```bash
# Build the binary
make build

# Run the service
make run

# Run tests
make test

# Install as system service (requires root)
sudo make install

# Clean build artifacts
make clean
```

## 🐳 Docker Deployment

```bash
# Build Docker image
docker build -t gofilebeam .

# Run container
docker run -p 8098:8098 -v ./uploads:/uploads gofilebeam
```

## 🔒 Security Features

- **End-to-End Encryption**: Password-protected files are encrypted with AES-256-GCM
- **Automatic Expiration**: Files are automatically deleted after expiration
- **Storage Quota**: Configurable storage limits prevent abuse
- **Secure Headers**: HTTP security headers enabled by default

## 📊 API Endpoints

### Upload File
```bash
curl -X POST -F "files=@myfile.txt" \
  -F "expiration_option=1 Download or 1 Day" \
  -F "password=secret123" \
  http://localhost:8098/api/upload
```

### Download File
```bash
curl -O "http://localhost:8098/api/download/{file_id}?password=secret123"
```

### Storage Info
```bash
curl http://localhost:8098/api/storage
```

### Health Check
```bash
curl http://localhost:8098/api/health
```

## 🛠️ Troubleshooting

### Port Already in Use
Change the port in `.env`:
```bash
GOFILEBEAM_PORT=9000
```

### Storage Full
Increase storage limit in `.env`:
```bash
GOFILEBEAM_MAX_STORAGE_GB=5
```

### Upload Fails
Check file size limit in `.env`:
```bash
GOFILEBEAM_MAX_FILE_SIZE_MB=200
```

### Service Won't Start
Check logs and ensure:
- Port is not in use
- Storage directory is writable
- Configuration is valid

## 📝 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GOFILEBEAM_PORT` | Server port | `8080` |
| `GOFILEBEAM_HOST` | Server host | `0.0.0.0` |
| `GOFILEBEAM_STORAGE_PATH` | Upload directory | `./uploads` |
| `GOFILEBEAM_MAX_STORAGE_GB` | Max storage in GB | `1` |
| `GOFILEBEAM_MAX_FILE_SIZE_MB` | Max file size in MB | `100` |
| `GOFILEBEAM_ENABLE_HTTPS` | Enable HTTPS | `false` |
| `GOFILEBEAM_TLS_CERT_PATH` | TLS certificate path | - |
| `GOFILEBEAM_TLS_KEY_PATH` | TLS key path | - |
| `GOFILEBEAM_CLEANUP_INTERVAL_MINUTES` | Cleanup interval | `60` |

## 🎯 Production Deployment

### Linux Systemd Service

1. Build the binary:
```bash
make build-prod
```

2. Install as service:
```bash
sudo ./deploy.sh install
```

3. Check status:
```bash
sudo systemctl status gofilebeam
```

4. View logs:
```bash
sudo journalctl -u gofilebeam -f
```

### HTTPS Configuration

1. Obtain SSL certificates (e.g., from Let's Encrypt)
2. Update `.env`:
```bash
GOFILEBEAM_ENABLE_HTTPS=true
GOFILEBEAM_TLS_CERT_PATH=/path/to/cert.pem
GOFILEBEAM_TLS_KEY_PATH=/path/to/key.pem
```

3. Restart service

## 📚 Additional Resources

- **Privacy Policy**: http://localhost:8098/pages/privacy.html
- **Terms of Service**: http://localhost:8098/pages/terms.html
- **Security Info**: http://localhost:8098/pages/security.html
- **Help & Support**: http://localhost:8098/pages/help.html

## 🐛 Known Issues

### Tailwind CSS CDN Warning
The UI currently uses Tailwind CSS via CDN. This is fine for development but not recommended for production. To fix:

1. Install Tailwind CSS locally
2. Build the CSS file
3. Update HTML to use local CSS

See README.md for detailed instructions.

## 💡 Tips

- Use strong passwords for sensitive files
- Choose appropriate expiration settings
- Monitor storage usage regularly
- Keep the service updated
- Use HTTPS in production
- Set up regular backups of the uploads directory

## 🤝 Support

For issues or questions:
- Check the Help page: http://localhost:8098/pages/help.html
- Review logs: `journalctl -u gofilebeam -f`
- Check GitHub issues (if applicable)

---

**GoFileBeam** - Secure, lightweight file sharing made simple.
