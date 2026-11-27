// Service worker: makes Dory installable and opens it without waiting for the
// network. It keeps only the page shell and the built assets; the API is never
// cached, so data is always live.
const SHELL = "dory-shell-v1";
const ASSETS = "dory-assets-v1";

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil((async () => {
  const keep = [SHELL, ASSETS];
  for (const name of await caches.keys()) if (!keep.includes(name)) await caches.delete(name);
  await self.clients.claim();
})()));

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith("/api/")) return;
  if (req.mode === "navigate") event.respondWith(shell(req, event));
  else if (url.pathname.startsWith("/assets/")) event.respondWith(asset(req));
});

// shell answers a page load with the saved page at once and fetches the
// current one behind it. When the node was updated the page differs: the
// assets of the old build go and the open pages are told to reload.
async function shell(req, event) {
  const cache = await caches.open(SHELL);
  const saved = await cache.match("/");
  const fresh = fetch(req).then(async (res) => {
    const html = res.ok && (res.headers.get("content-type") || "").includes("text/html");
    if (!html) return res;
    const text = await res.clone().text();
    const before = saved ? await saved.clone().text() : undefined;
    await cache.put("/", res.clone());
    if (before !== undefined && before !== text) {
      await caches.delete(ASSETS);
      for (const c of await self.clients.matchAll({ type: "window" })) c.postMessage({ type: "shell-updated" });
    }
    return res;
  });
  if (!saved) return fresh;
  event.waitUntil(fresh.catch(() => {}));
  return saved;
}

// asset serves a built file from the cache: Vite names them by content hash,
// so a saved copy never goes stale.
async function asset(req) {
  const cache = await caches.open(ASSETS);
  const hit = await cache.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) await cache.put(req, res.clone());
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
