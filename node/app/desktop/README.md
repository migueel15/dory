# dory-desktop

Linux desktop app for the Dory local-first node (Wails v3.1.0, WebKitGTK 6 / GTK4).
The window serves the `nodeapp` handler in-process through the Wails AssetServer: there is no local HTTP port for the UI.
Only the mTLS peer port (default 7710) is open, for other devices.

```sh
wails3 build            # or: task build. Builds ../../../web (npm ci + build) and bin/dory-desktop
./bin/dory-desktop
```

Environment: `DORY_DESKTOP_DATA_DIR` (default `~/.local/share/dory-desktop`), `DORY_DESKTOP_PEER_PORT` (7710; 0 = any),
`DORY_DESKTOP_TRACE=1` (log every request of the window), `DORY_DESKTOP_LOOPBACK=1` (fallback: serve the same handler on
`127.0.0.1:<random>` and point the window there).

Menu `Dory > Nodo y dispositivos` opens `/node/` (members with presence, add device with QR and confirm prompt, revoke, device name,
manual address). Closing the window quits the app.

## AssetServer on Linux (verified with `DORY_DESKTOP_TRACE=1` under xvfb)

Unlike Android (see `../../spikes/wails-android/README.md`), on Linux/WebKitGTK the in-process handler receives:
POST/PUT/PATCH bodies (as chunked, `ContentLength=-1`, but readable), query strings, and non-200 statuses
(400/401/404/503 reach the page with their bodies), and SSE (`/api/events/stream`) streams with `Flush`.
The SPA loads unchanged.

One real difference: the webview keeps no cookies for the `wails://` scheme, so the SPA got 401 on every call.
`Node.Handler()` therefore attaches the local session to each request and drops `Origin`/`Referer`/`Sec-Fetch-Site`
(the `wails://localhost` origin would fail the same-origin check on mutations). The loopback listener keeps the normal cookie flow.
