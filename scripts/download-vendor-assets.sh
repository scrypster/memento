#!/bin/bash
set -e

echo "Downloading vendor assets for Memento Web UI..."

# Create directories
mkdir -p web/static/vendor
mkdir -p web/static/css
mkdir -p web/static/js

# Download Alpine.js 3.14.9
echo "Downloading Alpine.js 3.14.9..."
curl -sL -o web/static/vendor/alpine-3.14.9.min.js \
  https://cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js

# Verify file was downloaded
if [ ! -f web/static/vendor/alpine-3.14.9.min.js ]; then
  echo "ERROR: Failed to download Alpine.js"
  exit 1
fi

FILE_SIZE=$(wc -c < web/static/vendor/alpine-3.14.9.min.js)
if [ "$FILE_SIZE" -lt 10000 ]; then
  echo "ERROR: Alpine.js file is too small ($FILE_SIZE bytes), download may have failed"
  exit 1
fi

echo "✓ Alpine.js downloaded successfully ($FILE_SIZE bytes)"

# Build Tailwind CSS (requires Node.js)
if command -v npx &> /dev/null; then
  echo "Building Tailwind CSS..."

  # Create minimal tailwind.config.js if it doesn't exist
  if [ ! -f tailwind.config.js ]; then
    cat > tailwind.config.js << 'EOF'
module.exports = {
  content: ["./web/templates/**/*.html", "./web/static/js/**/*.js"],
  theme: {
    extend: {},
  },
  plugins: [],
}
EOF
  fi

  # Create minimal input CSS
  cat > web/static/css/input.css << 'EOF'
@tailwind base;
@tailwind components;
@tailwind utilities;
EOF

  npx tailwindcss -i web/static/css/input.css -o web/static/css/tailwind.min.css --minify

  if [ -f web/static/css/tailwind.min.css ]; then
    echo "✓ Tailwind CSS built successfully"
  else
    echo "ERROR: Failed to build Tailwind CSS"
    exit 1
  fi
else
  echo "WARNING: npx not found, skipping Tailwind CSS build"
  echo "Install Node.js to enable Tailwind CSS generation"
fi

echo ""
echo "Vendor assets ready!"
echo "- Alpine.js: web/static/vendor/alpine-3.14.9.min.js"
echo "- Tailwind CSS: web/static/css/tailwind.min.css"
