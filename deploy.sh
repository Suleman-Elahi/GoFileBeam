#!/bin/bash

# GoFileBeam Deployment Script
# Usage: ./deploy.sh [install|uninstall|start|stop|status]

set -e

SERVICE_NAME="gofilebeam"
BINARY_NAME="gofilebeam"
INSTALL_DIR="/opt/gofilebeam"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
STORAGE_DIR="/var/gofilebeam/uploads"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}[+]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[!]${NC} $1"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "Please run as root"
        exit 1
    fi
}

install_service() {
    check_root
    
    print_status "Installing GoFileBeam..."
    
    # Create installation directory
    mkdir -p "$INSTALL_DIR"
    
    # Copy binary and UI
    cp "$BINARY_NAME" "$INSTALL_DIR/"
    cp "code.html" "$INSTALL_DIR/"
    
    # Create storage directory
    mkdir -p "$STORAGE_DIR"
    
    # Create service user
    if ! id "gofilebeam" &>/dev/null; then
        useradd -r -s /bin/false -d "$INSTALL_DIR" gofilebeam
    fi
    
    # Set permissions
    chown -R gofilebeam:gofilebeam "$INSTALL_DIR"
    chown -R gofilebeam:gofilebeam "$STORAGE_DIR"
    chmod 755 "$INSTALL_DIR/$BINARY_NAME"
    chmod 644 "$INSTALL_DIR/code.html"
    
    # Copy service file
    cp "gofilebeam.service" "$SERVICE_FILE"
    
    # Reload systemd
    systemctl daemon-reload
    
    # Enable and start service
    systemctl enable "$SERVICE_NAME"
    systemctl start "$SERVICE_NAME"
    
    print_status "GoFileBeam installed successfully!"
    print_status "Service: $SERVICE_NAME"
    print_status "Binary: $INSTALL_DIR/$BINARY_NAME"
    print_status "Storage: $STORAGE_DIR"
    print_status "Web UI: http://localhost:8080"
}

uninstall_service() {
    check_root
    
    print_warning "Uninstalling GoFileBeam..."
    
    # Stop and disable service
    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    
    # Remove service file
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    
    # Remove installation
    rm -rf "$INSTALL_DIR"
    
    # Remove storage (optional - comment out to keep data)
    # rm -rf "$STORAGE_DIR"
    
    # Remove user (optional)
    # userdel gofilebeam 2>/dev/null || true
    
    print_status "GoFileBeam uninstalled successfully!"
}

start_service() {
    check_root
    print_status "Starting GoFileBeam..."
    systemctl start "$SERVICE_NAME"
    systemctl status "$SERVICE_NAME"
}

stop_service() {
    check_root
    print_status "Stopping GoFileBeam..."
    systemctl stop "$SERVICE_NAME"
    systemctl status "$SERVICE_NAME"
}

restart_service() {
    check_root
    print_status "Restarting GoFileBeam..."
    systemctl restart "$SERVICE_NAME"
    systemctl status "$SERVICE_NAME"
}

status_service() {
    systemctl status "$SERVICE_NAME" --no-pager
}

show_usage() {
    echo "GoFileBeam Deployment Script"
    echo "Usage: $0 [install|uninstall|start|stop|restart|status]"
    echo ""
    echo "Commands:"
    echo "  install     - Install and start GoFileBeam as a system service"
    echo "  uninstall   - Remove GoFileBeam service"
    echo "  start       - Start the service"
    echo "  stop        - Stop the service"
    echo "  restart     - Restart the service"
    echo "  status      - Show service status"
    echo ""
    echo "Note: Install command requires the binary to be built first."
    echo "      Run 'go build -o gofilebeam ./cmd/gofilebeam' to build."
}

# Main script logic
case "$1" in
    install)
        if [ ! -f "$BINARY_NAME" ]; then
            print_error "Binary '$BINARY_NAME' not found. Build it first with:"
            print_error "  go build -o gofilebeam ./cmd/gofilebeam"
            exit 1
        fi
        install_service
        ;;
    uninstall)
        uninstall_service
        ;;
    start)
        start_service
        ;;
    stop)
        stop_service
        ;;
    restart)
        restart_service
        ;;
    status)
        status_service
        ;;
    *)
        show_usage
        exit 1
        ;;
esac