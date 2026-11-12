# Spike: Wails v3 on Android

Test app with a checks screen (OK/ERROR per check) for the Dory local-first node.
Wails v3.1.0, Go 1.27, package/applicationId `com.wails.app` (Wails template default).

| # | Check | Where | Verified |
|---|-------|-------|----------|
| 1 | `http.Handler` in-process, no ports | `internal/checks/handler.go` | x86_64 emulator, desktop tests |
| 2 | SQLite (`modernc.org/sqlite`) write, close, reopen, reread in `getFilesDir()` | `internal/checks/sqlite.go` | desktop tests only (see findings) |
| 3 | QR from the webview camera (`BarcodeDetector`, fallback jsQR) + camera permission | `frontend/dist/app.js`, `MainActivity.java` | permission prompt + live stream on emulator; QR decode NOT verified |
| 4 | mDNS announce/discover `_dory._tcp` + multicast lock | `internal/checks/mdns.go`, `DoryNative.multicast` | emulator (self-discovery) |
| 5 | mutual-TLS listener `:47890`, reachable from desktop | `internal/checks/mtls.go`, `scripts/mtls-check.sh` | emulator via `adb forward` + curl |
| 6 | Stays alive in background | `internal/checks/background.go`, foreground service | 3 min on emulator with foreground service |

## Build (Linux, verified)

Requirements: Go, JDK (25 worked), Android SDK (`~/Android/Sdk`, platform 36, build-tools),
**NDK 26.3** (`sdkmanager --sdk_root=$HOME/Android/Sdk "ndk;26.3.11579264"`, ~15 s, 1 download),
`wails3` (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`, v3.1.0), `adb`.
Gradle 9.2 and the Android Gradle plugin are downloaded by `gradlew` on first run.

```sh
export PATH=$PATH:~/go/bin ANDROID_HOME=$HOME/Android/Sdk
cd node/spikes/wails-android
wails3 task android:build ARCH=arm64            # Go -> libwails.so (c-shared, NDK clang)
wails3 task android:assemble:apk                # debug APK -> bin/dory-spike.apk
# release (stripped, 12 MB): wails3 task android:build PRODUCTION=true ARCH=arm64 && wails3 task android:assemble:apk:release
# emulator on an x86_64 host: ARCH=amd64 (the "ARCH=amd64" build packs only x86_64)
adb install -r bin/dory-spike.apk
adb shell pm grant com.wails.app android.permission.CAMERA   # optional, the app also asks
```

`wails3 task android:package` / `android:run` / `android:deploy-device` exist too.
Delete `build/android/app/src/main/jniLibs` when switching ABI, or the APK carries both.
Use `wails3 task android:logs` for logcat. `bin/`, `jniLibs`, `*.so`, `*.apk` are git-ignored.

Desktop checks: `go test ./internal/...` (sqlite, handler, mTLS, heartbeat).

## Desktop side of check 5

```sh
scripts/mtls-check.sh                # emulator / USB phone: uses `adb forward`, pulls certs via run-as
scripts/mtls-check.sh 192.168.1.50   # phone on the LAN (IP shown in the app header), port 47890
```

It pulls `ca.pem`, `client.pem`, `client.key` from the debug app's `files/pki/`, calls
`https://dory-spike.local:47890/ping` with curl (`--resolve`, `--cacert`, `--cert`, `--key`) and
proves a client without certificate is rejected (`tlsv13 alert certificate required`).

## Measurements (x86_64 emulator, API 36, debug unless noted)

- APK: release arm64 **12 MB** (libwails.so 17 MB, minify off); debug arm64 21 MB; debug x86_64 35 MB.
- Cold start `am start -W` TotalTime: 434-815 ms (emulator, debug). Webview shows the UI ~25-70 ms
  after the page script starts; Go-side time since process start at first request: 125-300 ms.
- First Go build (cold module cache) ~15 s; incremental `android:build` ~6 s; Gradle assemble ~45 s cold, <1 s warm.

## Findings (things that bit)

1. **Wails asset path**: the Android webview calls `shouldInterceptRequest` and forwards to Go as a GET.
   - Request bodies never reach the `http.Handler` (POST body lost). Use GET and put inputs in the **URL path**.
   - Query strings are stripped for every path outside `/wails/` (WebViewAssetLoader). Only `/wails/*` keeps them.
   - Any non-200 from the handler becomes "asset not found" (incl. the 301 `http.FileServer` gives for `/index.html`,
     which showed `ERR_CONNECTION_REFUSED`). Serve index directly.
2. **Camera**: the template's WebChromeClient has no `onPermissionRequest`, so `getUserMedia` is denied.
   Patched `MainActivity.java` (grant only VIDEO_CAPTURE to `wails.localhost`, ask the runtime permission first).
   Manifest already has CAMERA. `BarcodeDetector` exists in this WebView; jsQR 1.4 is bundled as fallback.
3. **mDNS**: Go cannot enumerate interfaces on Android 11+ (netlink blocked), so `zeroconf.Register` fails with
   "Could not determine host IP addresses". Fix: Java (`DoryNative.interfaces()`) lists interfaces, Go uses
   `zeroconf.RegisterProxy` with explicit IPs/ifaces. `CHANGE_WIFI_MULTICAST_STATE` + a `MulticastLock` are required.
   Only same-device self-discovery was tested; discovery across real Wi-Fi (AP isolation, other peers) is unverified.
4. **modernc sqlite on x86_64 Android**: `libc` issues the raw `lstat` syscall (nr 6) on amd64, which Android's
   seccomp filter kills with SIGSYS (app dies, no Go panic). arm64 has no `lstat` syscall, so phones should be fine,
   but this is **unverified**: the x86_64 emulator cannot run check 2, and its arm64 translation layer SIGILLs on the Go runtime.
   Needs a real arm64 device.
5. **Background**: `WailsForegroundService` (type `dataSync`) keeps the process alive; the heartbeat goroutine and the
   mTLS listener kept running 3 min with the app in the background (emulator). Not tested: real device Doze, battery
   saver, OEM killers, screen off, >10 min. On Android 15+ `dataSync` foreground services are limited to ~6 h per 24 h
   and the type must be declared; `connectedDevice`/`specialUse` are alternatives to evaluate. Without the foreground
   service Android may freeze or kill the cached process, so a peer cannot count on a listener in the background.
6. Release APK is signed with the debug keystore unless `ANDROID_KEYSTORE_FILE` is set.
