// The interface is the same web app as in the browser; it only learns that it
// runs inside the desktop app. setZoom applies the interface scale as the
// page's real zoom, so the layout reflows as in a narrower window.
const { contextBridge, webFrame } = require("electron");
contextBridge.exposeInMainWorld("doryDesktop", {
  platform: process.platform,
  setZoom: (factor) => webFrame.setZoomFactor(Number(factor) || 1),
});
