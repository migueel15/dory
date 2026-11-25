// Dory desktop: a window over the core of this machine. The app runs the core
// it ships as its child and lives in the tray, so threads keep running with
// the window closed; quitting from the tray stops both (ADR-029).
const { app, BrowserWindow, WebContentsView, ipcMain, shell, Menu, Tray, nativeImage, dialog, session } = require("electron");
const net = require("node:net");
const { execFile, spawn } = require("node:child_process");
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
// Where older versions installed the core as a user service; the app now
// leaves the dory CLI there on Linux and macOS.
const installed = process.platform === "win32"
  ? path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local"), "Dory", "bin", exe)
  : path.join(os.homedir(), ".local", "bin", exe);
const managed = path.join(profile, "core-managed");
const cliManaged = path.join(profile, "cli-managed");

// The core shipped with the app, or the one on PATH in a local build.
function doryBin() {
  return app.isPackaged && fs.existsSync(bundled) ? bundled : exe;
}

function run(bin, args) {
  return new Promise((resolve, reject) => {
    execFile(bin, args, { timeout: 60_000 }, (err, stdout, stderr) => (err ? reject(new Error(stderr || err.message)) : resolve(stdout.trim())));
  });
}

// migrateCore removes the core older versions installed as a user service
// (ADR-029): the app runs its own core now. A core installed by other means,
// without the marker, is left alone.
async function migrateCore() {
  if (!fs.existsSync(managed)) return;
  if (fs.existsSync(installed)) {
    await run(installed, ["stop"]).catch(() => {});
    await run(installed, ["service", "uninstall"]).catch(() => {});
  }
  for (const f of [installed, `${installed}.old`, `${installed}.new`]) fs.rmSync(f, { force: true });
  if (process.platform === "win32") fs.rmSync(path.dirname(installed), { recursive: true, force: true });
  fs.rmSync(managed, { force: true });
}

// installCLI leaves the dory command of the app on PATH: a copy in
// ~/.local/bin on Linux and macOS (an AppImage mounts elsewhere each run),
// and the app's own folder in the user PATH on Windows.
async function installCLI() {
  if (!app.isPackaged || !fs.existsSync(bundled)) return;
  if (process.platform === "win32") {
    const dir = path.dirname(bundled);
    const ps = `$p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not (($p -split ';') -contains '${dir}')) { [Environment]::SetEnvironmentVariable('Path', (($p.TrimEnd(';'), '${dir}') -join ';').TrimStart(';'), 'User') }`;
    await run("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", ps]).catch(() => {});
    return;
  }
  if (fs.existsSync(installed) && !fs.existsSync(cliManaged)) return;
  if (fs.existsSync(installed) && (await run(installed, ["version"]).catch(() => "")) === (await run(bundled, ["version"]).catch(() => "-"))) return;
  fs.mkdirSync(path.dirname(installed), { recursive: true });
  const tmp = `${installed}.new`;
  fs.copyFileSync(bundled, tmp);
  fs.chmodSync(tmp, 0o755);
  fs.renameSync(tmp, installed);
  fs.writeFileSync(cliManaged, "");
}

async function healthy(url) {
  try {
    const res = await fetch(`${url}/api/health`, { signal: AbortSignal.timeout(1500) });
    return res.ok && (await res.json()).app === "dory";
  } catch {
    return false;
  }
}

// The core runs as a child of the app and stops when the app quits. When it
// ends on its own, by `dory restart` or a failure, the app starts it again.
let core;

function spawnCore() {
  const log = fs.openSync(path.join(profile, "core.log"), "a");
  core = spawn(doryBin(), ["serve"], {
    env: { ...process.env, DORY_DESKTOP_CORE: "1" },
    stdio: ["ignore", log, log],
    windowsHide: true,
  });
  fs.closeSync(log);
  core.on("error", (err) => console.error("dory: core:", err.message));
  core.on("exit", () => {
    core = undefined;
    if (!quitting) setTimeout(() => startCore().catch(() => {}), 1000);
  });
}

// startCore uses a healthy core already on the port, such as a dev one, and
// otherwise starts the core of the app and waits for it.
// A core of another version, such as the one an update left behind, is
// stopped first so the app always runs the core it ships.
async function startCore(url = coreURL) {
  if (await healthy(url)) {
    if (core || !app.isPackaged) return;
    const mine = (await run(doryBin(), ["version"]).catch(() => "")).split(" ")[0];
    const running = await fetch(`${url}/api/health`).then((r) => r.json()).then((h) => h.version).catch(() => "");
    if (!mine || mine === running) return;
    await run(doryBin(), ["stop"]).catch(() => {});
  }
  spawnCore();
  for (let i = 0; i < 100; i++) {
    if (await healthy(url)) return;
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`el núcleo no arrancó; mira ${path.join(profile, "core.log")}`);
}

function stopCore() {
  if (core) core.kill();
}

// stopCoreAndWait ends the core and resolves once it has exited, killing it
// if it does not stop in time.
function stopCoreAndWait(timeout = 15_000) {
  if (!core) return Promise.resolve();
  const child = core;
  return new Promise((resolve) => {
    const timer = setTimeout(() => { child.kill("SIGKILL"); resolve(); }, timeout);
    child.once("exit", () => { clearTimeout(timer); resolve(); });
    child.kill();
  });
}

let coreURL;
let win;

// The core downloads the desktop release of its channel when the mesh updates
// (ADR-016); the app installs it from there as soon as it lands and restarts.
async function followUpdates(url) {
  if (!app.isPackaged) return;
  let dataDir;
  try {
    ({ data_dir: dataDir } = await (await fetch(`${url}/api/health`)).json());
    const token = fs.readFileSync(path.join(dataDir, "control.token"), "utf8").trim();
    autoUpdater.requestHeaders = { Authorization: `Bearer ${token}` };
  } catch {
    return;
  }
  autoUpdater.setFeedURL({ provider: "generic", url: `${url}/api/mesh/desktop` });
  autoUpdater.allowPrerelease = true;
  autoUpdater.autoInstallOnAppQuit = true;
  // The new app starts as soon as it is installed, so the old core must be
  // gone first: otherwise the new app finds it on the port and keeps it.
  const sendUpdate = (payload) => { if (win && !win.isDestroyed()) win.webContents.send("update:progress", payload); };
  autoUpdater.on("download-progress", (p) => {
    sendUpdate({ phase: "verify", percent: Math.round(p.percent || 0), transferred: p.transferred || 0, total: p.total || 0 });
  });
  autoUpdater.on("update-downloaded", async () => {
    sendUpdate({ phase: "restart", percent: 100 });
    quitting = true;
    await stopCoreAndWait();
    autoUpdater.quitAndInstall(true, true);
  });
  autoUpdater.on("error", () => {});
  const check = () => autoUpdater.checkForUpdates().catch(() => {});
  check();
  // The core moves the finished download into updates/desktop in one rename:
  // install as soon as it lands instead of polling.
  const updates = path.join(dataDir, "updates");
  fs.mkdirSync(updates, { recursive: true });
  let pending;
  fs.watch(updates, (_, name) => {
    if (name !== "desktop") return;
    clearTimeout(pending);
    pending = setTimeout(check, 500);
  });
}

async function open() {
  if (!devURL) {
    await migrateCore().catch((err) => console.error("dory: migrate core:", err.message));
    installCLI().catch((err) => console.error("dory: install cli:", err.message));
  }
  const url = devURL || await run(doryBin(), ["url"]).catch(() => "http://127.0.0.1:7700");
  coreURL = url;
  if (!devURL) await startCore(url);
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
  win.webContents.on("before-input-event", (e, input) => {
    if (input.type === "keyDown" && (input.control || input.meta) && !input.alt && !input.shift && input.key.toLowerCase() === "r") {
      e.preventDefault();
      win.webContents.reload();
    }
  });
  // Closing the window hides it: the app and its core stay in the tray.
  win.on("close", (e) => { if (!quitting) { e.preventDefault(); win.hide(); } });
  await win.loadURL(url);
  if (!devURL) followUpdates(url);
}

// Thread browser: each visible tab of a thread's browser is a native view
// over the window, in a session of its own shared by every thread. The page
// gets no preload and no Node; the interface drives it through its debugger,
// which the core of the thread's node reaches over the panel channel.
const views = new Map();
const fromUI = (e) => win && e.sender === win.webContents;

// The browser session presents itself as plain Chrome: sign-in pages such as
// Google's refuse browsers whose user agent names Electron.
let browserSessionReady = false;
function browserSession() {
  if (browserSessionReady) return;
  browserSessionReady = true;
  const ses = session.fromPartition("persist:dory-browser");
  ses.setUserAgent(ses.getUserAgent().replace(/\s(Electron|dory-desktop|Dory)\/\S+/gi, ""));
}

function browserView(tab, target) {
  let v = views.get(tab);
  if (v) return v;
  browserSession();
  const view = new WebContentsView({ webPreferences: { partition: "persist:dory-browser", sandbox: true, contextIsolation: true, nodeIntegration: false } });
  v = { view };
  views.set(tab, v);
  const wc = view.webContents;
  // Popups (sign-in with Google and the like) open as real child windows, so
  // they keep window.opener and can hand the result back to the page. Other
  // new windows become tabs of the same thread; the page stays here.
  wc.setWindowOpenHandler(({ url: next, disposition, features }) => {
    if (disposition === "new-window" || features) {
      return { action: "allow", overrideBrowserWindowOptions: { parent: win, width: 520, height: 680, autoHideMenuBar: true } };
    }
    if (win) win.webContents.send("browser:open", tab, next);
    return { action: "deny" };
  });
  // Browser shortcuts typed in the page belong to the panel.
  wc.on("before-input-event", (e, input) => {
    if (input.type !== "keyDown" || input.alt || input.shift || !(process.platform === "darwin" ? input.meta : input.control)) return;
    const key = String(input.key).toLowerCase();
    if (!["t", "w", "l"].includes(key)) return;
    e.preventDefault();
    if (win) win.webContents.send("browser:key", tab, key);
  });
  wc.debugger.attach("1.3");
  wc.debugger.on("message", (_e, method, params) => { if (win) win.webContents.send("browser:event", tab, method, params); });
  wc.debugger.on("detach", () => { if (win) win.webContents.send("browser:gone", tab); });
  // A new tab is an empty black page until the user goes somewhere; real
  // pages keep the white default under their own background.
  const paint = (url) => view.setBackgroundColor(!url || url === "about:blank" ? "#000000" : "#ffffff");
  paint(target);
  wc.on("did-start-navigation", (_e, url, _inPage, isMain) => { if (isMain) paint(url); });
  win.contentView.addChildView(view);
  if (target) wc.loadURL(target).catch(() => {});
  return v;
}

function dropView(tab) {
  const v = views.get(tab);
  if (!v) return;
  views.delete(tab);
  try { v.view.webContents.debugger.detach(); } catch {}
  if (win) win.contentView.removeChildView(v.view);
  v.view.webContents.close();
}

const rect = (b) => ({ x: Math.round(b.x), y: Math.round(b.y), width: Math.max(0, Math.round(b.width)), height: Math.max(0, Math.round(b.height)) });

ipcMain.handle("browser:show", (e, tab, target, bounds) => {
  if (!fromUI(e)) return;
  const v = browserView(tab, target);
  v.view.setBounds(rect(bounds));
  v.view.setVisible(true);
});
ipcMain.handle("browser:bounds", (e, tab, bounds, visible) => {
  const v = fromUI(e) && views.get(tab);
  if (!v) return;
  v.view.setBounds(rect(bounds));
  v.view.setVisible(visible !== false);
});
ipcMain.handle("browser:hide", (e, tab) => { if (fromUI(e)) dropView(tab); });
// park hides a view and keeps its page, so coming back to the tab keeps it as it was.
ipcMain.handle("browser:park", (e, tab) => { const v = fromUI(e) && views.get(tab); if (v) v.view.setVisible(false); });
// focusUI gives the keyboard back to the interface from a page view.
ipcMain.handle("browser:focus-ui", (e) => { if (fromUI(e)) win.webContents.focus(); });
// Cookie import: the settings (source browser and auto-import at start) live
// in this app's profile, as they belong to this machine's browsers.
const importFile = () => path.join(app.getPath("userData"), "browser-import.json");
function importSettings() {
  try { return { source: "", auto: false, ...JSON.parse(fs.readFileSync(importFile(), "utf8")) }; } catch { return { source: "", auto: false }; }
}
async function runImport(source) {
  browserSession();
  const cookies = require("./cookie-import");
  const id = source || importSettings().source || (await cookies.listSources()).default;
  return cookies.importCookies(session.fromPartition("persist:dory-browser"), id);
}
ipcMain.handle("browser:import-sources", async (e) => {
  if (!fromUI(e)) return;
  return { ...(await require("./cookie-import").listSources()), settings: importSettings() };
});
ipcMain.handle("browser:import-settings", (e, s) => {
  if (!fromUI(e)) return;
  const next = { source: String(s?.source || ""), auto: !!s?.auto };
  fs.writeFileSync(importFile(), JSON.stringify(next));
  return next;
});
ipcMain.handle("browser:import-cookies", (e, source) => { if (fromUI(e)) return runImport(source ? String(source) : ""); });
ipcMain.handle("browser:cdp", (e, tab, method, params) => {
  const v = fromUI(e) && views.get(tab);
  if (!v) throw new Error("no view");
  return v.view.webContents.debugger.sendCommand(method, params || {});
});

// Tunnel: a page of a remote node's thread asks for its localhost. The app
// listens on the same loopback port here and carries each connection, raw,
// through the interface to that node, so the page keeps its origin.
const tunnels = new Map();
const sockets = new Map();
let nextSocket = 0;
ipcMain.handle("tunnel:listen", (e, port) => new Promise((resolve) => {
  if (!fromUI(e)) return resolve(false);
  if (tunnels.has(port)) return resolve(true);
  const server = net.createServer((sock) => {
    const id = ++nextSocket;
    sockets.set(id, sock);
    if (win) win.webContents.send("tunnel:open", id, port);
    sock.on("data", (chunk) => { if (win) win.webContents.send("tunnel:data", id, chunk); });
    sock.on("close", () => { sockets.delete(id); if (win) win.webContents.send("tunnel:close", id); });
    sock.on("error", () => {});
  });
  server.once("error", () => resolve(false));
  server.listen(port, "127.0.0.1", () => { tunnels.set(port, server); resolve(true); });
}));
ipcMain.on("tunnel:write", (e, id, data) => { if (fromUI(e)) sockets.get(id)?.write(Buffer.from(data)); });
ipcMain.on("tunnel:end", (e, id) => { if (fromUI(e)) sockets.get(id)?.destroy(); });

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

// Chromium decodes video on the CPU on Linux unless VA-API is switched on. The
// device panel decodes a high-resolution H.264 stream, so let the GPU do it.
if (process.platform === "linux") app.commandLine.appendSwitch("enable-features", "AcceleratedVideoDecodeLinuxGL,AcceleratedVideoDecodeLinuxZeroCopyGL,VaapiIgnoreDriverChecks");

// One window per machine: a second launch focuses the first.
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", show);
  app.on("before-quit", () => { quitting = true; stopCore(); });
  app.whenReady().then(() => {
    Menu.setApplicationMenu(null);
    createTray();
    startAtLogin();
    integrateAppImage();
    // Auto-import refreshes the browser's sign-ins from the chosen browser.
    if (importSettings().auto) runImport("").catch(() => {});
    open().catch((err) => {
      dialog.showErrorBox("Dory", `No se pudo arrancar el núcleo.\n\n${err.message}`);
      app.quit();
    });
  });
  app.on("activate", show);
  // The window only hides; the app quits from the tray.
  app.on("window-all-closed", () => {});
}
