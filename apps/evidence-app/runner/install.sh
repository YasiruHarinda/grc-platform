#!/bin/bash
# WSO2 Compliance Runner — one-command installer
# Usage: bash install.sh
#
# NOTE: provisional. The packaging/distribution approach for the runner is not
# finalised yet (a pre-built binary bundle is under consideration), so this
# script may be replaced. Treat it as the current dev/local install path.

set -e

echo ""
echo "================================================"
echo "  WSO2 Compliance Runner — Installer"
echo "================================================"
echo ""

# 1. Check for a Python >= 3.11 interpreter
PYTHON_CMD=""
for candidate in python3.11 python3.12 python3.13 python3 python; do
    if command -v "$candidate" &>/dev/null && "$candidate" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 11) else 1)' &>/dev/null; then
        PYTHON_CMD="$candidate"
        break
    fi
done
if [ -z "$PYTHON_CMD" ]; then
    echo "ERROR: Python >= 3.11 not found."
    echo "Install it first:"
    echo "  Ubuntu/Debian: sudo apt install python3.11 python3.11-venv -y"
    echo "  macOS:         brew install python@3.11"
    exit 1
fi
echo "[1/4] Python found: $("$PYTHON_CMD" --version) ($PYTHON_CMD)"

# 2. Create venv in ~/.wso2-runner/venv
VENV_DIR="$HOME/.wso2-runner/venv"
echo "[2/4] Creating virtual environment at $VENV_DIR ..."
"$PYTHON_CMD" -m venv "$VENV_DIR"

# 3. Install the runner package
echo "[3/4] Installing wso2-runner ..."
"$VENV_DIR/bin/pip" install --quiet --upgrade pip
"$VENV_DIR/bin/pip" install --quiet -e "$(dirname "$0")"

# 4. Install Chromium
echo "[4/4] Installing Chromium browser for the agent ..."
"$VENV_DIR/bin/python" -m playwright install chromium

# 5. Create a launcher script so user can just type 'wso2-runner' from anywhere
LAUNCHER="$HOME/.local/bin/wso2-runner"
mkdir -p "$HOME/.local/bin"
cat > "$LAUNCHER" <<EOF
#!/bin/bash
source "$VENV_DIR/bin/activate"
exec wso2-runner "\$@"
EOF
chmod +x "$LAUNCHER"

echo ""
echo "================================================"
echo "  Installation complete!"
echo "================================================"
echo ""
echo "Next step — run the setup wizard:"
echo ""
echo "  wso2-runner configure"
echo ""
echo "  (If 'wso2-runner' is not found, add ~/.local/bin to your PATH:"
echo "     export PATH=\"\$HOME/.local/bin:\$PATH\"   — then re-open your terminal,"
echo "     or add that line to your ~/.bashrc to make it permanent.)"
echo ""
