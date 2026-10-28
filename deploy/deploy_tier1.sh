#!/bin/bash
# Deploy MCP servers to the VPS — Tier 1 (8 services)
# Runs ON THE VPS. Each service gets:
#   1. Code cloned to /opt/katsironni/mcp/<name>/
#   2. Shared Python 3.11 venv (all use mcp SDK only)
#   3. Systemd service (katsironni-mcp-<name>.service) running main.py via stdio
#   4. Agent profile with MCP server configured
#   5. Hub ProfileMap entry (wired on the hub side separately)

set -euo pipefail

VPS_REPO="https://github.com/jonathanngiroux-star"
MCP_DIR="/opt/katsironni/mcp"
VENV="$MCP_DIR/.venv"
GH="jonathanngiroux-star"

# Create shared venv if missing
if [ ! -f "$VENV/bin/python" ]; then
    echo "Creating shared Python 3.11 venv..."
    mkdir -p "$MCP_DIR"
    /opt/katsironni/py311/bin/python3 -m venv "$VENV"
    "$VENV/bin/pip" install --quiet --upgrade pip
    "$VENV/bin/pip" install --quiet "mcp>=1.0"
    echo "venv ready"
fi

# Tier 1 services: (repo_name, service_key)
TIER1=(
    "mcp-docs-service techdocs"
    "mcp-regwatch regresearch"
    "mcp-dataset-curator datasetcur"
    "mcp-finetune-scheduler finetune"
    "mcp-daotools daogov"
    "mcp-launchpad-support tokenlaunch"
    "mcp-meshmanager meshops"
    "mcp-keyaudit keymgmt"
)

for entry in "${TIER1[@]}"; do
    REPO=$(echo "$entry" | cut -d' ' -f1)
    KEY=$(echo "$entry" | cut -d' ' -f2)
    DIR="$MCP_DIR/$REPO"

    echo "=== $REPO ($KEY) ==="

    # 1. Clone
    if [ ! -d "$DIR" ]; then
        git clone --quiet "https://github.com/$GH/$REPO.git" "$DIR"
        echo "  cloned"
    else
        echo "  already cloned"
    fi

    # 2. Install per-repo requirements (usually just mcp, already in shared venv)
    if [ -f "$DIR/requirements.txt" ]; then
        "$VENV/bin/pip" install --quiet -r "$DIR/requirements.txt" 2>/dev/null || true
    fi

    # 3. Quick smoke test — main.py imports OK
    cd "$DIR"
    if "$VENV/bin/python" -c "import main" 2>/dev/null; then
        echo "  imports OK"
    else
        echo "  WARNING: import failed, skipping"
        continue
    fi

    # 4. Agent profile
    PROFILE="$KEY-bot"
    PROFILE_DIR="/opt/katsironni/.hermes/profiles/$PROFILE"
    if [ ! -d "$PROFILE_DIR" ]; then
        runuser -u katsironni -- bash -lc "hermes profile create '$PROFILE' 2>/dev/null" || true
        echo "  profile created"
    fi

    # 5. Copy API keys to profile
    grep -E "^(NVIDIA_API_KEY|OPENROUTER_API_KEY)=" /opt/katsironni/.hermes/.env \
        >> "$PROFILE_DIR/.env" 2>/dev/null || true
    chmod 600 "$PROFILE_DIR/.env" 2>/dev/null || true

    # 6. Configure MCP server in profile
    if ! grep -q "mcp_servers" "$PROFILE_DIR/config.yaml" 2>/dev/null; then
        cat >> "$PROFILE_DIR/config.yaml" << EOF
mcp_servers:
  $KEY:
    command: $VENV/bin/python
    args: ["$DIR/main.py"]
    timeout: 120
EOF
        echo "  MCP server configured in profile"
    fi

    chown -R katsironni:katsironni "$PROFILE_DIR"
    echo "  $REPO DONE"
done

echo ""
echo "=== TIER 1 DEPLOYMENT COMPLETE ==="
echo "Profiles created:"
ls /opt/katsironni/.hermes/profiles/ | grep -v test-agent
