// Dory desktop: a thin window over the core of this machine. The core is a
// background service; the app starts it when needed and never stops it, so
// threads keep running with the window closed (requirement 4.4).
const { app, BrowserWindow, shell, Menu, dialog } = require("electron");
const { execFile } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { autoUpdater } = require("electron-updater");

// Chromium names its audio streams "Chromium input" for PulseAudio and
// PipeWire. The audio service inherits this environment, so the dictation
// microphone shows up as Dory in wpctl and similar tools.
if (process.platform === "linux") {
  const props = "application.name=Dory application.icon_name=dory media.name=Dory";
  process.env.PULSE_PROP = process.env.PULSE_PROP ? `${process.env.PULSE_PROP} ${props}` : props;
}

const profile = path.join(os.homedir(), ".dory", "desktop");
if (fs.existsSync(profile) || !fs.existsSync(path.join(app.getPath("userData"), "Local State"))) {
  fs.mkdirSync(profile, { recursive: true, mode: 0o700 });
  app.setPath("userData", profile);
}

// The binary shipped with the app, or the one on PATH.
function doryBin() {
  const name = process.platform === "win32" ? "dory.exe" : "dory";
  const bundled = path.join(process.resourcesPath || "", "bin", name);
  return fs.existsSync(bundled) ? bundled : name;
}

function dory(args) {
  return new Promise((resolve, reject) => {
    execFile(doryBin(), args, { timeout: 30_000 }, (err, stdout, stderr) => (err ? reject(new Error(stderr || err.message)) : resolve(stdout.trim())));
  });
}

async function healthy(url) {
  try {
    const res = await fetch(`${url}/api/health`, { signal: AbortSignal.timeout(1500) });
    return res.ok && (await res.json()).app === "dory";
  } catch {
    return false;
  }
}

let win;

// The core downloads the desktop release of its channel when the mesh updates
// (ADR-016); the app installs it from there and restarts.
async function followUpdates(url) {
  if (!app.isPackaged) return;
  try {
    const { data_dir: dataDir } = await (await fetch(`${url}/api/health`)).json();
    const token = fs.readFileSync(path.join(dataDir, "control.token"), "utf8").trim();
    autoUpdater.requestHeaders = { Authorization: `Bearer ${token}` };
  } catch {
    return;
  }
  autoUpdater.setFeedURL({ provider: "generic", url: `${url}/api/mesh/desktop` });
  autoUpdater.allowPrerelease = true;
  autoUpdater.autoInstallOnAppQuit = true;
  autoUpdater.on("update-downloaded", () => autoUpdater.quitAndInstall(true, true));
  autoUpdater.on("error", () => {});
  const check = () => autoUpdater.checkForUpdates().catch(() => {});
  check();
  setInterval(check, 2 * 60_000);
}

async function open() {
  const url = await dory(["url"]).catch(() => "http://127.0.0.1:7700");
  if (!(await healthy(url))) await dory(["start"]);
  win = new BrowserWindow({
    width: 1400, height: 900, minWidth: 380, minHeight: 500,
    title: "Dory", backgroundColor: "#0f0f11", autoHideMenuBar: true,
    icon: path.join(__dirname, "build", "icon.png"),
    webPreferences: { preload: path.join(__dirname, "preload.js"), contextIsolation: true, sandbox: true },
  });
  // Links to other sites open in the system browser.
  win.webContents.setWindowOpenHandler(({ url: target }) => {
    if (!target.startsWith(url)) { shell.openExternal(target); return { action: "deny" }; }
    return { action: "allow" };
  });
  win.webContents.on("will-navigate", (e, target) => {
    if (!target.startsWith(url)) { e.preventDefault(); shell.openExternal(target); }
  });
  await win.loadURL(url);
  followUpdates(url);
}

// One window per machine: a second launch focuses the first.
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", () => {
    if (win) { if (win.isMinimized()) win.restore(); win.focus(); }
  });
  app.whenReady().then(() => {
    Menu.setApplicationMenu(null);
    open().catch((err) => {
      dialog.showErrorBox("Dory", `No se pudo arrancar el núcleo.\n\n${err.message}\n\nInstala el binario dory o ejecuta \`dory service install\`.`);
      app.quit();
    });
  });
  app.on("activate", () => { if (BrowserWindow.getAllWindows().length === 0) open(); });
  // Closing the window leaves the core running.
  app.on("window-all-closed", () => { if (process.platform !== "darwin") app.quit(); });
}
