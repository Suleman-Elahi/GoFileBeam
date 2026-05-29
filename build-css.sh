#!/bin/bash

echo "Building production Tailwind CSS..."

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

# Build minified CSS
echo "Generating minified CSS..."
npm run build:css

echo "✓ CSS built successfully at static/css/output.css"
echo ""
echo "File size:"
ls -lh static/css/output.css | awk '{print $5, $9}'
