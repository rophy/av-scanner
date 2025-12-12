#!/bin/bash
set -e

# AV Scanner Installation Script
# Installs the scanner service to run natively on the host

INSTALL_DIR="/opt/av-scanner"
CONFIG_DIR="/etc/av-scanner"
SCAN_DIR="/tmp/av-scanner"
SERVICE_USER="avscanner"

echo "=== AV Scanner Installation ==="

# Check for root
if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root"
   exit 1
fi

# Check for Node.js
if ! command -v node &> /dev/null; then
    echo "Node.js is not installed. Please install Node.js 20+ first."
    exit 1
fi

NODE_VERSION=$(node -v | cut -d'v' -f2 | cut -d'.' -f1)
if [[ $NODE_VERSION -lt 20 ]]; then
    echo "Node.js 20+ is required. Current version: $(node -v)"
    exit 1
fi

# Create service user
if ! id "$SERVICE_USER" &>/dev/null; then
    echo "Creating service user: $SERVICE_USER"
    useradd -r -s /sbin/nologin -d "$INSTALL_DIR" "$SERVICE_USER"
fi

# Create directories
echo "Creating directories..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$SCAN_DIR"

# Build the application
echo "Building application..."
npm ci
npm run build

# Copy files
echo "Installing files..."
cp -r dist "$INSTALL_DIR/"
cp -r node_modules "$INSTALL_DIR/"
cp package.json "$INSTALL_DIR/"

# Install configuration
if [[ ! -f "$CONFIG_DIR/av-scanner.env" ]]; then
    cp deploy/av-scanner.env "$CONFIG_DIR/"
    echo "Configuration installed to $CONFIG_DIR/av-scanner.env"
else
    echo "Configuration already exists, skipping..."
fi

# Set permissions
echo "Setting permissions..."
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
chown -R "$SERVICE_USER:$SERVICE_USER" "$SCAN_DIR"
chmod 750 "$INSTALL_DIR"
chmod 700 "$SCAN_DIR"

# Add user to clamav group for socket access (if ClamAV is installed)
if getent group clamav > /dev/null; then
    usermod -a -G clamav "$SERVICE_USER"
    echo "Added $SERVICE_USER to clamav group"
fi

# Install systemd service
echo "Installing systemd service..."
cp deploy/av-scanner.service /etc/systemd/system/
systemctl daemon-reload

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "  1. Edit configuration: $CONFIG_DIR/av-scanner.env"
echo "  2. Ensure ClamAV or DS Agent is running on the host"
echo "  3. Start the service: systemctl start av-scanner"
echo "  4. Enable on boot: systemctl enable av-scanner"
echo "  5. Check status: systemctl status av-scanner"
echo ""
echo "For ClamAV:"
echo "  - Ensure clamd is running: systemctl status clamav-daemon"
echo "  - Socket should be at: /var/run/clamav/clamd.sock"
echo ""
echo "For Trend Micro DS Agent:"
echo "  - Ensure ds_agent is running"
echo "  - Logs should be at: /var/log/ds_agent/ds_agent.log"
