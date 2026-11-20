// Launcher of the Android app: lists paired environments, finds others on
// the local network (UDP beacon) or in the mesh of a paired one (Tailscale
// addresses included), pairs with a code and opens the chosen node.
const Dory = window.Capacitor.Plugins.Dory;
const KEY = "dory.environments";
const PORT = 7710;
const CAPTURE_NODE = "dory.capture-node";

let saved = JSON.parse(localStorage.getItem(KEY) || "[]");
const online = new Map(); // id → reachable now
const found = new Map(); // id → {id, name, address, via}

const $ = (id) => document.getElementById(id);
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

$("capture-node").onchange = (event) => {
  captureNode = event.target.value;
  store();
};
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

  const pending = [...found.values()].filter((f) => !saved.some((s) => s.id === f.id));
  $("found-box").hidden = pending.length === 0;
  $("found").replaceChildren(
    ...pending.map((f) => {
      const li = el("li");
      const text = el("div", "grow");
      text.append(el("div", "name", f.name), el("div", "sub", `${f.address} · ${f.via}`));
      li.append(el("span", "dot on"), text);
      li.onclick = () => {
        $("address").value = f.address;
        $("code").focus();
      };
      return li;
    }),
  );
}

async function open(env) {
  $("error").textContent = "";
  try {
    await Dory.open({ address: env.address, pin: env.pin, token: env.token });
  } catch (e) {
    $("error").textContent = `${env.name}: ${e.message}`;
  }
}

// check sees whether a paired node answers and learns the other nodes of its
// mesh, whose addresses also work from outside home through Tailscale.
async function check(env) {
  try {
    const { body } = await Dory.get({ address: env.address, pin: env.pin, token: env.token, path: "/api/mesh" });
    online.set(env.id, true);
    for (const p of JSON.parse(body).peers || []) {
      if (p.revoked || !p.address) continue;
      const host = p.address.replace(/:\d+$/, "");
      found.set(p.id, { id: p.id, name: p.name, address: withPort(host), via: `malla de ${env.name}` });
    }
  } catch {
    online.set(env.id, false);
  }
  render();
}

$("pair").onsubmit = async (e) => {
  e.preventDefault();
  const button = e.target.querySelector("button");
  button.disabled = true;
  $("error").textContent = "";
  try {
    const env = await Dory.pair({ address: withPort($("address").value.trim()), code: $("code").value.trim() });
    saved = [...saved.filter((s) => s.id !== env.id), env];
    store();
    $("address").value = $("code").value = "";
    online.set(env.id, true);
    render();
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
  found.set(f.id, { ...f, via: "red local" });
  render();
});

store();
render();
Dory.startDiscovery();
saved.forEach(check);
setInterval(() => saved.forEach(check), 30_000);
