#!/usr/bin/env bash
set -euo pipefail
channel=nightly
[ "$GITHUB_REF_TYPE" = tag ] && channel=stable
branch="updates-$channel-$PLATFORM"
tag="build-$channel-$PLATFORM-$GITHUB_RUN_NUMBER-$GITHUB_RUN_ATTEMPT"
api="repos/$GITHUB_REPOSITORY"
# A completed release is immutable. Advancing the single manifest is the
# commit point; a failed build or upload leaves the previous manifest intact.
if gh api "$api/contents/manifest.json?ref=$branch" > "$RUNNER_TEMP/previous.json" 2>/dev/null; then
  status=0
  node -e 'const f=require(process.argv[1]); const m=JSON.parse(Buffer.from(f.content,"base64")); const [run,attempt,channel,version]=process.argv.slice(2); if(channel==="stable" && m.version) { const semver=require("./desktop/node_modules/semver"); if(semver.gt(m.version,version)) process.exit(2); if(semver.gt(version,m.version)) process.exit(0); } if(Number(m.run)>Number(run) || (Number(m.run)===Number(run) && Number(m.attempt)>=Number(attempt))) process.exit(2)' "$RUNNER_TEMP/previous.json" "$GITHUB_RUN_NUMBER" "$GITHUB_RUN_ATTEMPT" "$channel" "${GITHUB_REF_NAME#v}" || status=$?
  if [ "$status" -eq 2 ]; then echo 'Publicación más reciente ya disponible'; exit 0; fi
  if [ "$status" -ne 0 ]; then exit "$status"; fi
  old_sha=$(node -p 'require(process.argv[1]).sha' "$RUNNER_TEMP/previous.json")
else
  if ! gh api "$api/git/ref/heads/$branch" >/dev/null 2>&1; then
    gh api --method POST "$api/git/refs" -f "ref=refs/heads/$branch" -f "sha=$GITHUB_SHA" >/dev/null
  fi
  old_sha=""
fi
# The branch without a manifest is harmless: no client can select this build.
gh release create "$tag" out/* --target "$GITHUB_SHA" --draft --prerelease --title "$channel $PLATFORM $GITHUB_RUN_NUMBER" --notes "Commit $GITHUB_SHA"
gh release edit "$tag" --draft=false
content=$(node -e 'process.stdout.write(Buffer.from(JSON.stringify({tag:process.argv[1],sha:process.argv[2],run:Number(process.argv[3]),attempt:Number(process.argv[4]),version:process.argv[5]})).toString("base64"))' "$tag" "$GITHUB_SHA" "$GITHUB_RUN_NUMBER" "$GITHUB_RUN_ATTEMPT" "${GITHUB_REF_NAME#v}")
args=(-f "message=Actualiza $channel $PLATFORM" -f "content=$content" -f "branch=$branch")
[ -n "$old_sha" ] && args+=(-f "sha=$old_sha")
gh api --method PUT "$api/contents/manifest.json" "${args[@]}" >/dev/null
