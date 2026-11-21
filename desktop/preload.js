// The interface is the same web app as in the browser; it only learns that it
// runs inside the desktop app. setZoom applies the interface scale as the
// page's real zoom, so the layout reflows as in a narrower window. idleState
// tells whether the user is away from this computer.
const { contextBridge, ipcRenderer, webFrame } = require("electron");
contextBridge.exposeInMainWorld("doryDesktop", {
  platform: process.platform,
  idleState: () => ipcRenderer.invoke("idle-state"),
  setZoom: (factor) => webFrame.setZoomFactor(Number(factor) || 1),
});
