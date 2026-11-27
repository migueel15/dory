// Service worker: makes Dory installable and opens it when the node cannot be
// reached. It keeps only the page shell and the built assets; the API is never
// cached, so data is always live.
// Renaming the caches drops what older versions saved: v1 could keep a page
// from an old build forever.
const SHELL = "dory-shell-v2";
const ASSETS = "dory-assets-v2";
// SHELL_WAIT is how long a page load waits for the node before it opens the
// saved page.
const SHELL_WAIT = 1500;

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil((async () => {
  const keep = [SHELL, ASSETS];
  let oldShell = false;
  for (const name of await caches.keys()) {
    if (keep.includes(name)) continue;
    oldShell ||= name.startsWith("dory-shell-");
    await caches.delete(name);
  }
  await self.clients.claim();
  // A page opened from an older worker may run an old build: reload it.
  if (oldShell) await reloadPages();
})()));

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith("/api/")) return;
  if (req.mode === "navigate") event.respondWith(shell(req, event));
  else if (url.pathname.startsWith("/assets/")) event.respondWith(asset(req));
});

async function reloadPages() {
  for (const c of await self.clients.matchAll({ type: "window", includeUncontrolled: true })) c.postMessage({ type: "shell-updated" });
}

// shell answers a page load from the node and saves the page. Only when the
// node fails or takes longer than SHELL_WAIT it opens the saved page; if the
// node then answers with a newer page, the open pages are told to reload.
// A new page also drops the assets of the old build.
async function shell(req, event) {
  const cache = await caches.open(SHELL);
  const saved = await cache.match("/");
  // Read the saved page now: once it answers the page load its body is gone.
  const before = saved ? await saved.clone().text() : undefined;
  let fallback = false;
  const fresh = fetch(req).then(async (res) => {
    const html = res.ok && (res.headers.get("content-type") || "").includes("text/html");
    if (!html) return res;
    const text = await res.clone().text();
    if (text !== before) {
      await cache.put("/", res.clone());
      if (before !== undefined) {
        await caches.delete(ASSETS);
        if (fallback) await reloadPages();
      }
    }
    return res;
  });
  event.waitUntil(fresh.catch(() => {}));
  if (!saved) return fresh;
  const late = new Promise((resolve) => setTimeout(() => { fallback = true; resolve(); }, SHELL_WAIT));
  const res = await Promise.race([fresh.catch(() => undefined), late]);
  if (res && res.ok) return res;
  fallback = true;
  return saved;
}

// asset serves a built file from the cache: Vite names them by content hash,
// so a saved copy never goes stale. A page is never kept as an asset.
async function asset(req) {
  const cache = await caches.open(ASSETS);
  const hit = await cache.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok && !(res.headers.get("content-type") || "").includes("text/html")) await cache.put(req, res.clone());
  return res;
}

// Every push must end in a visible notification: if it does not (missing or
// unreadable payload, a failed showNotification), the browser shows its own
// generic "site updated in the background" message instead.
self.addEventListener("push", (event) => {
  let data = {};
  try { data = event.data ? event.data.json() : {}; }
  catch { try { data = { body: event.data.text() }; } catch { data = {}; } }
  const title = typeof data.title === "string" && data.title.trim() ? data.title : "Dory";
  const options = {
    body: data.body || "Abrir en Dory", icon: "/icon-192.png", badge: "/icon-192.png",
    tag: data.tag ? String(data.tag) : undefined, renotify: !!data.tag,
    data: { url: data.url || "/" },
  };
  event.waitUntil(
    self.registration.showNotification(title, options)
      .catch(() => self.registration.showNotification(title, { body: options.body, data: options.data })),
  );
});
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(event.notification.data?.url || "/", self.location.origin);
  if (target.protocol !== "https:" && target.protocol !== "http:") return;
  event.waitUntil(self.clients.openWindow(target.href));
});
