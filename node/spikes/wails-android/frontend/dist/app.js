"use strict";
const $ = (id) => document.getElementById(id);
const t0 = performance.now();

function show(name, ok, detail) {
  const row = $("r-" + name);
  const b = row.querySelector(".badge");
  b.textContent = ok === null ? "…" : ok ? "OK" : "ERROR";
  b.className = "badge " + (ok === null ? "" : ok ? "ok" : "err");
  row.querySelector(".detail").textContent = detail;
}

function b64url(str) {
  const bytes = new TextEncoder().encode(str);
  let bin = ""; bytes.forEach((b) => (bin += String.fromCharCode(b)));
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

async function api(path, opts) {
  const r = await fetch(path, opts);
  if (!r.ok) throw new Error(path + " -> HTTP " + r.status);
  return r.json();
}

async function server(name) {
  show(name, null, "running…");
  try {
    const r = await api("/api/checks/" + name);
    show(name, r.ok, `${r.detail}  [${r.ms} ms]`);
    return r;
  } catch (e) {
    show(name, false, String(e));
  }
}

// Android only: the Java side lists interfaces (Go cannot) and holds the multicast lock.
async function pushIfaces() {
  if (!window.DoryNative) return "DoryNative absent (desktop browser?)";
  const list = JSON.parse(window.DoryNative.interfaces());
  const r = await api("/api/ifaces/set/" + b64url(JSON.stringify(list)));
  return `${list.map((i) => i.name + "=" + i.ips.join("/")).join(", ")} (mtls restart: ${r.mtlsRestart || "ok"})`;
}

async function checkHandler() {
  show("handler", null, "running…");
  try {
    const t = performance.now();
    const p = await api("/api/ping");
    const rtt = (performance.now() - t).toFixed(1);
    const r = await api("/api/checks/handler");
    show("handler", p.ok && r.ok,
      `${r.detail}\nfetch RTT ${rtt} ms, ${p.goos}/${p.goarch} ${p.goVersion}, origin ${location.origin}, ` +
      `Go-side time since process start at first call ${p.sinceProcessStartMs} ms`);
    $("meta").textContent = `origin ${location.origin} · page ready ${(performance.now() - t0).toFixed(0)} ms after script start`;
  } catch (e) { show("handler", false, String(e)); }
}

async function checkMdns() {
  show("mdns", null, "taking multicast lock…");
  const held = window.DoryNative ? window.DoryNative.multicast(true) : "n/a";
  await server("mdns");
  const d = $("r-mdns").querySelector(".detail");
  d.textContent += `\nmulticast lock held: ${held}`;
}

let stream = null, scanning = false;
async function scan() {
  const v = $("video");
  show("camera", null, "asking for the camera…");
  try {
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia)
      throw new Error("navigator.mediaDevices.getUserMedia missing (insecure context?)");
    stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" }, audio: false });
    v.srcObject = stream; v.style.display = "block"; await v.play();
  } catch (e) { show("camera", false, `permission/stream: ${e.name}: ${e.message}`); return; }

  const hasBD = "BarcodeDetector" in window;
  let det = null;
  if (hasBD) { try { det = new BarcodeDetector({ formats: ["qr_code"] }); } catch (e) { det = null; } }
  let engine = det ? "BarcodeDetector" : "jsQR (zxing-js alternative)";
  show("camera", null, `camera open, engine: ${engine}. Show a QR code…`);
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  scanning = true;
  const started = performance.now();
  while (scanning) {
    if (v.videoWidth) {
      canvas.width = v.videoWidth; canvas.height = v.videoHeight;
      ctx.drawImage(v, 0, 0);
      let text = null;
      if (det) {
        try { const r = await det.detect(canvas); if (r.length) text = r[0].rawValue; }
        catch (e) { det = null; engine = "jsQR (fallback)"; show("camera", null, "BarcodeDetector failed (" + e.name + "), falling back to jsQR"); }
      } else {
        const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const c = window.jsQR(img.data, img.width, img.height, { inversionAttempts: "dontInvert" });
        if (c) text = c.data;
      }
      if (text) {
        show("camera", true, `${engine} decoded in ${((performance.now() - started) / 1000).toFixed(1)} s: ${text}`);
        stopScan(); return;
      }
    }
    await new Promise((r) => setTimeout(r, 120));
  }
}
function stopScan() {
  scanning = false;
  if (stream) stream.getTracks().forEach((t) => t.stop());
  stream = null; $("video").style.display = "none";
}

async function refreshBg() {
  try {
    const b = await api("/api/background");
    const h = b.heartbeat;
    const v = await api("/api/checks/background");
    show("background", v.ok,
      `${v.detail}\nTCP conns accepted by mTLS listener: ${b.mtlsConns}, requests: ${b.mtlsReqs}\n` +
      (h.gaps.length ? "gaps: " + h.gaps.map((g) => `${g.at} ${g.seconds.toFixed(0)}s`).join("; ") : "no gaps > " + h.tolerateSec + " s") +
      `\nBackground the app for 5+ min, come back, press Refresh. Toggle the foreground service to compare.`);
  } catch (e) { show("background", false, String(e)); }
}

$("scan").onclick = scan;
$("stopScan").onclick = stopScan;
$("bgRefresh").onclick = refreshBg;
$("fgOn").onclick = async () => { await api("/api/foreground/on"); setTimeout(refreshBg, 500); };
$("fgOff").onclick = async () => { await api("/api/foreground/off"); setTimeout(refreshBg, 500); };
$("runAll").onclick = async () => {
  await checkHandler();
  await checkMdns();
  await server("mtls");
  await refreshBg();
  await server("sqlite"); // last: on an x86_64 emulator modernc sqlite dies with SIGSYS (see README)
};
// one button per check, so a native crash in one does not hide the others
for (const [name, fn] of [["handler", checkHandler], ["sqlite", () => server("sqlite")], ["mdns", checkMdns], ["mtls", () => server("mtls")]]) {
  const b = document.createElement("button");
  b.textContent = "Run " + name; b.onclick = fn;
  $("r-" + name).appendChild(b);
}

(async () => {
  await checkHandler();
  try { $("meta").textContent += " · ifaces: " + await pushIfaces(); } catch (e) { $("meta").textContent += " · ifaces error: " + e; }
})();
