#!/bin/bash
#
# Install git hooks for this repository
# This script creates symlinks from .git/hooks to scripts/git-hooks/
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOOKS_DIR="$(git rev-parse --git-dir)/hooks"

echo "Installing git hooks..."
echo ""

# List of hooks to install
HOOKS=("pre-push")

for HOOK in "${HOOKS[@]}"; do
    TARGET="${SCRIPT_DIR}/git-hooks/${HOOK}"
    LINK="${HOOKS_DIR}/${HOOK}"
    
    if [ -f "$LINK" ] && [ ! -L "$LINK" ]; then
        echo "Backup existing ${HOOK} to ${HOOK}.backup"
        mv "$LINK" "${LINK}.backup"
    fi
    
    # Create relative symlink
    REL_PATH=$(realpath --relative-to="$HOOKS_DIR" "$TARGET")
    ln -sf "$REL_PATH" "$LINK"
    
    echo "✓ Installed ${HOOK}"
done

echo ""
echo "Git hooks installed successfully!"
echo ""
echo "Pre-push: Runs full checks (format, lint, tests, build)"
echo ""
echo "To bypass hooks temporarily, use:"
echo "  git commit --no-verify"
echo "  git push --no-verify"
