#!/bin/bash

# Claude Proxy - B Computer Setup Script
# This script configures the proxy environment on B computer

set -e

PROXY_PORT="${1:-8080}"
PROXY_URL="http://127.0.0.1:${PROXY_PORT}"

echo "╔════════════════════════════════════════════╗"
echo "║  Claude Proxy - B Computer Setup           ║"
echo "╚════════════════════════════════════════════╝"
echo ""

# Detect shell
SHELL_NAME=$(basename "$SHELL")
RC_FILE=""

case "$SHELL_NAME" in
    bash)
        if [[ -f "$HOME/.bashrc" ]]; then
            RC_FILE="$HOME/.bashrc"
        elif [[ -f "$HOME/.bash_profile" ]]; then
            RC_FILE="$HOME/.bash_profile"
        fi
        ;;
    zsh)
        RC_FILE="$HOME/.zshrc"
        ;;
    *)
        echo "Unsupported shell: $SHELL_NAME"
        echo "Please manually add the following to your shell profile:"
        echo ""
        echo "  export HTTP_PROXY=$PROXY_URL"
        echo "  export HTTPS_PROXY=$PROXY_URL"
        echo "  export http_proxy=$PROXY_URL"
        echo "  export https_proxy=$PROXY_URL"
        exit 1
        ;;
esac

echo "Detected shell: $SHELL_NAME"
echo "Config file: $RC_FILE"
echo "Proxy port: $PROXY_PORT"
echo ""

# Check if already configured
if grep -q "CLAUDE_PROXY_CONFIG" "$RC_FILE" 2>/dev/null; then
    echo "Proxy already configured in $RC_FILE"
    echo "To update, first run: $0 --remove"
    exit 0
fi

# Handle --remove flag
if [[ "$1" == "--remove" ]]; then
    if [[ -f "$RC_FILE" ]]; then
        sed -i.bak '/# CLAUDE_PROXY_CONFIG START/,/# CLAUDE_PROXY_CONFIG END/d' "$RC_FILE"
        echo "Removed proxy configuration from $RC_FILE"
        echo "Please run: source $RC_FILE"
    fi
    exit 0
fi

# Add proxy configuration
echo "Adding proxy configuration..."

cat >> "$RC_FILE" << EOF

# CLAUDE_PROXY_CONFIG START
# Claude Proxy configuration - added by setup-b.sh
export HTTP_PROXY=$PROXY_URL
export HTTPS_PROXY=$PROXY_URL
export http_proxy=$PROXY_URL
export https_proxy=$PROXY_URL

# Function to enable/disable proxy
claude-proxy-on() {
    export HTTP_PROXY=$PROXY_URL
    export HTTPS_PROXY=$PROXY_URL
    export http_proxy=$PROXY_URL
    export https_proxy=$PROXY_URL
    echo "Claude proxy enabled: $PROXY_URL"
}

claude-proxy-off() {
    unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy
    echo "Claude proxy disabled"
}

claude-proxy-status() {
    if [[ -n "\$HTTPS_PROXY" ]]; then
        echo "Claude proxy: ON (\$HTTPS_PROXY)"
        # Test connection
        if curl -s --connect-timeout 2 -x "\$HTTPS_PROXY" https://api.anthropic.com > /dev/null 2>&1; then
            echo "Connection: OK"
        else
            echo "Connection: FAILED (is A computer running?)"
        fi
    else
        echo "Claude proxy: OFF"
    fi
}
# CLAUDE_PROXY_CONFIG END
EOF

echo ""
echo "✓ Configuration added successfully!"
echo ""
echo "To activate now, run:"
echo "  source $RC_FILE"
echo ""
echo "Useful commands:"
echo "  claude-proxy-on     - Enable proxy"
echo "  claude-proxy-off    - Disable proxy"
echo "  claude-proxy-status - Check proxy status"
echo ""
echo "To test the connection:"
echo "  curl -x http://127.0.0.1:$PROXY_PORT https://api.anthropic.com/v1/messages -v"
echo ""
echo "To use Claude Code:"
echo "  claude"
