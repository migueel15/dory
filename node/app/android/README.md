# dory-android

The Dory local-first node (localstore + syncx + peer, no T3 and no GitHub) as an Android app, built with Wails v3.1.0.
Same SPA (`web/`) plus the `/node/` pages as the desktop app. T3 calls are forwarded to the owner device (`nodeapp`).

## How the UI reaches the node (decision)

Wails' in-process asset server on Android drops POST bodies and query strings and turns any non-200 into "not found"
(see `../../spikes/wails-android/README.md`), so the SPA cannot run through it. Instead:

1. The webview opens `https://wails.localhost/`, served by Wails: only `frontend/dist/index.html`, a bootstrap page.
2. The page asks the Java side (`DoryNative`) for the network interfaces (Go cannot list them on Android 11+), takes the
   multicast lock, and calls `GET /boot/<base64url interfaces>` (data in the path, which survives).
3. Go replies with the address of `nodeapp.Shell`: an HTTP listener on `127.0.0.1:<random port>`. The page does
   `location.replace(url)`; from then on the SPA and `/node/` run over real HTTP (POST, queries, redirects, SSE).

Any app on the phone can connect to a loopback port, so the shell rejects everything without the `dory_shell` cookie.
The cookie is set only by `/__enter?t=<token>`, whose random token lives in the process and in the bootstrap reply.
Behind the gate the node answers as the local user (`Node.Handler`), as on the desktop. Cleartext is allowed only for
`127.0.0.1` (`network_security_config.xml`); `allowBackup` is off because the identity key lives in `files/`.

Not served by the in-process handler on purpose: nothing else reaches `wails.localhost`.

## First start

No `group.json` yet: the shell serves only `/node/join.html` ("Unirse a un grupo"). It scans the desktop QR with the
camera (`BarcodeDetector`, jsQR fallback; the webview grants the camera only to `127.0.0.1`/`wails.localhost`) or takes
the pasted payload, then runs `peer.Join` + `SaveGroup` + `ApplySnapshot` and starts the node. The desktop accepts the
device on its confirmation prompt. Next starts go straight to the SPA.

## Offline, resume, Tailscale

- Everything reads the local SQLite replica; `peer.Connector` retries with backoff and syncs when a peer is reachable.
- On `onResume` and on network changes the Java side POSTs the current interfaces to `/__net` (port and gate secret in
  `files/shell.json`, 0600): the node re-announces over mDNS, republishes its address and `Connector.Kick()` wakes the
  retry loops at once. The Go `ActivityResumed` event also kicks.
- With the official Tailscale app active the desktop's tailnet name/IP is the `tailnet` address in its replicated record;
  `Connector.Candidates` already tries it in parallel with LAN, mDNS and manual addresses. Not tested on a real tailnet.
- A foreground service (`dataSync`) keeps the process and the mTLS listener (port 7710) alive in the background.

## Build

```sh
export PATH=$PATH:~/go/bin ANDROID_HOME=$HOME/Android/Sdk      # wails3 v3.1.0, NDK 26.3
cd node/app/android
(cd ../../../web && npm ci && npm run build)                    # the node embeds web/dist
rm -rf build/android/app/src/main/jniLibs                       # when switching ABI
wails3 task android:build PRODUCTION=true ARCH=arm64 && wails3 task android:assemble:apk:release   # phone
wails3 task android:build ARCH=amd64 && wails3 task android:assemble:apk                           # emulator, debug
adb install -r bin/dory-android.apk                             # applicationId net.dorlab.dory
```

The release APK is signed with the debug key unless `ANDROID_KEYSTORE_FILE` is set. `bin/`, `*.so`, `*.apk` are ignored.

## Known limits

- x86_64 emulator: opening SQLite kills the process with SIGSYS (modernc `lstat`, blocked by seccomp), so after joining the
  node cannot start there. The join screen, camera path and bootstrap do work. Needs an arm64 phone.
- The pairing QR shown by the phone keeps the addresses of the first boot; mDNS and the address record follow network changes.
