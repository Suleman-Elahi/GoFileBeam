#!/bin/bash

# GoFileBeam Sandbox Setup Script
# Configures secure storage directory with noexec mount

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
STORAGE_DIR="/var/gofilebeam/uploads"
SERVICE_USER="gofilebeam"

print_header() {
    echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║   GoFileBeam Sandbox Setup            ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
    echo ""
}

print_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

print_info() {
    echo -e "${BLUE}[i]${NC} $1"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "This script must be run as root"
        echo "Please run: sudo $0"
        exit 1
    fi
}

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        print_info "Detected OS: $PRETTY_NAME"
    else
        print_warning "Cannot detect OS, assuming Linux"
        OS="linux"
    fi
}

create_storage_directory() {
    print_info "Creating storage directory..."
    
    if [ -d "$STORAGE_DIR" ]; then
        print_warning "Directory already exists: $STORAGE_DIR"
    else
        mkdir -p "$STORAGE_DIR"
        print_success "Created directory: $STORAGE_DIR"
    fi
}

set_permissions() {
    print_info "Setting secure permissions..."
    
    # Set directory permissions
    chmod 755 "$STORAGE_DIR"
    print_success "Set directory permissions: 755"
    
    # Create service user if doesn't exist
    if ! id "$SERVICE_USER" &>/dev/null; then
        useradd -r -s /bin/false -d /nonexistent "$SERVICE_USER"
        print_success "Created service user: $SERVICE_USER"
    else
        print_info "Service user already exists: $SERVICE_USER"
    fi
    
    # Set ownership
    chown -R "$SERVICE_USER:$SERVICE_USER" "$STORAGE_DIR"
    print_success "Set ownership: $SERVICE_USER:$SERVICE_USER"
}

configure_noexec_tmpfs() {
    print_info "Configuring tmpfs with noexec (temporary, for testing)..."
    
    # Check if already mounted
    if mount | grep -q "$STORAGE_DIR"; then
        print_warning "Directory already mounted, unmounting first..."
        umount "$STORAGE_DIR" 2>/dev/null || true
    fi
    
    # Mount as tmpfs with noexec
    mount -t tmpfs -o size=1G,noexec,nosuid,nodev,mode=0755 tmpfs "$STORAGE_DIR"
    
    # Restore ownership after mount
    chown -R "$SERVICE_USER:$SERVICE_USER" "$STORAGE_DIR"
    
    print_success "Mounted tmpfs with noexec flags"
    print_warning "This is temporary and will be lost on reboot"
    print_info "To make permanent, add to /etc/fstab (see below)"
}

configure_noexec_bind() {
    print_info "Configuring bind mount with noexec..."
    
    # Create actual storage directory
    ACTUAL_DIR="/var/gofilebeam/storage"
    mkdir -p "$ACTUAL_DIR"
    chown -R "$SERVICE_USER:$SERVICE_USER" "$ACTUAL_DIR"
    
    # Bind mount with noexec
    mount --bind "$ACTUAL_DIR" "$STORAGE_DIR"
    mount -o remount,noexec,nosuid,nodev "$STORAGE_DIR"
    
    print_success "Configured bind mount with noexec"
}

add_to_fstab() {
    print_info "Adding to /etc/fstab for persistence..."
    
    # Backup fstab
    cp /etc/fstab /etc/fstab.backup.$(date +%Y%m%d_%H%M%S)
    print_success "Backed up /etc/fstab"
    
    # Check if entry already exists
    if grep -q "$STORAGE_DIR" /etc/fstab; then
        print_warning "Entry already exists in /etc/fstab"
        return
    fi
    
    # Add tmpfs entry
    echo "" >> /etc/fstab
    echo "# GoFileBeam secure storage (noexec)" >> /etc/fstab
    echo "tmpfs $STORAGE_DIR tmpfs size=1G,noexec,nosuid,nodev,mode=0755,uid=$(id -u $SERVICE_USER),gid=$(id -g $SERVICE_USER) 0 0" >> /etc/fstab
    
    print_success "Added entry to /etc/fstab"
    print_info "Mount will persist across reboots"
}

test_noexec() {
    print_info "Testing noexec protection..."
    
    # Create test script
    TEST_FILE="$STORAGE_DIR/test_exec.sh"
    echo '#!/bin/bash' > "$TEST_FILE"
    echo 'echo "This should not execute"' >> "$TEST_FILE"
    chmod +x "$TEST_FILE"
    
    # Try to execute
    if "$TEST_FILE" 2>/dev/null; then
        print_error "SECURITY ISSUE: File executed despite noexec!"
        print_error "noexec protection is NOT working"
        rm -f "$TEST_FILE"
        return 1
    else
        print_success "noexec protection is working correctly"
        print_success "Files cannot be executed from $STORAGE_DIR"
    fi
    
    # Cleanup
    rm -f "$TEST_FILE"
}

show_status() {
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║   Sandbox Status                       ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
    echo ""
    
    echo "Storage Directory: $STORAGE_DIR"
    echo "Permissions: $(stat -c %a $STORAGE_DIR 2>/dev/null || echo 'N/A')"
    echo "Owner: $(stat -c %U:%G $STORAGE_DIR 2>/dev/null || echo 'N/A')"
    echo ""
    
    echo "Mount Information:"
    mount | grep "$STORAGE_DIR" || echo "  Not mounted with special flags"
    echo ""
    
    echo "Security Flags:"
    if mount | grep "$STORAGE_DIR" | grep -q "noexec"; then
        print_success "noexec: Enabled"
    else
        print_warning "noexec: Not enabled"
    fi
    
    if mount | grep "$STORAGE_DIR" | grep -q "nosuid"; then
        print_success "nosuid: Enabled"
    else
        print_warning "nosuid: Not enabled"
    fi
    
    if mount | grep "$STORAGE_DIR" | grep -q "nodev"; then
        print_success "nodev: Enabled"
    else
        print_warning "nodev: Not enabled"
    fi
}

show_recommendations() {
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║   Security Recommendations             ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
    echo ""
    
    print_info "1. Verify noexec is working:"
    echo "   sudo $0 test"
    echo ""
    
    print_info "2. Monitor the directory:"
    echo "   watch -n 5 'ls -lah $STORAGE_DIR'"
    echo ""
    
    print_info "3. Check mount status:"
    echo "   mount | grep $STORAGE_DIR"
    echo ""
    
    print_info "4. View file permissions:"
    echo "   ls -la $STORAGE_DIR"
    echo ""
    
    print_info "5. Start GoFileBeam:"
    echo "   sudo systemctl start gofilebeam"
    echo ""
}

main() {
    print_header
    
    case "${1:-setup}" in
        setup)
            check_root
            detect_os
            create_storage_directory
            set_permissions
            configure_noexec_tmpfs
            add_to_fstab
            test_noexec
            show_status
            show_recommendations
            ;;
        test)
            check_root
            test_noexec
            ;;
        status)
            show_status
            ;;
        cleanup)
            check_root
            print_warning "Unmounting and removing sandbox..."
            umount "$STORAGE_DIR" 2>/dev/null || true
            print_success "Sandbox cleaned up"
            ;;
        *)
            echo "Usage: $0 {setup|test|status|cleanup}"
            echo ""
            echo "Commands:"
            echo "  setup   - Configure secure sandbox (default)"
            echo "  test    - Test noexec protection"
            echo "  status  - Show sandbox status"
            echo "  cleanup - Remove sandbox configuration"
            exit 1
            ;;
    esac
}

main "$@"
