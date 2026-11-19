import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AppleLogo, Briefcase, Cloud, Code, Cpu, Database, Desktop, Flask, GameController, Globe, HardDrives,
  House, Laptop, LinuxLogo, Lightning, Planet, Rocket, Terminal, WindowsLogo, type Icon,
} from "@phosphor-icons/react";
import { api } from "../api/client";
import { ErrorText } from "./ui";

// The icons an environment can take; the id is what the node stores.
export const ENV_ICONS: Record<string, Icon> = {
  laptop: Laptop, desktop: Desktop, server: HardDrives, cloud: Cloud, terminal: Terminal, house: House,
  briefcase: Briefcase, cpu: Cpu, database: Database, globe: Globe, rocket: Rocket, flask: Flask,
  game: GameController, code: Code, lightning: Lightning, planet: Planet,
};

// osIcon stands in for an environment that has not picked an icon yet.
export const osIcon = (os = "") => (os === "darwin" ? AppleLogo : os === "windows" ? WindowsLogo : os === "linux" ? LinuxLogo : Desktop);

export const envIcon = (icon?: string, os?: string) => (icon && ENV_ICONS[icon]) || osIcon(os);

// EnvIcon draws an environment with its name as the tooltip.
export function EnvIcon({ icon, os, name, className = "size-3.5" }: { icon?: string; os?: string; name: string; className?: string }) {
  const I = envIcon(icon, os);
  return <span title={name} aria-label={name} className="inline-grid place-items-center"><I weight="duotone" className={className} /></span>;
}

// EnvIconPicker changes the icon of a node; calls for a peer reach it through the mesh.
export function EnvIconPicker({ node, icon, os, name }: { node: string; icon?: string; os?: string; name: string }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const save = useMutation({
    mutationFn: (id: string) => api.setNodeIcon(node, id),
    onSuccess: () => { setOpen(false); qc.invalidateQueries({ queryKey: ["node"] }); qc.invalidateQueries({ queryKey: ["mesh"] }); },
  });
  const Current = envIcon(icon, os);
  return (
    <span className="relative">
      <button type="button" onClick={() => setOpen(!open)} title={`Cambiar el icono de ${name}`} aria-label={`Cambiar el icono de ${name}`}
        className="grid size-8 place-items-center rounded-md hover:bg-slate-100 dark:hover:bg-raised"><Current weight="duotone" className="size-5" /></button>
      {open && (
        <div className="absolute left-0 top-9 z-20 w-56 rounded-lg border border-slate-200 bg-white p-2 shadow-lg dark:border-border dark:bg-surface">
          <div className="grid grid-cols-6 gap-1">
            {Object.entries(ENV_ICONS).map(([id, I]) => (
              <button key={id} type="button" disabled={save.isPending} onClick={() => save.mutate(id)} title={id}
                className={`grid size-8 place-items-center rounded-md hover:bg-slate-100 dark:hover:bg-raised ${id === icon ? "bg-slate-200 dark:bg-raised" : ""}`}>
                <I weight="duotone" className="size-5" />
              </button>
            ))}
          </div>
          <ErrorText error={save.error} />
        </div>
      )}
    </span>
  );
}
