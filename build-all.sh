#!/usr/bin/env bash
# Cross-platform build script for greg
# Builds binaries for all major platforms and architectures

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BINARY_NAME="greg"
OUTPUT_DIR="dist"
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")

# Build flags
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}"

# Options
COMPRESS=${COMPRESS:-"yes"}  # Set COMPRESS=no to skip compression
UPX_COMPRESS=${UPX_COMPRESS:-"no"}  # Set UPX_COMPRESS=yes to use UPX (requires upx installed)

# Platform/Architecture combinations
# Format: "GOOS:GOARCH:output_suffix"
PLATFORMS=(
    "linux:amd64:linux-amd64"
    "linux:arm64:linux-arm64"
    "linux:386:linux-386"
    "darwin:amd64:macos-amd64"
    "darwin:arm64:macos-arm64"
    "windows:amd64:windows-amd64.exe"
    "windows:386:windows-386.exe"
    "freebsd:amd64:freebsd-amd64"
    "openbsd:amd64:openbsd-amd64"
)

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}   greg - Cross-Platform Build Script${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${GREEN}Version:${NC} ${VERSION}"
echo -e "${GREEN}Commit:${NC}  ${COMMIT}"
echo -e "${GREEN}Date:${NC}    ${BUILD_DATE}"
echo ""

# Clean and create output directory
echo -e "${YELLOW}→${NC} Cleaning ${OUTPUT_DIR}..."
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# Build counter
SUCCESS_COUNT=0
FAIL_COUNT=0
TOTAL=${#PLATFORMS[@]}

echo -e "${YELLOW}→${NC} Building ${TOTAL} binaries..."
echo ""

# Build for each platform
for platform in "${PLATFORMS[@]}"; do
    # Split platform string
    IFS=':' read -r GOOS GOARCH OUTPUT_SUFFIX <<< "$platform"
    
    OUTPUT_PATH="${OUTPUT_DIR}/${BINARY_NAME}-${OUTPUT_SUFFIX}"
    
    printf "  %-20s " "${GOOS}/${GOARCH}"
    
    # Build
    if GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build \
        -ldflags="${LDFLAGS}" \
        -o "${OUTPUT_PATH}" \
        ./cmd/greg 2>/dev/null; then
        
        # Get file size (cross-platform)
        SIZE=$(ls -lh "${OUTPUT_PATH}" | awk '{print $5}')
        
        echo -e "${GREEN}✓${NC} ${SIZE}"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo -e "${RED}✗ Failed${NC}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✓ Build Complete${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${GREEN}Success:${NC} ${SUCCESS_COUNT}/${TOTAL}"
if [ $FAIL_COUNT -gt 0 ]; then
    echo -e "${RED}Failed:${NC}  ${FAIL_COUNT}/${TOTAL}"
fi
echo ""
echo -e "${YELLOW}Output directory:${NC} ${OUTPUT_DIR}/"
echo ""

# List all built binaries
echo "Built binaries:"
ls -lh "${OUTPUT_DIR}" | tail -n +2 | awk '{printf "  %-30s %8s\n", $9, $5}'

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Optional: UPX compression
if [ "$UPX_COMPRESS" = "yes" ] && command -v upx &> /dev/null; then
    echo ""
    echo -e "${YELLOW}→${NC} Compressing binaries with UPX..."
    cd "${OUTPUT_DIR}"
    for binary in greg-*; do
        if [ -f "$binary" ]; then
            echo -n "  $binary... "
            if upx --best --lzma "$binary" &> /dev/null; then
                SIZE=$(ls -lh "$binary" | awk '{print $5}')
                echo -e "${GREEN}✓${NC} ${SIZE}"
            else
                echo -e "${RED}✗ Failed${NC}"
            fi
        fi
    done
    cd - > /dev/null
    echo ""
fi

# Optional: Create checksums
if command -v sha256sum &> /dev/null || command -v shasum &> /dev/null; then
    echo ""
    echo -e "${YELLOW}→${NC} Generating checksums..."
    
    cd "${OUTPUT_DIR}"
    if command -v sha256sum &> /dev/null; then
        sha256sum greg-* 2>/dev/null > checksums.txt || true
    else
        shasum -a 256 greg-* 2>/dev/null > checksums.txt || true
    fi
    cd - > /dev/null
    
    echo -e "${GREEN}✓${NC} Checksums saved to ${OUTPUT_DIR}/checksums.txt"
fi

# Create compressed archives for easier sharing
if [ "$COMPRESS" = "yes" ]; then
    echo ""
    echo -e "${YELLOW}→${NC} Creating compressed archives (for Discord/sharing)..."
    cd "${OUTPUT_DIR}"
    
    ARCHIVE_COUNT=0
    for binary in greg-*; do
        if [ -f "$binary" ] && [ "$binary" != "checksums.txt" ]; then
            # Create archive name (remove greg- prefix)
            ARCHIVE_NAME="${binary}.tar.gz"
            
            # Compress
            if tar -czf "$ARCHIVE_NAME" "$binary" 2>/dev/null; then
                SIZE=$(ls -lh "$ARCHIVE_NAME" | awk '{print $5}')
                ARCHIVE_COUNT=$((ARCHIVE_COUNT + 1))
                
                # Check if under 10MB for Discord
                SIZE_BYTES=$(ls -l "$ARCHIVE_NAME" | awk '{print $5}')
                if [ "$SIZE_BYTES" -lt 10485760 ]; then
                    echo -e "  ${ARCHIVE_NAME} ${GREEN}${SIZE}${NC} ${GREEN}✓ Discord-ready${NC}"
                else
                    echo -e "  ${ARCHIVE_NAME} ${YELLOW}${SIZE}${NC} ${YELLOW}⚠ >10MB${NC}"
                fi
            fi
        fi
    done
    
    cd - > /dev/null
    echo -e "${GREEN}✓${NC} Created ${ARCHIVE_COUNT} compressed archives"
fi

echo ""
echo -e "${GREEN}Done!${NC} Ready to share: ${OUTPUT_DIR}/"
if [ "$COMPRESS" = "yes" ]; then
    echo ""
    echo -e "${BLUE}Tip:${NC} Share the .tar.gz files for easier Discord/Telegram uploads"
    echo -e "${BLUE}Tip:${NC} Users can extract with: tar -xzf greg-*.tar.gz"
fi
echo ""
