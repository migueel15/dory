// The interface is the same web app as in the browser; it only learns that it
// runs inside the desktop app. setZoom applies the interface scale as the
// page's real zoom, so the layout reflows as in a narrower window.
const { contextBridge, webFrame, ipcRenderer } = require("electron");

// Bounds come in CSS pixels of the interface; the window works in its own.
const scaled = (b) => {
  const z = webFrame.getZoomFactor();
  return { x: b.x * z, y: b.y * z, width: b.width * z, height: b.height * z };
};
const listen = (channel, cb) => {
  const fn = (_e, ...args) => cb(...args);
  ipcRenderer.on(channel, fn);
  return () => ipcRenderer.removeListener(channel, fn);
};

contextBridge.exposeInMainWorld("doryDesktop", {
  platform: process.platform,
  setZoom: (factor) => webFrame.setZoomFactor(Number(factor) || 1),
  // The thread browser: native views of tabs and the tunnel to remote nodes.
  browser: {
    show: (tab, url, bounds) => ipcRenderer.invoke("browser:show", String(tab), String(url), scaled(bounds)),
    bounds: (tab, bounds, visible) => ipcRenderer.invoke("browser:bounds", String(tab), scaled(bounds), !!visible),
    hide: (tab) => ipcRenderer.invoke("browser:hide", String(tab)),
    park: (tab) => ipcRenderer.invoke("browser:park", String(tab)),
    importCookies: (source) => ipcRenderer.invoke("browser:import-cookies", source ? String(source) : ""),
    importSources: () => ipcRenderer.invoke("browser:import-sources"),
    importSettings: (s) => ipcRenderer.invoke("browser:import-settings", s),
    focusUI: () => ipcRenderer.invoke("browser:focus-ui"),
    onKey: (cb) => listen("browser:key", cb),
    cdp: (tab, method, params) => ipcRenderer.invoke("browser:cdp", String(tab), String(method), params),
    onEvent: (cb) => listen("browser:event", cb),
    onOpen: (cb) => listen("browser:open", cb),
    onGone: (cb) => listen("browser:gone", cb),
    tunnelListen: (port) => ipcRenderer.invoke("tunnel:listen", Number(port)),
    tunnelWrite: (id, data) => ipcRenderer.send("tunnel:write", Number(id), new Uint8Array(data)),
    tunnelEnd: (id) => ipcRenderer.send("tunnel:end", Number(id)),
    onTunnelOpen: (cb) => listen("tunnel:open", cb),
    onTunnelData: (cb) => listen("tunnel:data", cb),
    onTunnelClose: (cb) => listen("tunnel:close", cb),
  },
});
