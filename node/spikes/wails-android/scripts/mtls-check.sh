#!/usr/bin/env bash
# Desktop side of check 5: reach the phone's mutual-TLS listener with curl.
#
#   scripts/mtls-check.sh [host[:port]]
#
# - Pulls the client certificate and the CA from the (debug) app via adb run-as.
# - With no argument it uses `adb forward` to reach the listener (works with an
#   emulator or a USB-connected phone). With host[:port] it connects to the phone
#   over the LAN (port default 47890); the device IP is shown in the app's header.
# - Also proves that a client WITHOUT a certificate is rejected.
set -euo pipefail

PKG=com.wails.app
PORT=47890
HOSTARG=${1:-}
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

for f in ca.pem client.pem client.key; do
  adb exec-out run-as "$PKG" cat "files/pki/$f" >"$dir/$f"
done

if [ -z "$HOSTARG" ]; then
  adb forward "tcp:$PORT" "tcp:$PORT" >/dev/null
  host=127.0.0.1
else
  host=${HOSTARG%%:*}
  [[ "$HOSTARG" == *:* ]] && PORT=${HOSTARG##*:}
fi

# The server certificate has the device IPs and dory-spike.local as SANs; verify
# against dory-spike.local so the check also works through the adb forward.
url="https://dory-spike.local:$PORT/ping"
resolve=(--resolve "dory-spike.local:$PORT:$host")

echo "== with client certificate (expect: dory-spike mtls ok client=dory-spike-client)"
curl -sS --fail --max-time 5 "${resolve[@]}" --cacert "$dir/ca.pem" \
  --cert "$dir/client.pem" --key "$dir/client.key" "$url"

echo "== without client certificate (expect: failure)"
if curl -sS --max-time 5 "${resolve[@]}" --cacert "$dir/ca.pem" "$url" 2>&1; then
  echo "UNEXPECTED: server answered without a client certificate" >&2
  exit 1
fi
echo "OK: mutual TLS enforced"
