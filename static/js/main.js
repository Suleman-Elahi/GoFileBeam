// GoFileBeam Frontend JavaScript
// Connects the UI to the Go backend API

class GoFileBeamUI {
    constructor() {
        this.files = [];
        this.currentUploadId = null;
        this.apiBase = window.location.origin;
        
        this.initElements();
        this.initEventListeners();
        this.loadStorageInfo();
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

    async uploadFiles() {
        if (this.files.length === 0) return;
        
        // Show progress
        this.showUploadProgress();
        
        try {
            // Create FormData
            const formData = new FormData();
            
            // Add files with "files" field name (matches handler expectation)
            this.files.forEach(fileData => {
                formData.append('files', fileData.file);
            });
            
            // Add options
            formData.append('expiration_option', this.expirationSelect.value);
            
            const password = this.passwordInput.value.trim();
            if (password) {
                formData.append('password', password);
            }
            
            // Upload to backend
            const response = await fetch(`${this.apiBase}/api/upload`, {
                method: 'POST',
                body: formData
            });
            
            if (!response.ok) {
                throw new Error(`Upload failed: ${response.statusText}`);
            }
            
            const data = await response.json();
            
            // Show success
            this.showUploadSuccess(data);
            
            // Clear files
            this.clearAllFiles();
            this.passwordInput.value = '';
            
        } catch (error) {
            console.error('Upload error:', error);
            this.showToast(`Upload failed: ${error.message}`, 'error');
            this.hideUploadProgress();
        }
    }

    showUploadProgress() {
        this.uploadProgressContainer.classList.remove('hidden');
        this.uploadIcon.textContent = 'upload';
        this.dropZoneText.textContent = 'Uploading files...';
        this.uploadBtn.disabled = true;
        
        // Simulate progress (in real app, you'd use XMLHttpRequest for progress events)
        let progress = 0;
        const interval = setInterval(() => {
            progress += 10;
            if (progress >= 90) {
                clearInterval(interval);
            }
            this.updateProgressBar(progress);
        }, 100);
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