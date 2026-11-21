// Dory desktop: a thin window over the core of this machine. The core is a
// background service; the app starts it when needed and never stops it, so
// threads keep running with the window closed (requirement 4.4).
const { app, BrowserWindow, shell, Menu, Tray, nativeImage, dialog } = require("electron");
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

// DORY_DESKTOP_URL opens the window over another core, such as the dev
// server, with its own profile and without touching the installed core.
const devURL = process.env.DORY_DESKTOP_URL;
// The dev window shows a yellow icon so it is not mistaken for the installed app.
const iconPath = path.join(__dirname, "build", devURL ? "icon-dev.png" : "icon.png");
const profile = path.join(os.homedir(), ".dory", devURL ? "desktop-dev" : "desktop");
if (devURL || fs.existsSync(profile) || !fs.existsSync(path.join(app.getPath("userData"), "Local State"))) {
  fs.mkdirSync(profile, { recursive: true, mode: 0o700 });
  app.setPath("userData", profile);
}

const exe = process.platform === "win32" ? "dory.exe" : "dory";
const bundled = path.join(process.resourcesPath || "", "bin", exe);
// The core this app installs as a user service, next to the CLI.
const installed = process.platform === "win32"
  ? path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local"), "Dory", "bin", exe)
  : path.join(os.homedir(), ".local", "bin", exe);
const managed = path.join(profile, "core-managed");

// The installed core, the one shipped with the app, or the one on PATH.
function doryBin() {
  for (const bin of [installed, bundled]) if (fs.existsSync(bin)) return bin;
  return exe;
}

function run(bin, args) {
  return new Promise((resolve, reject) => {
    execFile(bin, args, { timeout: 60_000 }, (err, stdout, stderr) => (err ? reject(new Error(stderr || err.message)) : resolve(stdout.trim())));
  });
}

const dory = (args) => run(doryBin(), args);

// installCore leaves the core of the app as a user service, so it keeps
// running with the window closed and starts with the session. It installs
// it when there is no core yet and keeps updating the one it installed; a
// core installed by other means is left alone.
async function installCore() {
  if (!app.isPackaged || !fs.existsSync(bundled)) return;
  if (process.env.APPIMAGE) {
    // dory open and the launcher find the app here.
    const link = path.join(os.homedir(), ".local", "bin", "Dory.AppImage");
    try {
      if (fs.lstatSync(link, { throwIfNoEntry: false })?.isSymbolicLink() !== false) {
        fs.mkdirSync(path.dirname(link), { recursive: true });
        fs.rmSync(link, { force: true });
        fs.symlinkSync(process.env.APPIMAGE, link);
      }
    } catch {}
  }
  const exists = fs.existsSync(installed);
  if (exists && !fs.existsSync(managed)) return;
  if (exists && (await run(installed, ["version"]).catch(() => "")) === (await run(bundled, ["version"]).catch(() => "-"))) return;
  fs.mkdirSync(path.dirname(installed), { recursive: true });
  const tmp = `${installed}.new`;
  fs.copyFileSync(bundled, tmp);
  fs.chmodSync(tmp, 0o755);
  // Windows keeps a running executable locked but lets it be renamed.
  if (exists && process.platform === "win32") fs.renameSync(installed, `${installed}.old`);
  fs.renameSync(tmp, installed);
  fs.rmSync(`${installed}.old`, { force: true });
  fs.writeFileSync(managed, "");
  await run(installed, ["service", "install"]);
  if (exists) await run(installed, ["restart"]).catch(() => {});
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
  if (!devURL) await installCore().catch((err) => console.error("dory: install core:", err.message));
  const url = devURL || await dory(["url"]).catch(() => "http://127.0.0.1:7700");
  if (!devURL && !(await healthy(url))) await dory(["start"]);
  win = new BrowserWindow({
    width: 1400, height: 900, minWidth: 380, minHeight: 500, show: !hidden,
    title: devURL ? "Dory (dev)" : "Dory", backgroundColor: "#0f0f11", autoHideMenuBar: true,
    icon: iconPath,
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
  // Closing the window hides it: the app stays in the tray, like the core.
  win.on("close", (e) => { if (!quitting) { e.preventDefault(); win.hide(); } });
  await win.loadURL(url);
  if (!devURL) followUpdates(url);
}

// Started at sign-in, the app runs in the background without a window.
const hidden = process.argv.includes("--hidden");
let quitting = false;
let tray;

function show() {
  if (!win) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
}

function quit() {
  quitting = true;
  app.quit();
}

function createTray() {
  // Trays draw small icons: 16 px on Windows and macOS, 22 px on Linux.
  const size = process.platform === "linux" ? 22 : 16;
  const icon = nativeImage.createFromPath(iconPath);
  tray = new Tray(icon.resize({ width: size, height: size, quality: "best" }));
  tray.setToolTip("Dory");
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: "Abrir Dory", click: show },
    { type: "separator" },
    { label: "Salir", click: quit },
  ]));
  tray.on("click", show);
}

// An AppImage is not integrated in the system by itself: the app installs
// its icon and its launcher entry, pointing at the AppImage, so it shows up
// in the application launcher with its icon.
function integrateAppImage() {
  if (process.platform !== "linux" || !process.env.APPIMAGE) return;
  const share = path.join(os.homedir(), ".local", "share");
  const icon = path.join(share, "icons", "hicolor", "512x512", "apps", "dory.png");
  const entry = path.join(share, "applications", "dory.desktop");
  try {
    fs.mkdirSync(path.dirname(icon), { recursive: true });
    fs.copyFileSync(path.join(__dirname, "build", "icon.png"), icon);
    fs.mkdirSync(path.dirname(entry), { recursive: true });
    fs.writeFileSync(entry, `[Desktop Entry]\nType=Application\nName=Dory\nComment=Tu entorno de trabajo con agentes\nExec="${process.env.APPIMAGE}" %U\nIcon=dory\nTerminal=false\nCategories=Development;\nStartupWMClass=Dory\n`);
  } catch {}
}

// The app opens with the session, in the background. Linux has no login
// items in Electron, so it gets an autostart entry.
function startAtLogin() {
  if (!app.isPackaged) return;
  if (process.platform !== "linux") {
    app.setLoginItemSettings({ openAtLogin: true, args: ["--hidden"] });
    return;
  }
  const exec = process.env.APPIMAGE || process.execPath;
  const entry = path.join(os.homedir(), ".config", "autostart", "dory.desktop");
  try {
    fs.mkdirSync(path.dirname(entry), { recursive: true });
    fs.writeFileSync(entry, `[Desktop Entry]\nType=Application\nName=Dory\nExec="${exec}" --hidden\nIcon=dory\nX-GNOME-Autostart-enabled=true\n`);
  } catch {}
}

// One window per machine: a second launch focuses the first.
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", show);
  app.on("before-quit", () => { quitting = true; });
  app.whenReady().then(() => {
    Menu.setApplicationMenu(null);
    createTray();
    startAtLogin();
    integrateAppImage();
    open().catch((err) => {
      dialog.showErrorBox("Dory", `No se pudo arrancar el núcleo.\n\n${err.message}\n\nInstala el binario dory o ejecuta \`dory service install\`.`);
      app.quit();
    });
  });
  app.on("activate", show);
  // The window only hides; the app quits from the tray.
  app.on("window-all-closed", () => {});
}
