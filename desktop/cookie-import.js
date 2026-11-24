// Cookie import: copies the sign-ins of one of the user's browsers on Linux
// into the thread browser's session, so sites that refuse an embedded browser,
// such as Google, open already signed in. Firefox-based browsers (Zen,
// Firefox, LibreWolf, Floorp) keep cookies in plain text; Chromium-based ones
// (Chrome, Chromium, Brave) encrypt them with a key from the keyring. Windows
// Chrome 127+ binds its cookie key to Chrome itself, so no other app reads it.
const { execFile } = require("node:child_process");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const home = os.homedir();
const config = () => process.env.XDG_CONFIG_HOME || path.join(home, ".config");

// desktop is the name of the browser's .desktop entry, to recognise the
// system's default browser.
const SOURCES = [
  { id: "zen", name: "Zen", kind: "firefox", dir: () => path.join(home, ".zen"), desktop: /zen/i },
  { id: "firefox", name: "Firefox", kind: "firefox", dir: () => path.join(home, ".mozilla", "firefox"), desktop: /firefox/i },
  { id: "librewolf", name: "LibreWolf", kind: "firefox", dir: () => path.join(home, ".librewolf"), desktop: /librewolf/i },
  { id: "floorp", name: "Floorp", kind: "firefox", dir: () => path.join(home, ".floorp"), desktop: /floorp/i },
  { id: "chrome", name: "Google Chrome", kind: "chromium", dir: () => path.join(config(), "google-chrome"), app: "chrome", desktop: /google-chrome/i },
  { id: "chromium", name: "Chromium", kind: "chromium", dir: () => path.join(config(), "chromium"), app: "chromium", desktop: /chromium/i },
  { id: "brave", name: "Brave", kind: "chromium", dir: () => path.join(config(), "BraveSoftware", "Brave-Browser"), app: "brave", desktop: /brave/i },
];

// firefoxProfile is the profile the browser opens: the one its install
// section names, else the one marked default, else the first.
function firefoxProfile(dir) {
  let ini;
  try { ini = fs.readFileSync(path.join(dir, "profiles.ini"), "utf8"); } catch { return null; }
  const sections = [];
  for (const line of ini.split(/\r?\n/)) {
    const head = line.match(/^\[(.+)\]$/);
    if (head) sections.push({ name: head[1], kv: {} });
    else if (sections.length && line.includes("=")) { const i = line.indexOf("="); sections.at(-1).kv[line.slice(0, i)] = line.slice(i + 1); }
  }
  const resolve = (p, relative = true) => (relative ? path.join(dir, p) : p);
  const installed = sections.find((s) => s.name.startsWith("Install") && s.kv.Default);
  const candidates = [];
  if (installed) candidates.push(resolve(installed.kv.Default));
  for (const s of sections.filter((s) => s.name.startsWith("Profile"))) {
    const p = resolve(s.kv.Path, s.kv.IsRelative !== "0");
    if (s.kv.Default === "1") candidates.unshift(p); else candidates.push(p);
  }
  return candidates.find((p) => fs.existsSync(path.join(p, "cookies.sqlite"))) || null;
}

function cookiesFile(s) {
  if (s.kind === "firefox") { const p = firefoxProfile(s.dir()); return p && path.join(p, "cookies.sqlite"); }
  const f = path.join(s.dir(), "Default", "Cookies");
  return fs.existsSync(f) ? f : null;
}

const run = (cmd, args) => new Promise((resolve) => {
  execFile(cmd, args, { timeout: 10000 }, (err, out) => resolve(err ? "" : String(out).trim()));
});

// listSources returns the browsers this machine has and the system's default.
async function listSources() {
  if (process.platform !== "linux") return { sources: [], default: "" };
  const sources = SOURCES.filter((s) => cookiesFile(s)).map((s) => ({ id: s.id, name: s.name }));
  const entry = await run("xdg-settings", ["get", "default-web-browser"]);
  const def = SOURCES.find((s) => entry && s.desktop.test(entry) && sources.some((x) => x.id === s.id));
  return { sources, default: def ? def.id : sources[0]?.id || "" };
}

const derive = (pass) => crypto.pbkdf2Sync(pass, "saltysalt", 1, 16, "sha1");

function decrypt(buf, keys, hostKey, dbVersion) {
  const prefix = buf.subarray(0, 3).toString();
  const key = prefix === "v11" ? keys.v11 : prefix === "v10" ? keys.v10 : null;
  if (!key) return null;
  try {
    const d = crypto.createDecipheriv("aes-128-cbc", key, Buffer.alloc(16, " "));
    let out = Buffer.concat([d.update(buf.subarray(3)), d.final()]);
    // Since version 24 the value starts with the SHA-256 of its host.
    if (dbVersion >= 24 && out.length >= 32 && out.subarray(0, 32).equals(crypto.createHash("sha256").update(hostKey).digest())) out = out.subarray(32);
    return out.toString("utf8");
  } catch {
    return null;
  }
}

const SAME_SITE = { 0: "no_restriction", 1: "lax", 2: "strict" };

// chromiumRows reads a Chromium cookie database into neutral cookies.
async function chromiumRows(db, source) {
  // "peanuts" is Chromium's fixed key for v10 values written without a keyring.
  const pass = await run("secret-tool", ["lookup", "application", source.app]);
  const keys = { v10: derive("peanuts"), v11: pass ? derive(pass) : null };
  let dbVersion = 0;
  try { dbVersion = Number(db.prepare("SELECT value FROM meta WHERE key = 'version'").get()?.value) || 0; } catch {}
  const stmt = db.prepare("SELECT host_key, name, value, encrypted_value, path, expires_utc, is_secure, is_httponly, samesite, has_expires FROM cookies");
  stmt.setReadBigInts(true);
  return stmt.all().map((r) => ({
    host: r.host_key, name: r.name, path: r.path, secure: !!r.is_secure, httpOnly: !!r.is_httponly,
    sameSite: SAME_SITE[r.samesite],
    expires: r.has_expires ? Number(r.expires_utc) / 1e6 - 11644473600 : null,
    value: r.value || (r.encrypted_value?.length ? decrypt(Buffer.from(r.encrypted_value), keys, r.host_key, dbVersion) : ""),
  }));
}

// firefoxRows reads a Firefox cookie database; cookies of containers and
// partitioned storage stay out, as the thread browser has neither.
function firefoxRows(db) {
  const stmt = db.prepare("SELECT host, name, value, path, expiry, isSecure, isHttpOnly, sameSite FROM moz_cookies WHERE originAttributes = ''");
  stmt.setReadBigInts(true);
  return stmt.all().map((r) => {
    const expiry = Number(r.expiry);
    return {
      host: r.host, name: r.name, path: r.path, value: r.value, secure: !!r.isSecure, httpOnly: !!r.isHttpOnly,
      sameSite: SAME_SITE[r.sameSite],
      // Recent Firefox stores the expiry in milliseconds.
      expires: expiry > 1e11 ? expiry / 1000 : expiry,
    };
  });
}

// importCookies copies every unexpired cookie of the browser with the given
// id into ses and returns what it did.
async function importCookies(ses, id) {
  if (process.platform !== "linux") throw new Error("La importación de cookies solo está disponible en Linux.");
  const source = SOURCES.find((s) => s.id === id);
  const file = source && cookiesFile(source);
  if (!file) throw new Error("No se ha encontrado ese navegador en este equipo.");
  // The browser keeps its database locked while open: read a copy.
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dory-cookies-"));
  const copy = path.join(tmp, path.basename(file));
  try {
    fs.copyFileSync(file, copy);
    for (const ext of ["-wal", "-journal"]) if (fs.existsSync(file + ext)) fs.copyFileSync(file + ext, copy + ext);
    const { DatabaseSync } = require("node:sqlite");
    const db = new DatabaseSync(copy, { readOnly: true });
    let rows;
    try { rows = source.kind === "firefox" ? firefoxRows(db) : await chromiumRows(db, source); } finally { db.close(); }
    const now = Date.now() / 1000;
    let imported = 0, failed = 0;
    for (const r of rows) {
      if (r.expires !== null && r.expires < now) continue;
      if (r.value === null) { failed++; continue; }
      const host = r.host.replace(/^\./, "");
      const cookie = { url: `${r.secure ? "https" : "http"}://${host}${r.path || "/"}`, name: r.name, value: r.value, path: r.path || "/", secure: r.secure, httpOnly: r.httpOnly };
      // A leading dot marks a domain cookie; host-only cookies carry no domain.
      if (r.host.startsWith(".")) cookie.domain = r.host;
      if (r.sameSite) cookie.sameSite = r.sameSite;
      if (r.expires !== null) cookie.expirationDate = r.expires;
      try { await ses.cookies.set(cookie); imported++; } catch { failed++; }
    }
    await ses.cookies.flushStore();
    if (!imported && failed) throw new Error(`No se han podido leer las cookies de ${source.name}.`);
    return { browser: source.name, imported, failed };
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

module.exports = { listSources, importCookies };
