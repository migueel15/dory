import { SCALES, setScale, useScale } from "../scale";

// AppearanceSettings changes the scale of the whole interface on this device.
export function AppearanceSettings() {
  const scale = useScale();
  const i = SCALES.indexOf(scale);
  return (
    <div className="space-y-4 p-4">
      <div className="space-y-2">
        <label htmlFor="ui-scale" className="flex items-center text-sm font-medium">
          Escala de la interfaz
          <span className="ml-auto font-mono text-xs text-slate-500 dark:text-muted">{Math.round(scale * 100)}%</span>
        </label>
        <input id="ui-scale" type="range" min={0} max={SCALES.length - 1} step={1} value={i}
          onChange={(e) => setScale(SCALES[Number(e.target.value)])} className="w-full max-w-sm" />
        <p className="text-xs text-slate-500 dark:text-muted">
          También con Ctrl + / Ctrl − y Ctrl 0 para volver al 100% (Cmd en macOS). Se guarda en este navegador.
        </p>
      </div>
    </div>
  );
}
