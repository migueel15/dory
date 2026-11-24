import { useState } from "react";
import { ArrowSquareOut, Eye, EyeSlash, LockKey } from "@phosphor-icons/react";
import { type PendingRequest } from "../../api/client";
import { ErrorText } from "../../components/ui";

// SecretPanel is a request_secret form inside the composer. The values live
// only in this component's state: never in the draft, the query cache or
// local storage, and they are cleared once sent.

const primary = "inline-flex h-8 items-center rounded-full bg-slate-900 px-4 text-sm font-medium text-white transition hover:bg-slate-700 disabled:bg-slate-200 disabled:text-slate-400 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200 dark:disabled:bg-raised dark:disabled:text-muted";
const ghost = "inline-flex h-8 items-center rounded-full px-3 text-sm text-slate-600 transition hover:bg-slate-100 dark:text-text-secondary dark:hover:bg-raised";

type Props = { request: PendingRequest; busy: boolean; error: unknown; onSend: (values: Record<string, string>) => void; onDecline: () => void };

export function SecretPanel({ request, busy, error, onSend, onDecline }: Props) {
  const s = request.secret!;
  const fields = s.fields?.length ? s.fields : [{ name: "value", label: s.label, placeholder: s.placeholder, secret: true }];
  const [values, setValues] = useState<Record<string, string>>({});
  const [shown, setShown] = useState<Record<string, boolean>>({});
  const send = () => {
    if (busy) return;
    const out = Object.fromEntries(fields.map((f) => [f.name, values[f.name] ?? ""]));
    setValues({});
    onSend(out);
  };
  return (
    <div className="max-h-[45vh] overflow-y-auto border-b border-slate-200 px-4 pb-3 pt-3 dark:border-border">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-muted">
        <LockKey className="size-4" /><span>El agente pide un dato privado</span>
      </div>
      <p className="text-[0.9375rem] font-medium text-slate-900 dark:text-white">{s.label}</p>
      <p className="mb-2 text-sm text-slate-600 dark:text-text-secondary">{s.reason}</p>
      {s.url && (
        <a href={s.url} target="_blank" rel="noreferrer noopener" className={`${ghost} mb-2 gap-1.5 border border-slate-200 dark:border-border`}>
          <ArrowSquareOut className="size-4" />Abrir
        </a>
      )}
      <div className="space-y-2">
        {fields.map((f, i) => {
          const hidden = f.secret !== false && !shown[f.name];
          return (
            <label key={f.name} className="block">
              {fields.length > 1 && <span className="mb-0.5 block text-xs text-slate-500 dark:text-muted">{f.label}</span>}
              <span className="flex items-center gap-1 rounded-lg border border-slate-200 bg-white px-2 dark:border-border dark:bg-surface">
                <input type={hidden ? "password" : "text"} autoComplete="off" spellCheck={false} data-1p-ignore autoFocus={i === 0}
                  aria-label={f.label} placeholder={f.placeholder} disabled={busy} value={values[f.name] ?? ""}
                  onChange={(e) => setValues((v) => ({ ...v, [f.name]: e.target.value }))}
                  onKeyDown={(e) => {
                    if (e.key !== "Enter") return;
                    e.preventDefault();
                    e.stopPropagation();
                    if (i === fields.length - 1) send();
                    else (e.currentTarget.closest("div.space-y-2")?.querySelectorAll("input")[i + 1] as HTMLInputElement | undefined)?.focus();
                  }}
                  className="h-9 min-w-0 flex-1 bg-transparent text-sm outline-none" />
                {f.secret !== false && (
                  <button type="button" className="grid size-7 place-items-center rounded-full text-slate-500 hover:bg-slate-100 dark:text-muted dark:hover:bg-raised"
                    aria-label={hidden ? "Mostrar" : "Ocultar"} title={hidden ? "Mostrar" : "Ocultar"}
                    onClick={() => setShown((v) => ({ ...v, [f.name]: !v[f.name] }))}>
                    {hidden ? <Eye className="size-4" /> : <EyeSlash className="size-4" />}
                  </button>
                )}
              </span>
            </label>
          );
        })}
      </div>
      {!!error && <ErrorText error={error} />}
      <div className="mt-3 flex justify-end gap-2">
        <button type="button" className={ghost} disabled={busy} onClick={() => { setValues({}); onDecline(); }}>Rechazar</button>
        <button type="button" className={primary} disabled={busy} onClick={send}>Enviar</button>
      </div>
    </div>
  );
}
