import { ErrorText } from "../../components/ui";
import { SettingRow, useNodeSettings, type SectionProps } from "./settings-node";

// TaskSettings holds what the selected node(s) do when a task is done or
// cancelled.
export function TaskSettings({ node }: SectionProps) {
  const settings = useNodeSettings(node);
  const row = (field: Parameters<typeof SettingRow>[0]["field"], title: string, help: string) => <SettingRow settings={settings} field={field} title={title} help={help} />;
  return (
    <section aria-labelledby="tasks-title" className="space-y-4">
      <h2 id="tasks-title" className="text-lg font-semibold">Tareas e hilos</h2>
      {row("stop_services_on_shutdown", "Cerrar los servicios de los hilos al reiniciar Dory", "Detiene los servicios al cerrar o reiniciar el núcleo de este entorno.")}
      <h2 className="pt-2 text-lg font-semibold">Dispositivos</h2>
      {row("agents_use_devices", "Los agentes pueden usar dispositivos", "Los agentes de este nodo ven y controlan los móviles y emuladores Android (pantalla, toques, texto, apps). Un agente puede enviar mensajes o tocar datos reales del móvil: sus acciones se ven en directo en el panel Dispositivo y quedan en el hilo. Activado por defecto.")}
      <ErrorText error={settings.error} />
    </section>
  );
}
