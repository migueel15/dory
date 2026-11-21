// Launcher of the Android app: lists paired environments, finds others on
// the local network (UDP beacon) or in the mesh of a paired one (Tailscale
// addresses included), pairs with a code and opens the chosen node.
const Dory = window.Capacitor.Plugins.Dory;
const KEY = "dory.environments";
const PORT = 7710;
const CAPTURE_NODE = "dory.capture-node";

let saved = JSON.parse(localStorage.getItem(KEY) || "[]");
const online = new Map(); // id → reachable now
const localFound = new Map(); // id → anuncio reciente de la red local
const meshFound = new Map(); // nodo emparejado → última lista de pares conectados
const checkVersions = new Map();
const LOCAL_TTL = 30_000;
const MESH_TTL = 60_000;
const DISCOVERED_TTL = 75_000;

const $ = (id) => document.getElementById(id);
let settingsPushed = false;
function showPage() {
  const settings = location.hash === "#settings";
  if (!settings) settingsPushed = false;
  $("home-view").hidden = settings;
  $("settings-view").hidden = !settings;
  $("open-settings").hidden = settings;
  $("back-home").hidden = !settings;
  $("page-title").textContent = settings ? "Ajustes" : "Dory";
  if (location.hash !== "#pair" && $("pair-dialog").open) $("pair-dialog").close();
}
$("open-settings").onclick = () => { settingsPushed = true; location.hash = "settings"; };
$("back-home").onclick = () => {
  if (settingsPushed) history.back();
  else { history.replaceState(null, "", location.pathname + location.search); showPage(); }
};
window.addEventListener("hashchange", showPage);
showPage();
function openPair(environment) {
  $("pair").reset();
  $("error").textContent = "";
  $("address-field").hidden = Boolean(environment);
  $("address").value = environment ? environment.address : "";
  $("pair-title").textContent = environment ? `Emparejar ${environment.name}` : "Emparejar entorno";
  location.hash = "pair";
  $("pair-dialog").showModal();
  (environment ? $("code") : $("address")).focus();
}
function closePair() {
  if (location.hash === "#pair") history.back();
  else if ($("pair-dialog").open) $("pair-dialog").close();
}
$("pair-manual").onclick = () => openPair(null);
$("pair-close").onclick = closePair;
$("pair-dialog").addEventListener("cancel", event => { event.preventDefault(); closePair(); });


let captureNode = localStorage.getItem(CAPTURE_NODE) || "";
let settingsSaved = Promise.resolve();

function saveCaptureSettings() {
  const nodes = saved.map(node => ({ ...node }));
  const destination = captureNode;
  settingsSaved = settingsSaved.then(async () => {
    const error = $("capture-error");
    try {
      await Dory.saveCaptureSettings({ nodes, destination });
      error.hidden = true;
    } catch (e) {
      error.textContent = e.message || "No se pudieron guardar los ajustes de dictado.";
      error.hidden = false;
    }
  });
}

function store() {
  if (!saved.some(node => node.id === captureNode)) captureNode = "";
  localStorage.setItem(KEY, JSON.stringify(saved));
  localStorage.setItem(CAPTURE_NODE, captureNode);
  saveCaptureSettings();
}

$("choose-assistant").onclick = async () => {
  try {
    await Dory.openAssistantSettings();
  } catch (error) {
    $("capture-error").textContent = error.message || "No se pudo abrir la configuración del asistente.";
    $("capture-error").hidden = false;
  }
};

$("capture-node").onchange = (event) => {
  captureNode = event.target.value;
  store();
};
let voiceModels = null;
let voiceModelBusy = false;

async function refreshVoiceModels() {
  try {
    voiceModels = await Dory.assistantModels();
    renderVoiceModels();
  } catch (error) {
    $("voice-model-status").textContent = error.message || "No se pudieron cargar los modelos.";
  }
}

function renderVoiceModels() {
  if (!voiceModels) return;
  const choices = [
    { id: "android", name: "Android", description: "Reconocedor del sistema", installed: true },
    { id: "tiny", name: "Moonshine Tiny", description: "Modelo local ligero · español", installed: voiceModels.tiny },
    { id: "small", name: "Moonshine Small", description: "Modelo local · español", installed: voiceModels.small },
  ];
  $("voice-model-list").replaceChildren(...choices.filter(choice => choice.id === "android" || voiceModels.supported).map(choice => {
    const row = el("li");
    const text = el("div", "grow");
    text.append(el("div", "name", choice.name), el("div", "model-description", choice.description));
    row.append(text);
    const button = el("button", "model-action", choice.id === voiceModels.selected ? "Activo" : choice.installed ? "Activar" : "Descargar");
    button.disabled = voiceModelBusy || choice.id === voiceModels.selected;
    button.onclick = async () => {
      voiceModelBusy = true;
      renderVoiceModels();
      try {
        if (choice.installed) {
          await Dory.selectAssistantModel({ model: choice.id });
          $("voice-model-status").textContent = choice.name + " activado.";
        } else {
          $("voice-model-status").textContent = "Preparando descarga…";
          await Dory.downloadAssistantModel({ model: choice.id });
          $("voice-model-status").textContent = choice.name + " descargado. Puedes activarlo.";
        }
        await refreshVoiceModels();
      } catch (error) {
        $("voice-model-status").textContent = error.message || "No se pudo completar la operación.";
      } finally {
        voiceModelBusy = false;
        renderVoiceModels();
      }
    };
    row.append(button);
    return row;
  }));
}

Dory.addListener("assistantModelProgress", progress => {
  const fraction = progress.percent >= 0 ? ` · ${progress.percent}%` : "";
  $("voice-model-status").textContent = `Descargando archivo ${progress.file} de ${progress.files}${fraction}`;
});
refreshVoiceModels();

const withPort = (a) => (/:\d+$/.test(a) || a.startsWith("[") ? a : `${a}:${PORT}`);

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

function render() {
  const list = $("saved");
  list.replaceChildren();
  $("none").hidden = saved.length > 0;
  for (const env of saved) {
    const li = el("li");
    li.append(el("span", "dot" + (online.get(env.id) ? " on" : "")));
    const text = el("div", "grow");
    text.append(el("div", "name", env.name), el("div", "sub", env.address));
    li.append(text);
    const forget = el("button", "forget", "Olvidar");
    forget.onclick = (e) => {
      e.stopPropagation();
      if (!confirm(`¿Olvidar ${env.name}? Tendrás que emparejarlo de nuevo.`)) return;
      Dory.forget({ address: env.address });
      saved = saved.filter((s) => s.id !== env.id);
      meshFound.delete(env.id);
      store();
      render();
    };
    li.append(forget);
    li.onclick = () => open(env);
    list.append(li);
  }
  $("capture-setting").hidden = saved.length < 2;
  const captureSelect = $("capture-node");
  const placeholder = el("option", null, "Elige el nodo");
  placeholder.value = "";
  captureSelect.replaceChildren(placeholder, ...saved.map(node => {
    const option = el("option", null, node.name || node.address);
    option.value = node.id;
    return option;
  }));
  captureSelect.value = captureNode;

  const now = Date.now();
  const found = new Map();
  for (const source of meshFound.values()) {
    if (now - source.at <= MESH_TTL) {
      for (const entry of source.peers) {
        if (!entry.expires || now <= entry.expires) found.set(entry.id, entry);
      }
    }
  }
  for (const [id, entry] of localFound) {
    if (now - entry.at <= LOCAL_TTL) found.set(id, entry);
    else localFound.delete(id);
  }
  const pending = [...found.values()].filter((f) => !saved.some((s) => s.id === f.id));
  $("found-none").hidden = pending.length > 0;
  $("found").replaceChildren(
    ...pending.map((f) => {
      const li = el("li");
      const text = el("div", "grow");
      text.append(el("div", "name", f.name), el("div", "sub", `${f.address} · ${f.via}`));
      li.append(el("span", "dot on"), text);
      li.onclick = () => openPair(f);
      return li;
    }),
  );
}

async function open(env) {
  $("open-error").textContent = "";
  try {
    await Dory.open({ address: env.address, pin: env.pin, token: env.token });
  } catch (e) {
    $("open-error").textContent = `${env.name}: ${e.message}`;
  }
}

// check sees whether a paired node answers and learns the other nodes of its
// mesh, whose addresses also work from outside home through Tailscale.
async function check(env) {
  const version = (checkVersions.get(env.id) || 0) + 1;
  checkVersions.set(env.id, version);
  try {
    const { body } = await Dory.get({ address: env.address, pin: env.pin, token: env.token, path: "/api/mesh" });
    if (checkVersions.get(env.id) !== version || !saved.some(node => node.id === env.id)) return;
    const snapshot = JSON.parse(body);
    const peers = (snapshot.peers || [])
      .filter(peer => peer.connected === true && !peer.revoked && peer.address)
      .map(peer => {
        const host = peer.address.replace(/:\d+$/, "");
        return { id: peer.id, name: peer.name, address: withPort(host), via: `malla de ${env.name}` };
      });
    for (const node of snapshot.discovered || []) {
      const seen = Date.parse(node.seen);
      if (!node.id || !node.address || !Number.isFinite(seen) || Date.now() - seen > DISCOVERED_TTL) continue;
      const host = node.address.replace(/:\d+$/, "");
      peers.push({ id: node.id, name: node.name, address: withPort(host), via: `malla de ${env.name}`, expires: seen + DISCOVERED_TTL });
    }
    online.set(env.id, true);
    meshFound.set(env.id, { at: Date.now(), peers });
  } catch {
    if (checkVersions.get(env.id) !== version || !saved.some(node => node.id === env.id)) return;
    online.set(env.id, false);
    meshFound.delete(env.id);
  }
  render();
}

let refreshing = false;
let pullStart = null;
let pullDistance = 0;

async function refreshEnvironments() {
  if (refreshing) return;
  refreshing = true;
  const indicator = $("refresh-indicator");
  indicator.hidden = false;
  indicator.setAttribute("aria-label", "Actualizando entornos");
  indicator.classList.add("refreshing");
  $("refresh-icon").style.transform = "";
  try {
    await Promise.all([Promise.allSettled(saved.map(check)), new Promise(resolve => setTimeout(resolve, 700))]);
    render();
  } finally {
    refreshing = false;
    indicator.hidden = true;
    indicator.classList.remove("refreshing");
    indicator.setAttribute("aria-label", "Actualizar entornos");
  }
}

document.addEventListener("touchstart", event => {
  if (refreshing || location.hash || $("pair-dialog").open || window.scrollY > 0 || event.touches.length !== 1) return;
  pullStart = { x: event.touches[0].clientX, y: event.touches[0].clientY };
  pullDistance = 0;
}, { passive: true });

document.addEventListener("touchmove", event => {
  if (!pullStart || event.touches.length !== 1) return;
  const dx = event.touches[0].clientX - pullStart.x;
  const dy = event.touches[0].clientY - pullStart.y;
  if (dy < 0 || Math.abs(dx) > dy) {
    pullStart = null;
    $("refresh-indicator").hidden = true;
    return;
  }
  if (dy < 12) return;
  event.preventDefault();
  pullDistance = dy;
  const indicator = $("refresh-indicator");
  indicator.hidden = false;
  indicator.setAttribute("aria-label", dy >= 72 ? "Suelta para actualizar" : "Desliza para actualizar");
  $("refresh-icon").style.transform = `rotate(${Math.min(dy / 72, 1) * 270}deg)`;
}, { passive: false });

function endPull() {
  if (!pullStart) return;
  pullStart = null;
  if (pullDistance >= 72) refreshEnvironments();
  else if (!refreshing) $("refresh-indicator").hidden = true;
  pullDistance = 0;
}
document.addEventListener("touchend", endPull, { passive: true });
document.addEventListener("touchcancel", () => {
  pullStart = null;
  pullDistance = 0;
  if (!refreshing) $("refresh-indicator").hidden = true;
}, { passive: true });

$("pair").onsubmit = async (e) => {
  e.preventDefault();
  const button = $("pair-submit");
  button.disabled = true;
  $("error").textContent = "";
  try {
    const env = await Dory.pair({ address: withPort($("address").value.trim()), code: $("code").value.trim() });
    saved = [...saved.filter((s) => s.id !== env.id), env];
    store();
    $("address").value = $("code").value = "";
    online.set(env.id, true);
    render();
    closePair();
    check(env);
  } catch (err) {
    $("error").textContent = err.message;
  } finally {
    button.disabled = false;
  }
};

Dory.addListener("found", (f) => {
  const known = saved.find((s) => s.id === f.id);
  if (known) {
    // A paired node changed its address (DHCP): follow it.
    if (known.address !== f.address) {
      known.address = f.address;
      store();
      check(known);
    }
    if (!online.get(known.id)) check(known);
    return;
  }
  localFound.set(f.id, { ...f, via: "red local", at: Date.now() });
  render();
});

store();
render();
Dory.startDiscovery();
saved.forEach(check);
setInterval(() => saved.forEach(check), 30_000);
setInterval(render, 5_000);
