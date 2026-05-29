#!/bin/bash

# Build script for GoFileBeam
# This script copies static files and builds the binary with embedded assets

set -e

echo "🔨 Building GoFileBeam..."

# Copy static files for embedding
echo "📁 Copying static files..."
rm -rf internal/static/files
cp -r static internal/static/files

# Build the binary
echo "🏗️  Compiling binary..."
go build -ldflags="-w -s" -o gofilebeam ./cmd/gofilebeam

echo "✅ Build complete! Binary: ./gofilebeam"
echo ""
echo "The binary is now portable and includes all web UI files."
echo "You can run it from any directory with: ./gofilebeam"
