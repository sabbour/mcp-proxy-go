#!/bin/bash
# build-docker.sh: Build the MCP proxy Docker image locally
# Usage: ./build-docker.sh [tag]

set -e

IMAGE_NAME="mcp-proxy-go"
TAG="${1:-latest}"
DOCKERFILE="Dockerfile"

# Build the Docker image

echo "Building Docker image: $IMAGE_NAME:$TAG"
docker build -f "$DOCKERFILE" -t "$IMAGE_NAME:$TAG" .

echo "Docker image built: $IMAGE_NAME:$TAG"
