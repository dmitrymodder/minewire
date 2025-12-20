#!/bin/bash
# Minewire Server Installation Script
# This script compiles and installs the Minewire proxy server

set -e  # Exit on any error

echo "========================================="
echo "Minewire Server Installation"
echo "========================================="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "Error: This script must be run as root (use sudo)"
    exit 1
fi

# Check for Go compiler
echo "[1/7] Checking dependencies..."
if ! command -v go &> /dev/null; then
    echo "Error: Go compiler not found!"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi
echo "✓ Go compiler found: $(go version)"
echo ""

# Compile the server
echo "[2/7] Compiling Minewire server..."
go build -o minewire-server -ldflags="-s -w" .
if [ ! -f "minewire-server" ]; then
    echo "Error: Compilation failed!"
    exit 1
fi
echo "✓ Server compiled successfully"
echo ""

# Create minewire user if it doesn't exist
echo "[3/7] Creating system user..."
if id "minewire" &>/dev/null; then
    echo "✓ User 'minewire' already exists"
else
    useradd --system --no-create-home --shell /bin/false minewire
    echo "✓ Created system user 'minewire'"
fi
echo ""

# Install binary
echo "[4/7] Installing binary..."
install -m 755 minewire-server /usr/local/bin/minewire-server
echo "✓ Installed to /usr/local/bin/minewire-server"
echo ""

# Create configuration directory
echo "[5/7] Setting up configuration..."
mkdir -p /etc/minewire
if [ ! -f "/etc/minewire/server.yaml" ]; then
    cp server.yaml /etc/minewire/server.yaml
    echo "✓ Copied configuration to /etc/minewire/server.yaml"
else
    echo "⚠ Configuration already exists at /etc/minewire/server.yaml (not overwriting)"
fi

# Copy server icon if it exists
if [ -f "server-icon.png" ]; then
    cp server-icon.png /etc/minewire/server-icon.png
    echo "✓ Copied server icon"
fi

# Set proper ownership
chown -R minewire:minewire /etc/minewire
chmod 750 /etc/minewire
chmod 640 /etc/minewire/server.yaml
echo "✓ Set proper permissions"
echo ""

# Install systemd service
echo "[6/7] Installing systemd service..."
cp minewire-server.service /etc/systemd/system/minewire-server.service
chmod 644 /etc/systemd/system/minewire-server.service
systemctl daemon-reload
echo "✓ Service installed"
echo ""

# Cleanup
echo "[7/7] Cleaning up..."
rm -f minewire-server
echo "✓ Cleanup complete"
echo ""

echo "========================================="
echo "Installation Complete!"
echo "========================================="
echo ""
echo "IMPORTANT: Before starting the server, you must:"
echo ""
echo "1. Edit the configuration file:"
echo "   sudo nano /etc/minewire/server.yaml"
echo ""
echo "2. Generate secure passwords using:"
echo "   openssl rand -hex 16"
echo ""
echo "3. Replace the example passwords in the configuration"
echo ""
echo "4. Start the server:"
echo "   sudo systemctl start minewire-server"
echo ""
echo "5. Check server status:"
echo "   sudo systemctl status minewire-server"
echo ""
echo "6. View logs:"
echo "   sudo journalctl -u minewire-server -f"
echo ""
echo "7. Enable auto-start on boot (optional):"
echo "   sudo systemctl enable minewire-server"
echo ""
echo "========================================="
