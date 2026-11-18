// Minimal service worker: makes Dory installable. It caches nothing, so data is always live.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {});

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
