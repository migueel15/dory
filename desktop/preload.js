// The interface is the same web app as in the browser; it only learns that it
// runs inside the desktop app.
const { contextBridge } = require("electron");
contextBridge.exposeInMainWorld("doryDesktop", { platform: process.platform });
