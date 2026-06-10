#!/bin/bash

# Extract version from manage_telegram.sh or fallback to a default
VERSION=$(grep "^VERSION=" "$(dirname "$0")/manage_telegram.sh" | cut -d'"' -f2)

if [ -z "$VERSION" ]; then
    VERSION="1.1.9"
fi

echo "$VERSION"
