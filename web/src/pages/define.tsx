import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Link, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Item, type ItemPlan, type ItemRefinement, type PlanSections, type RefineInput } from "../api/client";
import { useThreadLive } from "../api/live";
import { ItemTitle, pendingPoll } from "../components/item-title";
import { PendingRequest } from "../components/pending-request";
import { ThreadLink } from "../components/thread-link";
import { ThreadStatus } from "../components/thread-status";
import { Badge, Button, Card, ErrorText, Label, Md, Notice, StatusPill, Textarea } from "../components/ui";
import { LaunchDialog } from "./launch";
import { useLaunchForm } from "./launch-form";
import { STATUS_INFO } from "./items";

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
  const [implement, setImplement] = useState(false);
  const composer = useRef<HTMLTextAreaElement>(null);
  const item = useQuery({ queryKey: ["items", "one", id], queryFn: () => api.item(id), refetchInterval: pendingPoll });
  const refinement = useQuery({ queryKey: ["refinement", id], queryFn: () => api.refinement(id), refetchInterval: (q) => (working(q.state.data) ? 15_000 : 60_000) });
  const plans = useQuery({ queryKey: ["refinement", id, "plans"], queryFn: () => api.plans(id) });
  const refresh = () => ["refinement", "items", "t3-status"].forEach((k) => qc.invalidateQueries({ queryKey: [k] }));
  useThreadLive(refinement.data?.thread?.thread_id, ["refinement", id]);
  const accept = useMutation({ mutationFn: () => api.acceptPlan(id), onSuccess: refresh });
  if (!item.data) return item.error ? <ErrorText error={item.error} /> : <Notice loading />;
  const it = item.data;
  const plan = refinement.data?.plan ?? null;
  const previous = plans.data?.find((p) => plan && p.version === plan.version - 1);
  const canAccept = !!plan && definable(it.status) && plan.open_questions.length === 0 && !(it.status === "ready" && plan.accepted_at);
  const comment = (title: string, text: string) => {
    const quote = text.split("\n").filter(Boolean).slice(0, 3).map((l) => `> ${l}`).join("\n");
    setDraft(`Sobre «${title}»:\n${quote}\n\n`);
    setTab("chat");
    setTimeout(() => composer.current?.focus(), 0);
  };
  const info = STATUS_INFO[it.status];
  return (
    <div className="flex flex-col gap-3 lg:h-[calc(100dvh-6rem)]">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 space-y-1">
          <Link to={it.project_id ? "/projects/$id" : "/inbox"} params={{ id: it.project_id ?? "" }} className={`${muted} hover:underline`}>← Volver</Link>
          <h1 className="text-lg font-semibold break-words"><ItemTitle item={it} /></h1>
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${info.tone}`}>{info.label}</span>
            {refinement.data?.thread && <ThreadState refinement={refinement.data} />}
          </div>
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
      <div className="grid min-h-0 flex-1 gap-3 lg:grid-cols-2">
        <section className={`${tab === "chat" ? "flex" : "hidden"} min-h-0 flex-col lg:flex`} aria-label="Chat">
          <Chat item={it} data={refinement.data} draft={draft} setDraft={setDraft} composer={composer} onChange={refresh} />
        </section>
        <section className={`${tab === "plan" ? "block" : "hidden"} min-h-0 overflow-y-auto lg:block`} aria-label="Plan">
          <PlanPanel item={it} plan={plan} previous={previous} onComment={comment} onSaved={refresh} />
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

function Chat({ item, data, draft, setDraft, composer, onChange }: { item: Item; data?: ItemRefinement; draft: string; setDraft: (s: string) => void; composer: React.RefObject<HTMLTextAreaElement | null>; onChange: () => void }) {
  const [restart, setRestart] = useState(false);
  const qc = useQueryClient();
  const key = ["refinement", item.id];
  // A message to an open thread shows at once; a failed send restores the chat and the draft.
  const send = useMutation({
    mutationFn: (body: RefineInput) => api.refine(item.id, body),
    onMutate: async (body) => {
      const prev = qc.getQueryData<ItemRefinement>(key);
      if (!prev?.thread || body.restart || !body.text) return { prev: undefined };
      await qc.cancelQueries({ queryKey: key });
      qc.setQueryData<ItemRefinement>(key, { ...prev, messages: [...prev.messages, { role: "user", text: body.text } as ItemRefinement["messages"][number]] });
      setDraft("");
      return { prev };
    },
    onError: (_e, body, c) => { if (c?.prev) { qc.setQueryData(key, c.prev); setDraft(body.text ?? ""); } },
    onSuccess: () => { setDraft(""); setRestart(false); },
    onSettled: onChange,
  });
  const end = useRef<HTMLDivElement>(null);
  const count = data?.messages.length ?? 0;
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [count, data?.pending?.id]);
  if (!data) return <Notice loading />;
  const open = definable(item.status);
  if (open && (!data.thread || restart)) return <StartForm item={item} draft={draft} setDraft={setDraft} restart={restart} send={send} onCancel={data.thread ? () => setRestart(false) : undefined} />;
  const submit = () => { if (draft.trim() && !send.isPending) send.mutate({ text: draft.trim() }); };
  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); submit(); } };
  return (
    <Card className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="min-h-64 flex-1 space-y-2 overflow-y-auto pr-1">
        {data.messages.map((m, i) => <Bubble key={i} role={m.role} text={m.text} first={i === 0} />)}
        {working(data) && <p className={`${muted} motion-safe:animate-pulse`}>El agente está trabajando…</p>}
        {data.pending && data.thread && (
          <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/30">
            <PendingRequest key={data.pending.id} threadId={data.thread.thread_id} pending={data.pending} onDone={onChange} />
          </div>
        )}
        <div ref={end} />
      </div>
      {data.thread_error && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded bg-slate-100 p-2 text-sm dark:bg-raised">
          <span>{data.thread_error}</span>
          {open && <Button variant="ghost" onClick={() => setRestart(true)}>Empezar un hilo nuevo</Button>}
        </div>
      )}
      {open ? (
        <div className="space-y-1">
          <Textarea ref={composer} value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={onKey} placeholder="Escribe al agente: correcciones, ideas o respuestas (Ctrl+Enter para enviar)" className="min-h-20" />
          <ErrorText error={send.error} />
          <div className="flex justify-end"><Button disabled={!draft.trim() || send.isPending} onClick={submit}>Enviar</Button></div>
        </div>
      ) : <p className={muted}>El item ya no está en definición; la conversación queda como referencia.</p>}
    </Card>
  );
}

// Bubble shows one message; the first one is Dory's setup prompt, kept folded.
function Bubble({ role, text, first }: { role: string; text: string; first: boolean }) {
  if (first && role === "user") {
    return (
      <details className="rounded-lg bg-slate-50 p-2 text-xs dark:bg-raised/60">
        <summary className="cursor-pointer text-slate-500 dark:text-muted">Instrucciones enviadas al agente</summary>
        <p className="mt-2 whitespace-pre-wrap break-words">{text}</p>
      </details>
    );
  }
  const mine = role === "user";
  return (
    <div className={`flex ${mine ? "justify-end" : "justify-start"}`}>
      <div className={`max-w-[85%] rounded-2xl px-3 py-2 text-sm break-words ${mine ? "rounded-br-sm bg-blue-600 text-white whitespace-pre-wrap dark:bg-accent" : "rounded-bl-sm bg-slate-100 dark:bg-raised"}`}>
        {mine ? text : <Md>{text}</Md>}
      </div>
    </div>
  );
}

function StartForm({ item, draft, setDraft, restart, send, onCancel }: { item: Item; draft: string; setDraft: (s: string) => void; restart: boolean; send: { mutate: (b: RefineInput) => void; isPending: boolean; error: unknown }; onCancel?: () => void }) {
  const form = useLaunchForm({ doryProject: item.project_id });
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

function PlanPanel({ item, plan, previous, onComment, onSaved }: { item: Item; plan: ItemPlan | null; previous?: ItemPlan; onComment: (title: string, text: string) => void; onSaved: () => void }) {
  const [editing, setEditing] = useState(false);
  if (editing) return <PlanEditor item={item} plan={plan} onDone={() => { setEditing(false); onSaved(); }} onCancel={() => setEditing(false)} />;
  const open = definable(item.status);
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
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className={muted}>Versión {plan.version} · {plan.author === "user" ? "escrita por ti" : "del agente"} · {date(plan.created_at)}</p>
        <div className="flex items-center gap-2">
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
          <Card key={s.key} className={changed ? "ring-2 ring-amber-300 dark:ring-amber-700" : ""}>
            <div className="mb-1 flex items-start justify-between gap-2">
              <div>
                <h3 className="text-sm font-semibold">{s.title} {changed && <StatusPill tone="amber">Cambiado</StatusPill>}</h3>
                <p className={muted}>{s.hint}</p>
              </div>
              {open && <Button variant="bare" className={`${muted} hover:underline`} onClick={() => onComment(s.title, plan.sections[s.key])}>Comentar</Button>}
            </div>
            <Md>{plan.sections[s.key]}</Md>
          </Card>
        );
      })}
      {missing.length > 0 && <p className={muted}>Sin definir: {missing.map((s) => s.title).join(", ")}.</p>}
    </div>
  );
}

function PlanEditor({ item, plan, onDone, onCancel }: { item: Item; plan: ItemPlan | null; onDone: () => void; onCancel: () => void }) {
  const [sections, setSections] = useState<PlanSections>(plan?.sections ?? EMPTY);
  const [questions, setQuestions] = useState((plan?.open_questions ?? []).join("\n"));
  const save = useMutation({
    mutationFn: () => api.createPlan(item.id, { sections, open_questions: questions.split("\n").map((q) => q.trim()).filter(Boolean) }),
    onSuccess: onDone,
  });
  return (
    <Card className="space-y-2">
      <p className={muted}>Guardar crea una versión nueva escrita por ti. Si el agente sigue trabajando, avísale en el chat.</p>
      {SECTIONS.map((s) => (
        <Label key={s.key} text={s.title}>
          <Textarea value={sections[s.key]} placeholder={s.hint} onChange={(e) => setSections({ ...sections, [s.key]: e.target.value })} />
        </Label>
      ))}
      <Label text="Preguntas abiertas (una por línea)"><Textarea value={questions} onChange={(e) => setQuestions(e.target.value)} className="min-h-16" /></Label>
      <ErrorText error={save.error} />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel}>Cancelar</Button>
        <Button disabled={save.isPending || SECTIONS.every((s) => !sections[s.key].trim())} onClick={() => save.mutate()}>Guardar versión</Button>
      </div>
    </Card>
  );
}
