#!/bin/bash

# MCP Proxy Go Build Script
# 
# Builds optimized binaries for multiple platforms
# Based on the original mcp-proxy TypeScript implementation:
# https://github.com/punkpeye/mcp-proxy

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Building optimized MCP Proxy binaries...${NC}"

# Create bin directory
mkdir -p bin

# Get version info
if git describe --tags --exact-match 2>/dev/null; then
    VERSION=$(git describe --tags --exact-match)
else
    VERSION=$(git describe --tags --always --dirty)
fi

BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT_SHA=$(git rev-parse --short HEAD)

echo -e "${YELLOW}Version: ${VERSION}${NC}"
echo -e "${YELLOW}Build Time: ${BUILD_TIME}${NC}"
echo -e "${YELLOW}Commit: ${COMMIT_SHA}${NC}"

# Optimized build flags
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.CommitSHA=${COMMIT_SHA}"

# Define platforms
declare -A platforms=(
    ["windows/amd64"]="mcp-proxy-windows-amd64.exe"
    ["windows/arm64"]="mcp-proxy-windows-arm64.exe"
    ["linux/amd64"]="mcp-proxy-linux-amd64"
    ["linux/arm64"]="mcp-proxy-linux-arm64"
    ["darwin/amd64"]="mcp-proxy-darwin-amd64"
    ["darwin/arm64"]="mcp-proxy-darwin-arm64"
)

# Build for each platform
for platform in "${!platforms[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "$platform"
    output_name="${platforms[$platform]}"
    
    echo -e "${YELLOW}Building for ${GOOS}/${GOARCH}...${NC}"
    
    env CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -a -installsuffix cgo \
        -ldflags="${LDFLAGS}" \
        -o "bin/${output_name}" \
        ./cmd/mcp-proxy
    
    if [ $? -eq 0 ]; then
        size=$(du -h "bin/${output_name}" | cut -f1)
        echo -e "${GREEN}✅ ${output_name} (${size})${NC}"
    else
        echo -e "${RED}❌ Failed to build for ${GOOS}/${GOARCH}${NC}"
        exit 1
    fi
done

echo -e "\n${GREEN}📦 Build complete! Files in bin/:${NC}"
ls -lah bin/

echo -e "\n${GREEN}🔍 Generating checksums...${NC}"
cd bin
sha256sum * > checksums.txt
echo -e "${YELLOW}Checksums saved to bin/checksums.txt${NC}"

echo -e "\n${GREEN}✨ Optimized binaries ready for distribution!${NC}"

# Optional: Check if UPX is available for further compression
if command -v upx >/dev/null 2>&1; then
    echo -e "\n${YELLOW}🗜️  UPX found! Compressing binaries...${NC}"
    for file in mcp-proxy-*; do
        if [[ ! "$file" == *.txt ]]; then
            echo -e "${YELLOW}Compressing ${file}...${NC}"
            upx --best --quiet "$file" 2>/dev/null || echo -e "${YELLOW}⚠️  Could not compress ${file}${NC}"
        fi
    done
    echo -e "${GREEN}🗜️  Compression complete!${NC}"
    ls -lah
else
    echo -e "\n${YELLOW}💡 Install UPX for even smaller binaries: https://upx.github.io/${NC}"
fi