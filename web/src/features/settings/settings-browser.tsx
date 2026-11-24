import { useEffect, useState } from "react";
import { Button, ErrorText, Select } from "../../components/ui";
import { SettingRow, useNodeSettings, type SectionProps } from "./settings-node";

// The ids are the keys of browser.SearchEngines in Go.
const ENGINES = [
  { value: "google", label: "Google" },
  { value: "duckduckgo", label: "DuckDuckGo" },
  { value: "bing", label: "Bing" },
  { value: "brave", label: "Brave Search" },
  { value: "startpage", label: "Startpage" },
];

type Sources = { sources: { id: string; name: string }[]; default: string };

// BrowserSettings holds the choices of the thread browser of the selected
// node(s): whether agents may use it, the search engine of its address bar
// and the tunnel; and, in the desktop app, where this machine's browser takes
// its sign-ins from (ADR-045).
export function BrowserSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  return (
    <div className="space-y-6">
      <section aria-labelledby="browser-title" className="space-y-4">
        <h2 id="browser-title" className="text-lg font-semibold">Navegador</h2>
        <SettingRow settings={settings} field="agents_use_browser" title="Permitir que los agentes usen el navegador integrado"
          help="Los agentes de este nodo pueden abrir el panel del navegador, navegar, leer páginas, pulsar elementos y hacer capturas con las herramientas del navegador. Desmarcado, no tienen esas herramientas. Activado por defecto." />
        <SettingRow settings={settings} field="browser_search_engine" title="Buscador"
          help="Lo que escribas en la barra de direcciones y no sea una dirección se busca con él."
          control={(value, set, disabled) => <Select className="w-48 shrink-0" value={value || "google"} onChange={set} options={ENGINES} disabled={disabled} />} />
        <SettingRow settings={settings} field="browser_tunnel_disabled" title="Bloquear el túnel del navegador" help="La app de escritorio no podrá abrir los puertos de localhost de este nodo desde el navegador de un hilo remoto. Los puertos de Dory nunca son accesibles." />
        <ErrorText error={settings.error} />
      </section>
      <CookieSettings />
    </div>
  );
}

// CookieSettings sets where the side browser of the desktop app takes its
// sign-ins from: one of this machine's browsers, imported now or at every
// start. It only exists in the desktop app, which keeps it.
function CookieSettings() {
  const bridge = window.doryDesktop?.browser;
  const [list, setList] = useState<Sources | null>(null);
  const [settings, setSettings] = useState<CookieImportSettings>({ source: "", auto: false });
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    bridge?.importSources?.().then((r) => { setList(r); setSettings({ ...r.settings, source: r.settings.source || r.default }); });
  }, [bridge]);

  if (!bridge?.importSources) return null;
  const save = async (next: CookieImportSettings) => { setSettings(next); await bridge.importSettings?.(next); };
  const importNow = async () => {
    setBusy(true);
    setStatus("");
    try {
      const r = await bridge.importCookies!(settings.source);
      setStatus(`Importadas ${r.imported} cookies de ${r.browser}${r.failed ? ` (${r.failed} no se pudieron leer)` : ""}.`);
    } catch (e) {
      setStatus(String(e instanceof Error ? e.message : e).replace(/^Error invoking remote method '[^']+': (Error: )?/, ""));
    } finally {
      setBusy(false);
    }
  };
  const options = (list?.sources ?? []).map((s) => ({ value: s.id, label: s.id === list?.default ? `${s.name} (predeterminado)` : s.name }));

  return (
    <section aria-labelledby="cookies-title" className="space-y-6">
      <div>
        <h2 id="cookies-title" className="text-lg font-semibold">Sesiones de este equipo</h2>
        <p className="text-xs text-slate-500 dark:text-muted">Solo la app de escritorio y solo para este equipo, en todos los nodos. Las cookies van a las pestañas que ves en la app; el navegador de los nodos, que usan los agentes en segundo plano, nunca las recibe.</p>
      </div>
      <div className="max-w-sm space-y-2">
        <span className="block text-sm font-medium">Importar sesiones desde</span>
        {list && !options.length
          ? <p className="text-sm text-slate-500 dark:text-muted">No se ha encontrado ningún navegador compatible (Zen, Firefox, LibreWolf, Floorp, Chrome, Chromium o Brave).</p>
          : <Select value={settings.source} onChange={(source) => save({ ...settings, source })} options={options} placeholder="Cargando…" />}
        <p className="text-xs text-slate-500 dark:text-muted">Copia las cookies de ese navegador al navegador lateral, para entrar ya con tu sesión en sitios que no dejan iniciarla en él, como Google.</p>
      </div>
      <div className="space-y-2">
        <Button onClick={importNow} disabled={busy || !settings.source}>{busy ? "Importando…" : "Importar ahora"}</Button>
        {status && <p className="text-xs text-slate-500 dark:text-muted">{status}</p>}
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input type="checkbox" className="mt-1" checked={settings.auto} disabled={!settings.source} onChange={(e) => save({ ...settings, auto: e.target.checked })} />
        <span>
          <span className="block font-medium">Importar al iniciar Dory</span>
          <span className="block text-xs text-slate-500 dark:text-muted">Cada vez que abres la app vuelve a copiar las cookies, así las sesiones nuevas de tu navegador llegan solas. Las sesiones cerradas allí siguen aquí hasta que caducan.</span>
        </span>
      </label>
    </section>
  );
}
