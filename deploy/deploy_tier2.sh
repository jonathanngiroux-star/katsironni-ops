#!/bin/bash
# Deploy MCP servers — Tier 2 (9 services)
set -euo pipefail

MCP_DIR="/opt/katsironni/mcp"
VENV="$MCP_DIR/.venv"
GH="jonathanngiroux-star"

TIER2=(
    "mcp-deliverability emailops"
    "mcp-flagstrategy flagstrategy"
    "mcp-airdroppremium airdroppremium"
    "mcp-proofspan agentbench"
    "mcp-switchyard flagstrategy-x"
    "mcp-warmline emailops-x"
    "mcp-oneprobe nodeops"
    "mcp-walletrisk walletguard"
    "tokenomics tokenomics"
)

for entry in "${TIER2[@]}"; do
    REPO=$(echo "$entry" | cut -d' ' -f1)
    KEY=$(echo "$entry" | cut -d' ' -f2)
    # skip duplicates (some repos map to already-live services)
    case "$KEY" in
        *-x) continue ;;  # skip alternate mappings
    esac
    DIR="$MCP_DIR/$REPO"
    echo "=== $REPO ($KEY) ==="

    if [ ! -d "$DIR" ]; then
        git clone --quiet "https://github.com/$GH/$REPO.git" "$DIR"
        echo "  cloned"
    else
        echo "  already cloned"
    fi

    if [ -f "$DIR/requirements.txt" ]; then
        "$VENV/bin/pip" install --quiet -r "$DIR/requirements.txt" 2>/dev/null || true
    fi

    cd "$DIR"
    if "$VENV/bin/python" -c "import main" 2>/dev/null; then
        echo "  imports OK"
    else
        echo "  WARNING: import failed — checking error..."
        "$VENV/bin/python" -c "import main" 2>&1 | tail -3
        # try common fixes
        sed -i 's/"default": true/"default": True/g; s/"default": false/"default": False/g' "$DIR/main.py" 2>/dev/null
        if "$VENV/bin/python" -c "import main" 2>/dev/null; then
            echo "  fixed + imports OK"
        else
            echo "  SKIPPING (import still fails)"
            continue
        fi
    fi

    PROFILE="$KEY-bot"
    PROFILE_DIR="/opt/katsironni/.hermes/profiles/$PROFILE"
    if [ ! -d "$PROFILE_DIR" ]; then
        runuser -u katsironni -- bash -lc "hermes profile create '$PROFILE' 2>/dev/null" || true
        echo "  profile created"
    fi

    grep -E "^(NVIDIA_API_KEY|OPENROUTER_API_KEY)=" /opt/katsironni/.hermes/.env \
        >> "$PROFILE_DIR/.env" 2>/dev/null || true
    chmod 600 "$PROFILE_DIR/.env" 2>/dev/null || true

    if ! grep -q "mcp_servers" "$PROFILE_DIR/config.yaml" 2>/dev/null; then
        cat >> "$PROFILE_DIR/config.yaml" << EOF
mcp_servers:
  $KEY:
    command: $VENV/bin/python
    args: ["$DIR/main.py"]
    timeout: 120
EOF
        echo "  MCP server configured"
    fi

    chown -R katsironni:katsironni "$PROFILE_DIR"
    echo "  $REPO DONE"
done

echo ""
echo "=== TIER 2 DEPLOYMENT COMPLETE ==="
