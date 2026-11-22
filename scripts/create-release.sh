#!/usr/bin/env bash
# Creates the shared release of this run. Platforms upload into it as they
# finish, so a fast build is available without waiting for the others.
set -euo pipefail
tag=$(bash "$(dirname "$0")/release-tag.sh")
if gh release view "$tag" >/dev/null 2>&1; then exit 0; fi
if [ "$GITHUB_REF_TYPE" = tag ]; then
  gh release create "$tag" --verify-tag --title "$tag" --generate-notes
else
  gh release create "$tag" --target "$GITHUB_SHA" --prerelease --title "Nightly $GITHUB_RUN_NUMBER · $(date -u +%F)" \
    --notes "Compilación nightly del commit $GITHUB_SHA. Cada plataforma añade sus archivos al terminar."
fi
