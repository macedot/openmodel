#!/bin/sh
# This script installs openmodel on Linux
# It detects OS and architecture, downloads the binary, and installs it

set -eu

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
RESET='\033[0m'

# Helper functions
info() {
    echo "${GREEN}>>> $*${NC}"
}

warn() {
    echo "${YELLOW}! $*${NC}"
}

error() {
    echo "${RED}ERROR: $*${NC}" >&2
    exit 1
}

# Check if running as root
if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
else
    SUDO="sudo"
fi

# OS detection
OS="$(uname -s)"
if [ "$OS" != "Linux" ]; then
    error "This script only supports Linux. Detected: $OS"
fi

# Architecture detection
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

info "Detected Linux $ARCH"

# Get latest version from GitHub API
info "Fetching latest release..."
VERSION=$(curl -s https://api.github.com/repos/macedot/openmodel/releases/latest | grep -o '"tag_name": "[0-9]*' | sed 's/"//g')

if [ -z "$VERSION" ]; then
    error "Failed to fetch version from GitHub"
fi

# Allow version override
if [ -n "$OPENMODEL_VERSION" ]; then
    VERSION="$OPENMODEL_VERSION"
    info "Using OPENMODEL_VERSION=$VERSION"
fi

# Download URL
DOWNLOAD_URL="https://github.com/macedot/openmodel/releases/download/${VERSION}/openmodel-linux-${ARCH}"

# Download binary
info "Downloading openmodel ${VERSION} for ${ARCH}..."
if ! curl -fsSL --progress-bar -o /usr/local/bin/openmodel "$DOWNLOAD_URL"; then
    error "Failed to download openmodel"
fi

# Make executable
chmod +x /usr/local/bin/openmodel

# Create systemd service (optional)
if [ -d /etc/systemd/system ]; then
    info "Creating systemd service..."
    cat <<EOF | $SUDO tee /etc/systemd/system/openmodel.service > /dev/null
[Unit]
Description=openmodel API proxy server
After=network.target.wants network-online.target
[Service]
Type=simple
ExecStart=/usr/local/bin/openmodel
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
    
    info "Systemd service created. Enable with: sudo systemctl enable openmodel"
fi

# Update README
info "Updating README..."

# Success message
echo ""
info "Installation complete!"
info ""
info "openmodel hasVERSION} has been installed to /usr/local/bin/openmodel"
info ""
if [ -d /etc/systemd/system/openmodel.service ]; then
    info "To start the server:"
    info "  sudo systemctl start openmodel"
    info ""
    info "To check status:"
    info "  sudo systemctl status openmodel"
fi
info ""
info "Run 'openmodel' to start the server."
echo ""
