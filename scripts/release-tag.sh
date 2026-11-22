#!/usr/bin/env bash
# Prints the release tag of this workflow run: the version tag for stable and
# one nightly per run otherwise.
set -euo pipefail
if [ "$GITHUB_REF_TYPE" = tag ]; then echo "$GITHUB_REF_NAME"; else echo "nightly-$GITHUB_RUN_NUMBER"; fi
