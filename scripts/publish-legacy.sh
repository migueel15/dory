#!/usr/bin/env bash
set -euo pipefail
api="repos/$GITHUB_REPOSITORY"
if [ "$GITHUB_REF_TYPE" = tag ]; then
  # Old stable clients continue to receive complete stable releases. Drafts
  # are hidden until all assets have been uploaded.
  if gh release view "$GITHUB_REF_NAME" --json isDraft --jq .isDraft > "$RUNNER_TEMP/legacy-draft" 2>/dev/null; then
    [ "$(cat "$RUNNER_TEMP/legacy-draft")" = true ] || exit 0
    gh release upload "$GITHUB_REF_NAME" assets/* --clobber
  else
    gh release create "$GITHUB_REF_NAME" assets/* --draft --title "$GITHUB_REF_NAME" --generate-notes
  fi
  gh release edit "$GITHUB_REF_NAME" --draft=false
  exit
fi
# Keep a complete transition release for old nightly clients, then freeze it.
# It contains the new updater, which will use the per-platform manifests.
marker=updates-legacy-nightly
if gh api "$api/git/ref/heads/$marker" >/dev/null 2>&1; then exit 0; fi
current=$(gh api "$api/commits/main" --jq .sha)
if [ "$current" != "$GITHUB_SHA" ]; then exit 0; fi
if gh release view nightly >/dev/null 2>&1; then
  gh release delete nightly --yes --cleanup-tag
fi
gh release create nightly assets/* --target "$GITHUB_SHA" --draft --prerelease --title Nightly --notes "Versión de transición para el actualizador por plataforma."
gh release edit nightly --draft=false
gh api --method POST "$api/git/refs" -f "ref=refs/heads/$marker" -f "sha=$GITHUB_SHA" >/dev/null
