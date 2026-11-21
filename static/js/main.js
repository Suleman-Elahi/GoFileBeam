// GoFileBeam Frontend JavaScript
// Connects the UI to the Go backend API

class GoFileBeamUI {
    constructor() {
        this.files = [];
        this.currentUploadId = null;
        this.apiBase = window.location.origin;

        // Upload config fetched from the server (chunk size / threshold).
        // Defaults are deliberately chunking-friendly: if the config request
        // fails, a large file still takes the chunked path (server responds
        // with a clear 503 if chunking is truly unavailable) instead of the
        // legacy single-request path, which a CDN in front would reject
        // with 413 for anything over its body size limit.
        this.uploadConfig = {
            maxFileSize: 100 * 1024 * 1024,
            chunkSize: 8 * 1024 * 1024,
            chunkUploadThreshold: 80 * 1024 * 1024,
            chunkedUploadEnabled: true,
            chunkConcurrency: 6
        };

        this.initElements();
        this.initEventListeners();
        this.loadStorageInfo();
        // Keep the promise so uploadFiles() can await it: if the config has
        // not loaded yet, the chunked path would be considered "disabled"
        // and a large file would silently fall back to the single-request
        // endpoint (which the CDN rejects with 413).
        this.uploadConfigPromise = this.loadUploadConfig();
    }

    initElements() {
        // Form elements
        this.dropZone = document.getElementById('dropZone');
        this.fileInput = document.getElementById('fileInput');
        this.selectFilesBtn = document.getElementById('selectFilesBtn');
        this.uploadBtn = document.getElementById('uploadBtn');
        this.clearAllBtn = document.getElementById('clearAllBtn');
        
        // Options
        this.expirationSelect = document.getElementById('expirationSelect');
        this.passwordInput = document.getElementById('passwordInput');
        
        // Progress elements
        this.uploadProgressContainer = document.getElementById('uploadProgressContainer');
        this.uploadProgressBar = document.getElementById('uploadProgressBar');
        this.uploadProgressText = document.getElementById('uploadProgressText');
        this.uploadIcon = document.getElementById('uploadIcon');
        this.dropZoneText = document.getElementById('dropZoneText');
        
        // File list
        this.fileList = document.getElementById('fileList');
        this.fileListContainer = document.getElementById('fileListContainer');
        
        // Share link elements
        this.shareLinkContainer = document.getElementById('shareLinkContainer');
        this.shareLinkInput = document.getElementById('shareLinkInput');
        this.copyLinkBtn = document.getElementById('copyLinkBtn');
        this.downloadLinkBtn = document.getElementById('downloadLinkBtn');
        this.expirationInfo = document.getElementById('expirationInfo');
        
        // Storage elements
        this.storageUsed = document.getElementById('storageUsed');
        this.storageTotal = document.getElementById('storageTotal');
        this.storageProgress = document.getElementById('storageProgress');
        
        // Toast
        this.toast = document.getElementById('toast');
        this.toastMessage = document.getElementById('toastMessage');
        this.toastIcon = document.getElementById('toastIcon');
        this.toastClose = document.getElementById('toastClose');
    }

    initEventListeners() {
        // File selection
        this.selectFilesBtn.addEventListener('click', () => this.fileInput.click());
        this.fileInput.addEventListener('change', (e) => this.handleFileSelect(e));
        
        // Drag and drop
        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
            this.dropZone.addEventListener(eventName, this.preventDefaults, false);
        });
        
        ['dragenter', 'dragover'].forEach(eventName => {
            this.dropZone.addEventListener(eventName, () => this.dropZone.classList.add('dragover'), false);
        });
        
        ['dragleave', 'drop'].forEach(eventName => {
            this.dropZone.addEventListener(eventName, () => this.dropZone.classList.remove('dragover'), false);
        });
        
        this.dropZone.addEventListener('drop', (e) => this.handleDrop(e), false);
        
        // Upload button
        this.uploadBtn.addEventListener('click', () => this.uploadFiles());
        
        // Clear all files
        this.clearAllBtn.addEventListener('click', () => this.clearAllFiles());
        
        // Copy link
        this.copyLinkBtn.addEventListener('click', () => this.copyShareLink());
        
        // Toast close
        this.toastClose.addEventListener('click', () => this.hideToast());
        
        // Update upload button state when files change
        this.fileInput.addEventListener('change', () => this.updateUploadButton());
    }

    preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    handleFileSelect(e) {
        const files = Array.from(e.target.files);
        this.addFiles(files);
    }

    handleDrop(e) {
        const files = Array.from(e.dataTransfer.files);
        this.addFiles(files);
    }

    addFiles(files) {
        files.forEach(file => {
            // Check if file already exists
            if (this.files.some(f => f.name === file.name && f.size === file.size)) {
                this.showToast('File already added', 'warning');
                return;
            }
            
            // Add to files array
            this.files.push({
                file: file,
                id: Date.now() + Math.random().toString(36).substr(2, 9),
                name: file.name,
                size: file.size,
                type: file.type
            });
        });
        
        this.renderFileList();
        this.updateUploadButton();
    }

    removeFile(id) {
        this.files = this.files.filter(file => file.id !== id);
        this.renderFileList();
        this.updateUploadButton();
    }

    clearAllFiles() {
        this.files = [];
        this.renderFileList();
        this.updateUploadButton();
        this.showToast('All files cleared', 'info');
    }

    renderFileList() {
        this.fileList.innerHTML = '';
        
        if (this.files.length === 0) {
            this.fileListContainer.classList.add('hidden');
            return;
        }
        
        this.fileListContainer.classList.remove('hidden');
        
        this.files.forEach(fileData => {
            const fileItem = document.createElement('div');
            fileItem.className = 'group flex items-center justify-between p-3 bg-surface-container-low rounded-lg border border-transparent hover:border-primary-fixed transition-colors';
            fileItem.innerHTML = `
                <div class="flex items-center gap-3 overflow-hidden">
                    <span class="material-symbols-outlined text-outline">${this.getFileIcon(fileData.type)}</span>
                    <div class="flex flex-col truncate">
                        <span class="font-body-md text-body-md text-on-surface truncate">${fileData.name}</span>
                        <span class="font-mono-sm text-mono-sm text-on-surface-variant">${this.formatFileSize(fileData.size)}</span>
                    </div>
                </div>
                <button class="text-outline-variant hover:text-error opacity-0 group-hover:opacity-100 transition-opacity p-1" data-id="${fileData.id}">
                    <span class="material-symbols-outlined">close</span>
                </button>
            `;
            
            this.fileList.appendChild(fileItem);
            
            // Add event listener to remove button
            const removeBtn = fileItem.querySelector('button');
            removeBtn.addEventListener('click', () => this.removeFile(fileData.id));
        });
    }

    getFileIcon(fileType) {
        if (fileType.startsWith('image/')) return 'image';
        if (fileType.includes('pdf')) return 'description';
        if (fileType.includes('zip') || fileType.includes('compressed')) return 'folder_zip';
        if (fileType.includes('text') || fileType.includes('code')) return 'description';
        return 'insert_drive_file';
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    updateUploadButton() {
        this.uploadBtn.disabled = this.files.length === 0;
    }

    async loadUploadConfig() {
        try {
            const response = await fetch(`${this.apiBase}/api/upload/config`);
            if (!response.ok) return;
            const data = await response.json();
            this.uploadConfig = {
                maxFileSize: data.max_file_size,
                chunkSize: data.chunk_size,
                chunkUploadThreshold: data.chunk_upload_threshold,
                chunkedUploadEnabled: data.chunked_upload_enabled,
                chunkConcurrency: data.chunk_concurrency || 6
            };
        } catch (error) {
            console.error('Failed to load upload config, using defaults:', error);
        }
    }

    async uploadFiles() {
        if (this.files.length === 0) return;

        // Wait for /api/upload/config so the chunked-vs-legacy decision is
        // made with real server settings, not the conservative defaults.
        try {
            await this.uploadConfigPromise;
        } catch (error) {
            // loadUploadConfig already logs; defaults are used as-is.
        }

        this.showUploadProgress();

        const password = this.passwordInput.value.trim();
        const expirationOption = this.expirationSelect.value;

        // Multiple files are zipped together server-side via the existing
        // single-request multipart endpoint. Chunking that path would
        // require restructuring how the ZIP is assembled, so for now only
        // a single selected file is eligible for the chunked path; a
        // multi-file selection always uses the legacy endpoint.
        const singleFile = this.files.length === 1 ? this.files[0].file : null;
        const useChunked =
            this.uploadConfig.chunkedUploadEnabled &&
            singleFile &&
            singleFile.size >= this.uploadConfig.chunkUploadThreshold;

        const onSuccess = (data) => {
            this.updateProgressBar(100);
            this.showUploadSuccess(data);
            this.clearAllFiles();
            this.passwordInput.value = '';
        };

        const onError = (message) => {
            console.error('Upload error:', message);
            this.showToast(`Upload failed: ${message}`, 'error');
            this.hideUploadProgress();
        };

        // Multi-file selections go through the legacy single-request
        // endpoint (the server zips them together), so they cannot be
        // chunked. Fail fast with a clear message instead of letting the
        // request grow past the CDN's body limit and die with an opaque
        // 413 partway through the transfer.
        if (!useChunked) {
            const totalBytes = this.files.reduce((sum, f) => sum + f.file.size, 0);
            if (totalBytes >= this.uploadConfig.chunkUploadThreshold) {
                onError(
                    'Selecting multiple files at once is limited to ' +
                    `${this.formatFileSize(this.uploadConfig.chunkUploadThreshold)}` +
                    ' total. Upload large files one at a time to use chunked upload.'
                );
                return;
            }
        }

        if (useChunked) {
            this.uploadFileChunked(singleFile, expirationOption, password, onSuccess, onError);
        } else {
            this.uploadFilesLegacy(expirationOption, password, onSuccess, onError);
        }
    }

    // uploadFilesLegacy sends all selected files as a single multipart
    // request, exactly as before. Used for anything under the chunking
    // threshold, and for multi-file selections (server zips them together).
    uploadFilesLegacy(expirationOption, password, onSuccess, onError) {
        const formData = new FormData();

        this.files.forEach(fileData => {
            formData.append('files', fileData.file);
        });
        formData.append('expiration_option', expirationOption);
        if (password) {
            formData.append('password', password);
        }

        // Use XMLHttpRequest so we get real upload progress events.
        // fetch() has no cross-browser way to observe request body upload
        // progress, which is why the previous implementation faked it with
        // a timer.
        const xhr = new XMLHttpRequest();

        xhr.upload.addEventListener('progress', (e) => {
            if (e.lengthComputable) {
                const percent = Math.round((e.loaded / e.total) * 100);
                // Cap client-visible progress at 99% until the server
                // actually responds, since bytes-sent != processing-finished
                // (encryption, disk write, etc. still happen after the last
                // byte is received).
                this.updateProgressBar(Math.min(percent, 99));
            }
        });

        xhr.addEventListener('load', () => {
            if (xhr.status >= 200 && xhr.status < 300) {
                try {
                    onSuccess(JSON.parse(xhr.responseText));
                } catch (err) {
                    onError('invalid server response');
                }
            } else {
                onError(xhr.statusText || xhr.responseText || `HTTP ${xhr.status}`);
            }
        });

        xhr.addEventListener('error', () => onError('network error'));
        xhr.addEventListener('abort', () => this.hideUploadProgress());

        xhr.open('POST', `${this.apiBase}/api/upload`);
        xhr.send(formData);
    }

    // uploadFileChunked splits a single large file into fixed-size chunks
    // and uploads them as a sequence of small, independent requests. This
    // exists specifically so no single HTTP request body ever approaches a
    // reverse proxy / CDN's body size limit (e.g. Cloudflare's 100 MB cap
    // on Free/Pro plans) - a problem that cannot be solved by raising this
    // app's own size limits, since the proxy sits in front of it and
    // enforces its limit regardless of what the app allows.
    //
    // Chunks are uploaded by a small worker pool (concurrency taken from
    // /api/upload/config) rather than one at a time. Sequential uploads
    // leave the connection idle for a full round trip between every chunk
    // (server ack, next chunk request, TLS/TTFB), so throughput collapses
    // on high-latency links. The server writes each chunk to its own file
    // offset under a per-session mutex, so out-of-order arrival is safe by
    // design. Concurrency is modest (default 6) to stay at the browser's
    // per-host connection limit; the server caps it at 6 as well.
    async uploadFileChunked(file, expirationOption, password, onSuccess, onError) {
        let sessionId = null;

        try {
            const initResp = await fetch(`${this.apiBase}/api/upload/init`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    filename: file.name,
                    size: file.size,
                    expiration_option: expirationOption,
                    password: password || undefined
                })
            });

            if (!initResp.ok) {
                const text = await initResp.text();
                onError(text || `HTTP ${initResp.status}`);
                return;
            }

            const { session_id, chunk_size, total_chunks } = await initResp.json();
            sessionId = session_id;

            const totalBytes = file.size;
            // Per-chunk bytes acknowledged so far, indexed by chunk. Used
            // instead of a single running counter because with parallel
            // uploads chunks complete out of order and a simple
            // "bytesSent += " would double-count or go backwards.
            const chunkLoaded = new Array(total_chunks).fill(0);
            const reportProgress = () => {
                let sent = 0;
                for (let i = 0; i < total_chunks; i++) sent += chunkLoaded[i];
                // Cap at 99% until /complete finishes server-side assembly
                // (and encryption, if a password was set).
                this.updateProgressBar(Math.min(Math.round((sent / totalBytes) * 100), 99));
            };

            const concurrency = Math.max(
                1,
                Math.min(this.uploadConfig.chunkConcurrency || 6, 6, total_chunks)
            );

            // Work queue of chunk indexes; workers pull from it until empty.
            let nextIndex = 0;
            let firstError = null;

            const worker = async () => {
                while (firstError === null) {
                    const index = nextIndex++;
                    if (index >= total_chunks) return;

                    const start = index * chunk_size;
                    const end = Math.min(start + chunk_size, file.size);
                    const chunkBlob = file.slice(start, end);

                    try {
                        await this.uploadSingleChunkWithRetry(
                            sessionId, index, chunkBlob,
                            (loaded) => {
                                chunkLoaded[index] = loaded;
                                reportProgress();
                            }
                        );
                        chunkLoaded[index] = chunkBlob.size;
                        reportProgress();
                    } catch (err) {
                        // Record the first failure and stop pulling new
                        // work; other in-flight chunks finish on their own.
                        if (firstError === null) firstError = err;
                        return;
                    }
                }
            };

            await Promise.all(
                Array.from({ length: concurrency }, () => worker())
            );

            if (firstError !== null) throw firstError;

            const completeResp = await fetch(`${this.apiBase}/api/upload/complete`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ session_id })
            });

            if (!completeResp.ok) {
                const text = await completeResp.text();
                onError(text || `HTTP ${completeResp.status}`);
                return;
            }

            onSuccess(await completeResp.json());
        } catch (err) {
            // Best-effort cleanup so a failed upload doesn't leave the
            // server holding staged data until the idle reaper times out.
            if (sessionId) {
                try {
                    await fetch(`${this.apiBase}/api/upload/abort`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ session_id: sessionId })
                    });
                } catch (abortErr) {
                    // Ignored: the reaper cleans up abandoned sessions.
                }
            }
            onError(err.message || 'network error');
        }
    }

    // uploadSingleChunkWithRetry wraps uploadSingleChunk with bounded
    // retries and exponential backoff. Parallel uploads multiply the number
    // of requests in flight, which raises the odds of hitting a transient
    // failure (connection reset, edge hiccup, brief 5xx); without retries
    // one bad chunk would fail the whole multi-hundred-megabyte upload.
    // Client errors (4xx) are not retried - they are deterministic and
    // retrying would just burn time before failing anyway.
    async uploadSingleChunkWithRetry(sessionId, index, blob, onProgress, attempts = 3) {
        let lastErr = null;
        for (let attempt = 0; attempt < attempts; attempt++) {
            if (attempt > 0) {
                // 500ms, 1500ms - long enough to ride out a transient
                // blip, short enough not to stall the overall upload.
                await new Promise((resolve) => setTimeout(resolve, 500 * Math.pow(3, attempt - 1)));
                // Reset per-chunk progress so the bar doesn't show stale
                // bytes from the failed attempt.
                onProgress(0);
            }
            try {
                await this.uploadSingleChunk(sessionId, index, blob, onProgress);
                return;
            } catch (err) {
                lastErr = err;
                const status = err && err.status;
                if (typeof status === 'number' && status >= 400 && status < 500 && status !== 429) {
                    throw err;
                }
            }
        }
        throw lastErr;
    }

    // uploadSingleChunk uploads one chunk's raw bytes and resolves once the
    // server has acknowledged it, reporting fine-grained progress via
    // onProgress(bytesLoadedForThisChunk) so the overall progress bar moves
    // smoothly even while a single (multi-megabyte) chunk is in flight.
    uploadSingleChunk(sessionId, index, blob, onProgress) {
        return new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();

            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    onProgress(e.loaded);
                }
            });

            xhr.addEventListener('load', () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    onProgress(blob.size);
                    resolve();
                } else {
                    const error = new Error(xhr.responseText || `HTTP ${xhr.status}`);
                    error.status = xhr.status;
                    reject(error);
                }
            });

            xhr.addEventListener('error', () => reject(new Error('network error uploading chunk')));

            const url = `${this.apiBase}/api/upload/chunk?session_id=${encodeURIComponent(sessionId)}&index=${index}`;
            xhr.open('POST', url);
            xhr.setRequestHeader('Content-Type', 'application/octet-stream');
            xhr.send(blob);
        });
    }

    showUploadProgress() {
        this.uploadProgressContainer.classList.remove('hidden');
        this.uploadIcon.textContent = 'upload';
        this.dropZoneText.textContent = 'Uploading files...';
        this.uploadBtn.disabled = true;
        this.updateProgressBar(0);
    }

    updateProgressBar(percent) {
        this.uploadProgressBar.style.width = `${percent}%`;
        this.uploadProgressText.textContent = `${percent}%`;
    }

    hideUploadProgress() {
        this.uploadProgressContainer.classList.add('hidden');
        this.uploadIcon.textContent = 'upload_file';
        this.dropZoneText.textContent = 'Drag and drop files to start sharing';
        this.uploadProgressBar.style.width = '0%';
        this.uploadProgressText.textContent = '0%';
    }

    showUploadSuccess(data) {
        this.hideUploadProgress();
        
        // Show share link - use download page URL instead of direct API
        this.shareLinkContainer.classList.remove('hidden');
        
        // Build download page URL
        const downloadPageUrl = `${window.location.origin}/download/${data.id}`;
        this.shareLinkInput.value = downloadPageUrl;
        this.downloadLinkBtn.href = downloadPageUrl;
        
        // Set expiration info
        const expirationDate = new Date(data.expires_at);
        const options = { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' };
        const formattedDate = expirationDate.toLocaleDateString('en-US', options);
        this.expirationInfo.textContent = `Expires after ${data.max_downloads} downloads or on ${formattedDate}`;
        
        this.showToast('Files uploaded successfully!', 'success');
        
        // Refresh storage info
        this.loadStorageInfo();
    }

    async loadStorageInfo() {
        try {
            const response = await fetch(`${this.apiBase}/api/storage`);
            if (!response.ok) return;
            
            const data = await response.json();
            
            // Update storage display
            this.storageUsed.textContent = this.formatFileSize(data.used_bytes);
            this.storageTotal.textContent = this.formatFileSize(data.total_bytes);
            this.storageProgress.style.width = `${data.usage_percent}%`;
            
        } catch (error) {
            console.error('Failed to load storage info:', error);
        }
    }

    copyShareLink() {
        this.shareLinkInput.select();
        document.execCommand('copy');
        
        // Show feedback
        const originalText = this.copyLinkBtn.innerHTML;
        this.copyLinkBtn.innerHTML = '<span class="material-symbols-outlined text-[16px]">check</span> Copied';
        
        setTimeout(() => {
            this.copyLinkBtn.innerHTML = originalText;
        }, 2000);
        
        this.showToast('Link copied to clipboard', 'success');
    }

    showToast(message, type = 'info') {
        // Set message and icon
        this.toastMessage.textContent = message;
        
        switch (type) {
            case 'success':
                this.toastIcon.textContent = 'check_circle';
                this.toastIcon.className = 'material-symbols-outlined text-primary';
                break;
            case 'error':
                this.toastIcon.textContent = 'error';
                this.toastIcon.className = 'material-symbols-outlined text-error';
                break;
            case 'warning':
                this.toastIcon.textContent = 'warning';
                this.toastIcon.className = 'material-symbols-outlined text-tertiary';
                break;
            default:
                this.toastIcon.textContent = 'info';
                this.toastIcon.className = 'material-symbols-outlined text-on-surface-variant';
        }
        
        // Show toast
        this.toast.classList.remove('hidden');
        
        // Auto-hide after 5 seconds
        setTimeout(() => this.hideToast(), 5000);
    }

    hideToast() {
        this.toast.classList.add('hidden');
    }
}

// Initialize the UI when the page loads
document.addEventListener('DOMContentLoaded', () => {
    new GoFileBeamUI();
});