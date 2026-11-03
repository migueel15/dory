// Minimal service worker: makes Dory installable. It caches nothing, so data is always live.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {});

self.addEventListener("push", (event) => {
  if (!event.data) return;
  const data = event.data.json();
  event.waitUntil(self.registration.showNotification(data.title, {
    body: "Abrir en Dory", icon: "/icon-192.png", tag: data.tag,
    data: { url: data.url },
  }));
});
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(event.notification.data.url, self.location.origin);
  if (target.protocol !== "https:" && target.protocol !== "http:") return;
  event.waitUntil(self.clients.openWindow(target.href));
});
