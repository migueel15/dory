import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowLeft, ChatCircleText, PencilSimpleLine, Question } from "@phosphor-icons/react";
import { Link, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, itemInput, type Item, type ItemPlan, type ItemRefinement, type PlanSections, type RefineInput } from "../api/client";
import { useThreadLive } from "../api/live";
import { ItemTitle, pendingPoll } from "../components/item-title";
import { ThreadLink } from "../components/thread-link";
import { ThreadStatus } from "../components/thread-status";
import { Badge, Button, Card, ErrorText, Label, Md, Notice, Select, StatusPill, Textarea } from "../components/ui";
import { LaunchDialog } from "./launch";
import { useLaunchForm } from "./launch-form";
import { STATUS_INFO } from "./items";
import { ResizeHandle, ThreadChat, type Insert } from "./thread";
import { quoteLabel } from "../components/composer-tokens";
import { SelectionMenu } from "../components/selection-menu";

const SECTIONS: { key: keyof PlanSections; title: string; hint: string }[] = [
  { key: "goal", title: "Objetivo", hint: "Qué se quiere conseguir y por qué." },
  { key: "scope", title: "Alcance", hint: "Qué entra y qué queda fuera." },
  { key: "where", title: "Dónde", hint: "Archivos, módulos y servicios afectados." },
  { key: "design", title: "Diseño", hint: "Arquitectura y enfoque técnico." },
  { key: "requirements", title: "Requisitos", hint: "Dependencias y condiciones previas." },
  { key: "validation", title: "Cómo probar", hint: "Cómo se comprueba que funciona." },
  { key: "risks", title: "Riesgos", hint: "Lo que puede salir mal." },
];
const EMPTY: PlanSections = { goal: "", scope: "", where: "", design: "", requirements: "", validation: "", risks: "" };
const definable = (status: string) => ["inbox", "triaged", "ready"].includes(status);
const working = (r?: ItemRefinement) => ["running", "working"].includes(r?.thread?.status ?? "");
const muted = "text-xs text-slate-500 dark:text-muted";
const date = (iso: string) => new Date(iso).toLocaleString("es-ES", { dateStyle: "short", timeStyle: "short" });

// Define is where an idea becomes a ready ticket: a chat with the definition
// agent on one side and the plan it writes, live, on the other.
export function Define() {
  const { id } = useParams({ strict: false }) as { id: string };
  const qc = useQueryClient();
  const [tab, setTab] = useState<"chat" | "plan">("chat");
  const [draft, setDraft] = useState("");
  // The plan column is resizable, like the thread's changes panel. Each visit
  // starts at 60 % chat / 40 % plan; a drag fixes it in pixels until leaving.
  const side = useRef<HTMLElement>(null);
  const [width, setWidth] = useState<number | null>(null);
  const [insert, setInsert] = useState<Insert>();
  const [implement, setImplement] = useState(false);
  const item = useQuery({ queryKey: ["items", "one", id], queryFn: () => api.item(id), refetchInterval: pendingPoll });
  const refinement = useQuery({ queryKey: ["refinement", id], queryFn: () => api.refinement(id), refetchInterval: (q) => (working(q.state.data) ? 15_000 : 60_000) });
  const plans = useQuery({ queryKey: ["refinement", id, "plans"], queryFn: () => api.plans(id) });
  const refresh = () => ["refinement", "items", "t3-status"].forEach((k) => qc.invalidateQueries({ queryKey: [k] }));
  useThreadLive(refinement.data?.thread?.thread_id, ["refinement", id]);
  const accept = useMutation({ mutationFn: () => api.acceptPlan(id), onSuccess: refresh });
  if (!item.data) return item.error ? <ErrorText error={item.error} /> : <Notice loading />;
  const it = item.data;
  const plan = refinement.data?.plan ?? null;
  const canAccept = !!plan && definable(it.status) && plan.open_questions.length === 0 && !(it.status === "ready" && plan.accepted_at);
  // comment quotes a section (its first lines) or, with a lead, the selected text whole.
  const comment = (title: string, text: string, lead?: string) => {
    const lines = text.split("\n").filter(Boolean);
    const quote = (lead ? lines : lines.slice(0, 3)).map((l) => `> ${l}`).join("\n");
    const ref = `Sobre «${title}»:\n${quote}\n\n${lead ?? ""}`;
    // With a thread the reference goes to its composer; before, to the start form.
    if (refinement.data?.thread) setInsert((p) => ({ text: lead ?? "", quote: { label: lead ? `${title}: ${quoteLabel(text, 24)}` : title, body: text }, seq: (p?.seq ?? 0) + 1 }));
    else setDraft(ref);
    setTab("chat");
  };
  const info = STATUS_INFO[it.status];
  const accepted = it.status === "ready" && plan?.accepted_at ? plan.version : null;
  if (!definable(it.status)) {
    return (
      <div className="space-y-3">
        <Link to={it.project_id ? "/projects/$id" : "/inbox"} params={{ id: it.project_id ?? "" }} className={`${muted} hover:underline`}>← Volver</Link>
        <h1 className="text-lg font-semibold break-words"><ItemTitle item={it} /></h1>
        <Notice>Este item ya está en «{info.label}»; la definición está cerrada. <Link to={it.project_id ? "/projects/$id" : "/inbox"} params={{ id: it.project_id ?? "" }} className="underline">Ver el item en su lista</Link></Notice>
      </div>
    );
  }
  return (
    <div className="flex h-[calc(100dvh-4rem-1px)] min-h-0 flex-col gap-3 sm:h-[calc(100dvh-3rem-2px)]">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <Link to={it.project_id ? "/projects/$id" : "/inbox"} params={{ id: it.project_id ?? "" }} title="Volver" aria-label="Volver"
            className="grid size-8 shrink-0 place-items-center rounded-md text-slate-500 hover:bg-slate-200 hover:text-slate-900 dark:text-muted dark:hover:bg-raised dark:hover:text-white"><ArrowLeft className="size-4" /></Link>
          <h1 className="min-w-0 text-lg font-semibold break-words"><ItemTitle item={it} /></h1>
          <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${info.tone}`}>{info.label}</span>
          {refinement.data?.thread && <ThreadState refinement={refinement.data} />}
        </div>
        <div className="flex flex-wrap gap-2">
          {definable(it.status) && <Button disabled={!canAccept || accept.isPending} title={plan?.open_questions.length ? "Quedan preguntas abiertas" : undefined} onClick={() => accept.mutate()}>Aceptar plan</Button>}
          {it.status === "ready" && <Button variant={canAccept ? "ghost" : "default"} onClick={() => setImplement(true)}>Enviar a implementar</Button>}
        </div>
      </header>
      <ErrorText error={accept.error ?? refinement.error} />
      <div className="flex gap-1 lg:hidden" role="tablist">
        {(["chat", "plan"] as const).map((t) => (
          <Button key={t} role="tab" aria-selected={tab === t} variant={tab === t ? "default" : "ghost"} className="flex-1" onClick={() => setTab(t)}>{t === "chat" ? "Chat" : `Plan${plan ? ` · v${plan.version}` : ""}`}</Button>
        ))}
      </div>
      <div className="flex min-h-0 flex-1">
        <section className={`${tab === "chat" ? "flex" : "hidden"} min-h-0 min-w-0 flex-1 flex-col lg:flex`} aria-label="Chat">
          <Chat item={it} data={refinement.data} draft={draft} setDraft={setDraft} accepted={accepted} insert={insert} onChange={refresh} />
        </section>
        <section ref={side} style={{ "--panel-w": width ? `${width}px` : "40%" } as React.CSSProperties}
          className={`${tab === "plan" ? "flex" : "hidden"} min-h-0 w-full lg:flex lg:w-[var(--panel-w)] lg:shrink-0`} aria-label="Plan">
          <ResizeHandle panel={side} width={width ?? side.current?.offsetWidth ?? 0} onResize={setWidth} storageKey={null} min={360} rest={360} />
          <div className="min-h-0 min-w-0 flex-1 overflow-y-auto pb-4 pr-2 [scrollbar-gutter:stable] lg:pl-2">
            <PlanPanel item={it} latest={plan} versions={plans.data ?? []} onComment={comment} onSaved={refresh} />
          </div>
        </section>
      </div>
      {implement && <LaunchDialog items={[it]} role="implementation" onClose={() => setImplement(false)} />}
    </div>
  );
}

function ThreadState({ refinement }: { refinement: ItemRefinement }) {
  const t = refinement.thread!;
  const label = <span className="inline-flex items-center gap-1"><ThreadStatus status={t.status || "unknown"} attention={t.attention} />{t.model && <Badge>{t.model}{t.effort ? ` · ${t.effort}` : ""}</Badge>}</span>;
  return t.url ? <ThreadLink url={t.url} threadId={t.thread_id} title="Abrir el hilo en T3 Code">{label}</ThreadLink> : label;
}

// Chat starts the definition thread or, once it runs, shows the same chat as
// the thread page. With the plan accepted, a message reopens it first.
function Chat({ item, data, draft, setDraft, accepted, insert, onChange }: { item: Item; data?: ItemRefinement; draft: string; setDraft: (s: string) => void; accepted: number | null; insert?: { text: string; seq: number }; onChange: () => void }) {
  const [restart, setRestart] = useState(false);
  const qc = useQueryClient();
  const send = useMutation({
    mutationFn: (body: RefineInput) => api.refine(item.id, body),
    onSuccess: () => { setDraft(""); setRestart(false); },
    onSettled: onChange,
  });
  // Back to triaged so the plan must be accepted again; body_md is untouched.
  const reopen = async () => {
    await api.items.update(item.id, itemInput(item, { status: "triaged" }));
    ["items", "item", "refinement"].forEach((k) => qc.invalidateQueries({ queryKey: [k] }));
  };
  if (!data) return <Notice loading />;
  if (!data.thread || restart) return <StartForm item={item} draft={draft} setDraft={setDraft} restart={restart} send={send} onCancel={data.thread ? () => setRestart(false) : undefined} />;
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      {data.thread_error && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded bg-slate-100 p-2 text-sm dark:bg-raised">
          <span>{data.thread_error}</span>
          <Button variant="ghost" onClick={() => setRestart(true)}>Empezar un hilo nuevo</Button>
        </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col">
        <ThreadChat threadId={data.thread.thread_id} insert={insert} beforeSend={accepted !== null ? reopen : undefined}
          notice={accepted !== null && <p className="mx-3 -mb-4 rounded-t-3xl border border-b-0 border-green-300 bg-green-50 px-4 pb-6 pt-2 text-xs font-medium text-green-800 dark:border-green-800 dark:bg-green-950/30 dark:text-green-300">Plan v{accepted} aceptado · si escribes, se reabrirá y habrá que volver a aceptarlo</p>} />
      </div>
    </div>
  );
}

function StartForm({ item, draft, setDraft, restart, send, onCancel }: { item: Item; draft: string; setDraft: (s: string) => void; restart: boolean; send: { mutate: (b: RefineInput) => void; isPending: boolean; error: unknown }; onCancel?: () => void }) {
  const form = useLaunchForm({ section: "refine", doryProject: item.project_id });
  return (
    <Card className="space-y-3">
      <div>
        <h2 className="font-semibold">{restart ? "Empezar un hilo nuevo" : "Definir con un agente"}</h2>
        <p className={muted}>El agente analiza la idea y el código, te pregunta lo que no esté claro y escribe el plan. No toca el código ni marca el item como listo: eso lo decides tú.</p>
      </div>
      {item.raw_text && <blockquote className="border-l-4 border-slate-300 pl-3 text-sm whitespace-pre-wrap break-words dark:border-border">{item.raw_text}</blockquote>}
      <Label text="Indicaciones para el agente (opcional)"><Textarea value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="Contexto, restricciones o por dónde empezar" /></Label>
      {form.fields}
      <ErrorText error={form.error ?? send.error} />
      <div className="flex justify-end gap-2">
        {onCancel && <Button variant="ghost" onClick={onCancel}>Cancelar</Button>}
        <Button disabled={!form.ready || send.isPending} onClick={() => { form.remember(); send.mutate({ ...form.value, text: draft.trim(), restart }); }}>Empezar</Button>
      </div>
    </Card>
  );
}

// PlanPanel shows the latest plan or, picked from the version select, an older
// one read-only; each is compared with the version right before it.
function PlanPanel({ item, latest, versions, onComment, onSaved }: { item: Item; latest: ItemPlan | null; versions: ItemPlan[]; onComment?: (title: string, text: string, lead?: string) => void; onSaved: () => void }) {
  const [editing, setEditing] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const [picked, setPicked] = useState<number | null>(null);
  if (editing) return <PlanEditor item={item} plan={latest} onDone={() => { setEditing(false); setPicked(null); onSaved(); }} onCancel={() => setEditing(false)} />;
  const old = picked !== null && picked !== latest?.version ? versions.find((p) => p.version === picked) : undefined;
  const plan = old ?? latest;
  const previous = plan ? versions.find((p) => p.version === plan.version - 1) : undefined;
  const open = definable(item.status) && !old;
  const options = [...new Map([...(latest ? [latest] : []), ...versions].map((p) => [p.version, p])).values()]
    .sort((a, b) => b.version - a.version)
    .map((p) => ({ value: String(p.version), label: `Versión ${p.version}${p.version === latest?.version ? " (actual)" : ""} · ${p.author === "user" ? "tuya" : "del agente"} · ${date(p.created_at)}` }));
  if (!plan) {
    return (
      <Card className="space-y-2">
        <Notice>Aún no hay plan. El agente lo publicará aquí y lo irá actualizando con cada respuesta.</Notice>
        {open && <Button variant="ghost" onClick={() => setEditing(true)}>Escribirlo yo</Button>}
      </Card>
    );
  }
  const filled = SECTIONS.filter((s) => plan.sections[s.key].trim());
  const missing = SECTIONS.filter((s) => !plan.sections[s.key].trim());
  return (
    <div ref={box} className="space-y-3">
      {open && onComment && <SelectionMenu box={box} scope="[data-section]" label="Enviar la selección al chat" actions={[
        { label: "Pedir cambio", icon: <PencilSimpleLine className="size-3.5" />, run: (t, el) => onComment(el.dataset.section ?? "", t, "Cambia esto: ") },
        { label: "Preguntar", icon: <Question className="size-3.5" />, run: (t, el) => onComment(el.dataset.section ?? "", t, "Pregunta: ") },
      ]} />}
      <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-2 border-b border-slate-200 bg-slate-50 py-2 dark:border-border dark:bg-canvas">
        {options.length > 1
          ? <Select value={String(plan.version)} onChange={(v) => setPicked(Number(v))} options={options} className="max-w-full text-xs" />
          : <p className={muted}>Versión {plan.version} · {plan.author === "user" ? "escrita por ti" : "del agente"} · {date(plan.created_at)}</p>}
        <div className="flex items-center gap-2">
          {old && <Button variant="ghost" onClick={() => setPicked(null)}>Ver la actual</Button>}
          {plan.accepted_at && <StatusPill tone="green">Aceptado</StatusPill>}
          {open && <Button variant="ghost" onClick={() => setEditing(true)}>Editar</Button>}
        </div>
      </div>
      {plan.open_questions.length > 0 && (
        <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-800 dark:bg-amber-950/30">
          <p className="font-medium text-amber-900 dark:text-amber-300">Preguntas abiertas</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5">{plan.open_questions.map((q) => <li key={q}>{q}</li>)}</ul>
        </div>
      )}
      {filled.map((s) => {
        const changed = !!previous && previous.sections[s.key] !== plan.sections[s.key];
        return (
          <div key={s.key} data-section={s.title}><Card className={changed ? "!border-amber-400 dark:!border-amber-600" : ""}>
            <div className="mb-1 flex items-start justify-between gap-2">
              <div>
                <h3 className="flex items-center gap-1.5 text-sm font-semibold">{s.title}{changed && <PencilSimpleLine weight="bold" className="size-3.5 text-amber-600 dark:text-amber-400" aria-label={`Cambiado desde la versión ${previous!.version}`}><title>{`Cambiado desde la versión ${previous!.version}`}</title></PencilSimpleLine>}</h3>
                <p className={muted}>{s.hint}</p>
              </div>
              {open && onComment && <Button variant="ghost" className="!p-1.5" title="Citar en el chat" aria-label={`Citar «${s.title}» en el chat`} onClick={() => onComment(s.title, plan.sections[s.key])}><ChatCircleText className="size-4" /></Button>}
            </div>
            <Md>{plan.sections[s.key]}</Md>
          </Card></div>
        );
      })}
      {missing.length > 0 && <p className={muted}>Sin definir: {missing.map((s) => s.title).join(", ")}.</p>}
    </div>
  );
}

// PlanEditor keeps the plan's look: the same separate cards, each with its
// Markdown text editable in place. Saving creates a version written by the user.
function PlanEditor({ item, plan, onDone, onCancel }: { item: Item; plan: ItemPlan | null; onDone: () => void; onCancel: () => void }) {
  const [sections, setSections] = useState<PlanSections>(plan?.sections ?? EMPTY);
  const [questions, setQuestions] = useState((plan?.open_questions ?? []).join("\n"));
  const save = useMutation({
    mutationFn: () => api.createPlan(item.id, { sections, open_questions: questions.split("\n").map((q) => q.trim()).filter(Boolean) }),
    onSuccess: onDone,
  });
  return (
    <div className="space-y-3">
      <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-2 border-b border-slate-200 bg-slate-50 py-2 dark:border-border dark:bg-canvas">
        <p className={muted}>Editando: guardar crea una versión nueva escrita por ti. Si el agente sigue trabajando, avísale en el chat.</p>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onCancel}>Cancelar</Button>
          <Button disabled={save.isPending || SECTIONS.every((s) => !sections[s.key].trim())} onClick={() => save.mutate()}>Guardar versión</Button>
        </div>
      </div>
      <ErrorText error={save.error} />
      <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-800 dark:bg-amber-950/30">
        <p className="font-medium text-amber-900 dark:text-amber-300">Preguntas abiertas</p>
        <InlineText value={questions} onChange={setQuestions} placeholder="Una por línea; vacío si no quedan" />
      </div>
      {SECTIONS.map((s) => (
        <Card key={s.key}>
          <div className="mb-1">
            <h3 className="text-sm font-semibold">{s.title}</h3>
            <p className={muted}>{s.hint}</p>
          </div>
          <InlineText value={sections[s.key]} onChange={(v) => setSections({ ...sections, [s.key]: v })} placeholder={s.hint} mono />
        </Card>
      ))}
    </div>
  );
}

// InlineText is a borderless textarea that grows with its content, so editing
// keeps the size and look of the text it replaces.
function InlineText({ value, onChange, placeholder, mono }: { value: string; onChange: (v: string) => void; placeholder?: string; mono?: boolean }) {
  const ref = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, [value]);
  return (
    <textarea ref={ref} value={value} rows={1} placeholder={placeholder} onChange={(e) => onChange(e.target.value)}
      className={`mt-1 block w-full resize-none overflow-hidden rounded bg-transparent p-0 text-sm leading-6 outline-none placeholder:text-slate-400 focus:ring-0 ${mono ? "font-mono text-[13px]" : ""}`} />
  );
}
