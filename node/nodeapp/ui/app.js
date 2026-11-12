(() => {
  const $ = (id) => document.getElementById(id);
  const err = (m) => { $('err').textContent = m || ''; };
  async function call(method, path, body) {
    const res = await fetch('/node/api/' + path, {
      method,
      credentials: 'same-origin',
      headers: { 'X-Dory-Node': '1', 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || res.status + ' ' + res.statusText);
    return data;
  }
  const act = (fn) => async (...a) => { try { err(''); await fn(...a); await refresh(); } catch (e) { err(e.message); } };
  const text = (tag, s, cls) => { const e = document.createElement(tag); e.textContent = s; if (cls) e.className = cls; return e; };
  const label = { self: 'este dispositivo', connected: 'conectado', seen: 'visto', disconnected: 'desconectado' };

  let expiresTimer = null, lastState = null;

  function renderMembers(s) {
    const tb = $('members');
    tb.replaceChildren();
    for (const m of s.members) {
      const tr = document.createElement('tr');
      tr.append(text('td', m.name || m.device_id));
      const st = document.createElement('td');
      st.className = m.state;
      st.append(text('span', '', 'dot'), document.createTextNode((label[m.state] || m.state) + (m.state === 'seen' ? ' hace ' + m.ago_seconds + ' s' : '')));
      tr.append(st, text('td', m.manual || '', 'mut'));
      const act_ = document.createElement('td');
      if (!m.self) {
        const b = text('button', 'Revocar', 'danger');
        b.onclick = act(async () => {
          if (!confirm('¿Revocar «' + (m.name || m.device_id) + '»? Dejará de poder conectarse.')) return;
          await call('POST', 'members/' + encodeURIComponent(m.device_id) + '/revoke');
        });
        act_.append(b);
      }
      tr.append(act_);
      tb.append(tr);
    }
  }

  function renderPrompt(s) {
    const box = $('prompt');
    const key = s.pending ? s.pending.id : '';
    if (box.dataset.id === key) return;
    box.dataset.id = key;
    box.replaceChildren();
    if (!s.pending) return;
    const div = document.createElement('div');
    div.className = 'box prompt';
    div.append(text('div', '«' + s.pending.name + '» quiere unirse a este grupo.'), text('code', 'Clave ' + s.pending.public_key.slice(0, 16) + '…', 'mut'));
    const row = document.createElement('div');
    row.className = 'row';
    row.style.marginTop = '8px';
    const yes = text('button', 'Aceptar', 'primary'), no = text('button', 'Rechazar');
    yes.onclick = act(() => call('POST', 'pending', { id: s.pending.id, accept: true }));
    no.onclick = act(() => call('POST', 'pending', { id: s.pending.id, accept: false }));
    row.append(yes, no);
    div.append(row);
    box.append(div);
  }

  async function refresh() {
    try {
      const s = await call('GET', 'state');
      lastState = s;
      $('self').textContent = s.device.name + ' · ' + s.device.device_id;
      renderMembers(s);
      renderPrompt(s);
      if (document.activeElement !== $('name')) $('name').value = s.device.name;
      if (document.activeElement !== $('addr')) $('addr').value = s.device.manual || '';
      $('pair-close').hidden = !s.pairing.open;
      if (!s.pairing.open) $('qr').replaceChildren();
    } catch (e) { err(e.message); }
  }

  $('pair').onclick = act(async () => {
    const o = await call('POST', 'pair');
    const img = document.createElement('img');
    img.className = 'qr'; img.alt = 'Código QR para añadir un dispositivo'; img.src = o.png;
    const p = text('p', '', 'mut'); p.id = 'expires';
    const pl = text('code', o.payload);
    $('qr').replaceChildren(img, p, pl);
    clearInterval(expiresTimer);
    const end = new Date(o.expires).getTime();
    const tick = () => { const left = Math.max(0, Math.round((end - Date.now()) / 1000)); p.textContent = 'Ventana abierta ' + Math.floor(left / 60) + ':' + String(left % 60).padStart(2, '0') + '. Escanéalo con el otro dispositivo y confirma aquí.'; };
    tick(); expiresTimer = setInterval(tick, 1000);
  });
  $('pair-close').onclick = act(() => call('DELETE', 'pair'));
  $('name-form').onsubmit = (e) => { e.preventDefault(); act(() => call('PUT', 'device', { name: $('name').value }))(); };
  $('addr-form').onsubmit = (e) => { e.preventDefault(); act(() => call('PUT', 'address', { manual: $('addr').value }))(); };

  refresh();
  setInterval(refresh, 2000);
})();
