#!/bin/bash
# Wrapper script for manage-sites.sh compatibility
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/manage_telegram.sh" "$@"
