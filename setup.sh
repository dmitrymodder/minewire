#!/bin/bash
# Minewire Server Installation & Update Script
# Usage: sudo ./setup.sh

set -e  # Exit on any error

# Configuration
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="minewire-server"
CONFIG_DIR="/etc/minewire"
SERVICE_NAME="minewire-server"
DATA_BACKUP="/tmp/minewire_config_backup"

print_header() {
    echo "========================================="
    echo "Minewire Server Setup (v25.12.4)"
    echo "========================================="
    echo ""
}

check_root() {
    if [ "$EUID" -ne 0 ]; then 
        echo "Error: This script must be run as root (use sudo)"
        exit 1
    fi
}

check_deps() {
    echo "[Checking dependencies...]"
    if ! command -v go &> /dev/null; then
        echo "Error: Go compiler not found!"
        echo "Please install Go from https://golang.org/dl/"
        exit 1
    fi
    echo "✓ Go compiler found: $(go version)"
    echo ""
}

detect_installed_version() {
    if command -v $BINARY_NAME &> /dev/null; then
        # Try to get version using --version flag (added in 25.12.4)
        if $BINARY_NAME --version &> /dev/null; then
             INSTALLED_VER=$($BINARY_NAME --version | head -n 1)
             echo "Detected installed version: $INSTALLED_VER"
        else
             echo "Detected installed version: Legacy (Pre-25.12.4)"
        fi
        return 0 # Found
    fi
    return 1 # Not found
}

compile_server() {
    echo "[Compiling Minewire server...]"
    go build -o $BINARY_NAME -ldflags="-s -w" .
    if [ ! -f "$BINARY_NAME" ]; then
        echo "Error: Compilation failed!"
        exit 1
    fi
    echo "✓ Server compiled successfully"
    echo ""
}

backup_config() {
    if [ -f "$CONFIG_DIR/server.yaml" ]; then
        echo "[Backing up configuration...]"
        mkdir -p $DATA_BACKUP
        cp $CONFIG_DIR/server.yaml $DATA_BACKUP/server.yaml
        if [ -f "$CONFIG_DIR/server-icon.png" ]; then
            cp $CONFIG_DIR/server-icon.png $DATA_BACKUP/server-icon.png
        fi
        echo "✓ Backup saved to $DATA_BACKUP"
    fi
}

restore_config() {
    if [ -f "$DATA_BACKUP/server.yaml" ]; then
        echo "[Restoring configuration...]"
        mkdir -p $CONFIG_DIR
        cp $DATA_BACKUP/server.yaml $CONFIG_DIR/server.yaml
        if [ -f "$DATA_BACKUP/server-icon.png" ]; then
             cp $DATA_BACKUP/server-icon.png $CONFIG_DIR/server-icon.png
        fi
        echo "✓ Configuration restored."
        return 0
    fi
    return 1
}

stop_server() {
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo "Stopping running server..."
        systemctl stop $SERVICE_NAME
    fi
}

install_files() {
    # Create user
    if ! id "minewire" &>/dev/null; then
        useradd --system --no-create-home --shell /bin/false minewire
    fi

    # Install binary
    echo "[Installing binary...]"
    install -m 755 $BINARY_NAME $INSTALL_DIR/$BINARY_NAME
    
    # Config
    mkdir -p $CONFIG_DIR
    
    # Try to restore first, otherwise copy default
    if ! restore_config; then
        if [ ! -f "$CONFIG_DIR/server.yaml" ]; then
             echo "Copying default configuration..."
             cp server.yaml $CONFIG_DIR/server.yaml
        else
             echo "Keeping existing configuration."
        fi
        
        if [ -f "server-icon.png" ] && [ ! -f "$CONFIG_DIR/server-icon.png" ]; then
            cp server-icon.png $CONFIG_DIR/server-icon.png
        fi
    fi

    # Permissions
    chown -R minewire:minewire $CONFIG_DIR
    chmod 750 $CONFIG_DIR
    chmod 640 $CONFIG_DIR/server.yaml

    # Service
    cp minewire-server.service /etc/systemd/system/$SERVICE_NAME.service
    chmod 644 /etc/systemd/system/$SERVICE_NAME.service
    systemctl daemon-reload
}

main() {
    print_header
    check_root
    check_deps

    MODE="INSTALL"

    if detect_installed_version; then
        echo ""
        echo "Existing installation found."
        echo "Do you want to [U]pdate (keep config) or [R]einstall (wipe config)?"
        read -p "(U/r): " CHOICE
        case "$CHOICE" in 
            r|R) MODE="REINSTALL" ;;
            *) MODE="UPDATE" ;;
        esac
    fi

    echo ""
    echo "Starting $MODE process..."
    echo ""

    compile_server
    
    if [ "$MODE" == "UPDATE" ]; then
        stop_server
        backup_config
    fi
    
    if [ "$MODE" == "REINSTALL" ]; then
         stop_server
         # No backup, clean start
         rm -rf $CONFIG_DIR
    fi

    install_files
    
    # Cleanup build artifact
    rm -f $BINARY_NAME
    rm -rf $DATA_BACKUP

    echo ""
    echo "========================================="
    echo "Setup Complete!"
    echo "========================================="
    
    if [ "$MODE" == "UPDATE" ]; then
        echo "Attempting to restart service..."
        systemctl start $SERVICE_NAME
        
        sleep 2
        if systemctl is-active --quiet $SERVICE_NAME; then
            echo "✓ Service restarted successfully."
        else
            echo "❌ Service failed to start!"
            echo "--- Recent Logs ---"
            journalctl -u $SERVICE_NAME -n 20 --no-pager
            echo "-------------------"
            echo "Please check /etc/minewire/server.yaml for syntax errors."
        fi
    else
        echo "Don't forget to configure: /etc/minewire/server.yaml"
        echo "Then start: systemctl start $SERVICE_NAME"
    fi
}

main
