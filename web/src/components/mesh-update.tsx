import { ArrowCircleUp, CaretUp, Check, CircleNotch } from "@phosphor-icons/react";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { type NodeUpdate } from "../api/client";
import { qk } from "../api/queries/keys";
import { useMeshUpdates as useMeshUpdatesQuery, useUpdateMesh } from "../api/queries/settings";
import { askConfirm } from "./confirm";

interface DesktopUpdateEvent {
  phase: "verify" | "restart";
  percent: number;
  transferred?: number;
  total?: number;
}

interface UpdateStep {
  id: string;
  label: string;
  detail?: string;
  status: string;
}

type DesktopBridge = {
  onUpdateProgress?: (cb: (e: DesktopUpdateEvent) => void) => () => void;
};

// What a node has to update: its core and the harnesses with a newer release.
function pendingOf(n: NodeUpdate) {
  const st = n.status;
  if (!st) return { core: false, providers: [] as string[] };
  return { core: st.available, providers: st.providers.filter((p) => p.available).map((p) => `${p.name} ${p.current} → ${p.latest}`) };
}

// useMeshUpdates reads the update state of every connected node; it polls
// faster while one of them is updating.
export function useMeshUpdates(enabled = true) {
  return useMeshUpdatesQuery({
    enabled,
    refetchInterval: (q) => (q.state.data?.some((n) => n.status?.state === "updating") ? 1_000 : 5 * 60_000),
  });
}

// Card anchored to the right of the button and growing upward so its bottom
// edge never drops below the button itself.
function UpdateCard({ anchor, children }: { anchor: HTMLElement | null; children: React.ReactNode }) {
  const [pos, setPos] = useState<{ left: number; bottom: number; maxHeight: number } | null>(null);

  useLayoutEffect(() => {
    if (!anchor) return;
    const update = () => {
      const a = anchor.getBoundingClientRect();
      const gap = 8;
      const margin = 8;
      const bottom = Math.max(margin, window.innerHeight - a.bottom);
      const maxHeight = Math.max(120, window.innerHeight - bottom - margin);
      setPos({ left: a.right + gap, bottom, maxHeight });
    };
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [anchor]);

  if (!anchor || !pos) return null;
  return createPortal(
    <div
      role="status"
      style={{ left: pos.left, bottom: pos.bottom, maxHeight: pos.maxHeight }}
      className="tip-in fixed z-[100] overflow-y-auto rounded-lg bg-slate-900 text-xs text-white shadow-xl dark:bg-raised dark:text-text"
    >
      {children}
    </div>,
    anchor.closest("dialog") ?? document.body,
  );
}

// Circular progress ring shown inside the 32x32 sidebar button.
function ProgressRing({ progress, done }: { progress: number; done?: boolean }) {
  const size = 26;
  const half = size / 2;
  const stroke = 2.25;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const pct = Math.max(0, Math.min(100, progress));
  const offset = c * (1 - pct / 100);

  if (done) {
    return (
      <span className="relative inline-flex size-[26px] items-center justify-center">
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="block">
          <circle cx={half} cy={half} r={r} fill="none" strokeWidth={stroke} className="stroke-emerald-500" />
        </svg>
        <Check weight="bold" className="absolute size-3 text-emerald-500" aria-hidden="true" />
      </span>
    );
  }

  return (
    <span className="relative inline-flex size-[26px] items-center justify-center">
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="block">
        <circle cx={half} cy={half} r={r} fill="none" strokeWidth={stroke} className="stroke-current opacity-20" />
        <circle
          cx={half}
          cy={half}
          r={r}
          fill="none"
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={offset}
          transform={`rotate(-90 ${half} ${half})`}
          className="stroke-blue-500 transition-[stroke-dashoffset] duration-150 ease-out"
        />
        <text
          x={half}
          y={half}
          textAnchor="middle"
          dominantBaseline="central"
          className="fill-current text-[8.5px] font-semibold tabular-nums"
        >
          {Math.round(pct)}
        </text>
      </svg>
    </span>
  );
}

function mergeDesktopProgress(steps: UpdateStep[], percent: number, desktop: DesktopUpdateEvent | null): { steps: UpdateStep[]; percent: number } {
  if (!desktop) return { steps, percent };
  const baseSteps: UpdateStep[] = steps.length
    ? steps.map((s) => ({ ...s }))
    : [
        { id: "download", label: "Descargando actualización", status: "done" },
        { id: "stage", label: "Preparando archivos", status: "done" },
        { id: "verify", label: "Verificando paquete", status: "pending" },
        { id: "restart", label: "Reiniciando Dory", status: "pending" },
      ];

  const markDone = (ids: string[]) => {
    for (const s of baseSteps) if (ids.includes(s.id)) s.status = "done";
  };

  if (desktop.phase === "verify") {
    markDone(["providers", "check", "download", "stage"]);
    const detail = desktop.total && desktop.total > 0
      ? `${((desktop.transferred ?? 0) / (1024 * 1024)).toFixed(1)} / ${(desktop.total / (1024 * 1024)).toFixed(1)} MB · ${desktop.percent}%`
      : `${desktop.percent}%`;
    const verify = baseSteps.find((s) => s.id === "verify");
    if (verify) {
      verify.status = "active";
      verify.detail = detail;
    }
    const overall = 82 + Math.round((Math.min(100, Math.max(0, desktop.percent)) / 100) * 13);
    return { steps: baseSteps, percent: Math.max(percent, overall) };
  }

  markDone(["providers", "check", "download", "stage", "verify"]);
  const restart = baseSteps.find((s) => s.id === "restart");
  if (restart) {
    restart.status = "active";
    restart.detail = "Aplicando cambios…";
  }
  return { steps: baseSteps, percent: 98 };
}

// MeshUpdateButton sits right of the commit in the sidebar. It shows only when
// a connected node, or one of its harnesses, has a newer release (ADR-016).
export function MeshUpdateButton() {
  const qc = useQueryClient();
  const list = useMeshUpdates();
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [hover, setHover] = useState(false);
  const [pinned, setPinned] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [desktopProgress, setDesktopProgress] = useState<DesktopUpdateEvent | null>(null);
  const cardRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const bridge = (window as unknown as { doryDesktop?: DesktopBridge }).doryDesktop;
    if (!bridge?.onUpdateProgress) return;
    return bridge.onUpdateProgress((ev) => setDesktopProgress(ev));
  }, []);

  useEffect(() => {
    if (!pinned) return;
    const onDown = (e: PointerEvent) => {
      const target = e.target as Node | null;
      if (anchor?.contains(target) || cardRef.current?.contains(target)) return;
      setPinned(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [pinned, anchor]);

  const nodes = list.data ?? [];
  const rows = nodes.map((n) => ({ n, p: pendingOf(n) })).filter((r) => r.p.core || r.p.providers.length);
  const updatingNode = nodes.find((n) => n.status?.state === "updating");
  const working = Boolean(updatingNode) || desktopProgress !== null;
  const providers = rows.some((r) => r.p.providers.length > 0);
  const run = useUpdateMesh({ providers, force: false }, {
    onSuccess: (data) => qc.setQueryData(qk.meshUpdate(), data),
  });

  if (!rows.length && !working) return null;

  const busy = working || run.isPending;
  const showCard = busy ? pinned : hover;

  const rawSteps: UpdateStep[] = updatingNode?.status?.progress?.steps ?? [];
  const rawPercent = updatingNode?.status?.progress?.percent ?? 0;
  const { steps, percent } = mergeDesktopProgress(rawSteps, rawPercent, desktopProgress);
  const activeStep = steps.find((s) => s.status === "active");

  const start = async () => {
    if (busy) {
      setPinned((p) => !p);
      return;
    }
    setHover(false);
    const turns = nodes.reduce((a, n) => a + (n.status?.active_turns ?? 0), 0);
    const note = turns ? ` Los nodos con turnos en marcha (${turns}) se saltarán.` : "";
    if (await askConfirm(`¿Actualizar ${rows.length === 1 ? "1 nodo" : `${rows.length} nodos`}? Cada nodo se reinicia al terminar.${note}`, "Actualizar")) {
      setPinned(false);
      setExpanded(false);
      run.mutate();
    }
  };

  return (
    <>
      <button
        ref={setAnchor}
        type="button"
        aria-label={busy ? "Actualizando Dory" : "Actualizar la malla"}
        onMouseEnter={() => setHover(true)}
        onMouseLeave={() => setHover(false)}
        onFocus={() => setHover(true)}
        onBlur={() => setHover(false)}
        onClick={start}
        className="relative grid size-8 shrink-0 place-items-center rounded-md hover:bg-slate-200 dark:hover:bg-raised"
      >
        {busy ? (
          <ProgressRing progress={percent} />
        ) : (
          <>
            <ArrowCircleUp weight="regular" className="size-5" aria-hidden="true" />
            <span className="absolute right-1 top-1 size-2 rounded-full bg-blue-500 ring-2 ring-slate-100 dark:ring-surface" aria-hidden="true" />
          </>
        )}
      </button>

      {showCard && (
        <UpdateCard anchor={anchor}>
          <div ref={cardRef}>
            {busy ? (
              <div className="w-60 overflow-hidden rounded-lg">
                {steps.length > 0 && (
                  <div
                    className={`grid transition-[grid-template-rows,opacity] duration-200 ease-out ${
                      expanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
                    }`}
                  >
                    <div className="overflow-hidden">
                      <ul className="space-y-1.5 border-b border-white/10 px-3 py-2.5">
                        {steps.map((step) => (
                          <li key={step.id} className="flex items-start gap-2">
                            <span className="mt-0.5 grid size-3.5 shrink-0 place-items-center">
                              {step.status === "done" ? (
                                <Check weight="bold" className="size-3 text-emerald-400" aria-hidden="true" />
                              ) : step.status === "active" ? (
                                <CircleNotch weight="bold" className="size-3 animate-spin text-blue-400" aria-hidden="true" />
                              ) : (
                                <span className="size-1.5 rounded-full bg-current opacity-30" />
                              )}
                            </span>
                            <div className="min-w-0 flex-1">
                              <div className={step.status === "pending" ? "opacity-45" : step.status === "done" ? "opacity-70" : "font-medium"}>
                                {step.label}
                              </div>
                              {step.detail && step.status !== "pending" && (
                                <div className="truncate text-[11px] opacity-60 tabular-nums">{step.detail}</div>
                              )}
                            </div>
                          </li>
                        ))}
                      </ul>
                    </div>
                  </div>
                )}

                <button
                  type="button"
                  onClick={() => steps.length > 0 && setExpanded((e) => !e)}
                  aria-expanded={expanded}
                  className="flex w-full cursor-pointer flex-col gap-2 px-3 py-2.5 text-left transition-colors hover:bg-white/10"
                >
                  <div className="flex w-full items-center justify-between gap-2">
                    <span className="truncate font-medium">
                      {activeStep ? activeStep.label : "Actualizando…"}
                    </span>
                    <span className="flex shrink-0 items-center gap-1 tabular-nums opacity-70">
                      <span>{Math.round(percent)}%</span>
                      {steps.length > 0 && (
                        <CaretUp
                          className={`size-3 transition-transform duration-200 ease-out ${expanded ? "rotate-180" : ""}`}
                          aria-hidden="true"
                        />
                      )}
                    </span>
                  </div>

                  <div className="h-1 w-full overflow-hidden rounded-full bg-white/15">
                    <div
                      className="h-full rounded-full bg-blue-500 transition-[width] duration-150 ease-out"
                      style={{ width: `${Math.round(percent)}%` }}
                    />
                  </div>
                </button>
              </div>
            ) : (
              <div className="space-y-1 px-3 py-2">
                <div className="font-medium">Actualizar la malla</div>
                {rows.map(({ n, p }) => (
                  <div key={n.node}>
                    <div className="font-medium opacity-90">{n.name}</div>
                    {p.core && <div className="opacity-70">Dory {n.status?.current} → {n.status?.latest}</div>}
                    {p.providers.map((t) => <div key={t} className="opacity-70">{t}</div>)}
                  </div>
                ))}
                {run.error && <div className="mt-1 text-red-500">{(run.error as Error).message}</div>}
              </div>
            )}
          </div>
        </UpdateCard>
      )}
    </>
  );
}
