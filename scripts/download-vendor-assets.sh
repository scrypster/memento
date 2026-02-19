#!/bin/bash
set -e

echo "Downloading vendor assets for Memento Web UI..."

# Create directories
mkdir -p web/static/vendor

VENDOR_DIR="web/static/vendor"

# Helper: download and verify a vendor file
download_vendor() {
  local name="$1"
  local url="$2"
  local file="$3"
  local min_size="${4:-1000}"

  echo "Downloading $name..."
  curl -sL -o "$VENDOR_DIR/$file" "$url"

  if [ ! -f "$VENDOR_DIR/$file" ]; then
    echo "ERROR: Failed to download $name"
    exit 1
  fi

  FILE_SIZE=$(wc -c < "$VENDOR_DIR/$file")
  if [ "$FILE_SIZE" -lt "$min_size" ]; then
    echo "ERROR: $name file is too small ($FILE_SIZE bytes), download may have failed"
    exit 1
  fi

  echo "✓ $name downloaded ($FILE_SIZE bytes)"
}

# Alpine.js 3.14.9
download_vendor "Alpine.js 3.14.9" \
  "https://cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js" \
  "alpine-3.14.9.min.js" 10000

# Cytoscape.js 3.30.4
download_vendor "Cytoscape.js 3.30.4" \
  "https://cdn.jsdelivr.net/npm/cytoscape@3.30.4/dist/cytoscape.min.js" \
  "cytoscape-3.30.4.min.js" 100000

# layout-base 2.0.1 (dependency for cose-base / fcose)
download_vendor "layout-base 2.0.1" \
  "https://unpkg.com/layout-base@2.0.1/layout-base.js" \
  "layout-base-2.0.1.js" 1000

# cose-base 2.2.0 (dependency for fcose)
download_vendor "cose-base 2.2.0" \
  "https://unpkg.com/cose-base@2.2.0/cose-base.js" \
  "cose-base-2.2.0.js" 1000

# cytoscape-fcose 2.2.0
download_vendor "cytoscape-fcose 2.2.0" \
  "https://unpkg.com/cytoscape-fcose@2.2.0/cytoscape-fcose.js" \
  "cytoscape-fcose-2.2.0.js" 1000

# Chart.js 4.4.0
download_vendor "Chart.js 4.4.0" \
  "https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js" \
  "chart-4.4.0.min.js" 50000

echo ""
echo "All vendor assets ready!"
echo "  - Alpine.js:        $VENDOR_DIR/alpine-3.14.9.min.js"
echo "  - Cytoscape.js:     $VENDOR_DIR/cytoscape-3.30.4.min.js"
echo "  - layout-base:      $VENDOR_DIR/layout-base-2.0.1.js"
echo "  - cose-base:        $VENDOR_DIR/cose-base-2.2.0.js"
echo "  - cytoscape-fcose:  $VENDOR_DIR/cytoscape-fcose-2.2.0.js"
echo "  - Chart.js:         $VENDOR_DIR/chart-4.4.0.min.js"
