#!/bin/bash
# tag-release.sh: Create a git tag and push to origin to trigger CI/CD release
# Usage: ./tag-release.sh <version>

set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

VERSION="$1"

git tag "$VERSION"
git push origin "$VERSION"

echo "Tag $VERSION created and pushed to origin."
