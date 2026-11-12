(() => {
  const $ = (id) => document.getElementById(id);
  const msg = (m, bad) => { $('msg').textContent = m || ''; $('msg').className = bad ? 'bad' : ''; };
  let stream = null, scanning = false;

  try { $('name').value = (window.DoryNative && window.DoryNative.deviceName()) || ''; } catch (e) {}

  async function join(payload) {
    stopScan();
    $('join').disabled = true;
    msg('Emparejando… acepta la solicitud en el otro dispositivo.');
    try {
      const res = await fetch('/node/api/join', {
        method: 'POST', credentials: 'same-origin',
        headers: { 'X-Dory-Node': '1', 'Content-Type': 'application/json' },
        body: JSON.stringify({ payload, name: $('name').value.trim() }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || res.status + ' ' + res.statusText);
      msg('Listo. Abriendo Dory…');
      location.replace('/');
    } catch (e) {
      msg(e.message, true);
      $('join').disabled = false;
    }
  }

  async function scan() {
    msg('');
    const v = $('video');
    try {
      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) throw new Error('la cámara no está disponible aquí');
      stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false });
      v.srcObject = stream; v.style.display = 'block'; await v.play();
    } catch (e) { msg('No se pudo abrir la cámara (' + e.message + '). Pega el contenido del QR.', true); stopScan(); return; }
    $('stop').hidden = false;
    let det = null;
    if ('BarcodeDetector' in window) { try { det = new BarcodeDetector({ formats: ['qr_code'] }); } catch (e) { det = null; } }
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    scanning = true;
    while (scanning) {
      if (v.videoWidth) {
        canvas.width = v.videoWidth; canvas.height = v.videoHeight;
        ctx.drawImage(v, 0, 0);
        let text = null;
        if (det) {
          try { const r = await det.detect(canvas); if (r.length) text = r[0].rawValue; } catch (e) { det = null; }
        } else {
          const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
          const c = window.jsQR(img.data, img.width, img.height, { inversionAttempts: 'dontInvert' });
          if (c) text = c.data;
        }
        if (text) { $('payload').value = text; await join(text); return; }
      }
      await new Promise((r) => setTimeout(r, 120));
    }
  }

  function stopScan() {
    scanning = false;
    if (stream) stream.getTracks().forEach((t) => t.stop());
    stream = null;
    $('video').style.display = 'none';
    $('stop').hidden = true;
  }

  $('scan').onclick = scan;
  $('stop').onclick = stopScan;
  $('join').onclick = () => { const p = $('payload').value.trim(); if (!p) { msg('Pega el contenido del QR.', true); return; } join(p); };
})();
