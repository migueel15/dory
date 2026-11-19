// Messages Dory writes for the user (to define or implement a task, review a
// pull request…) wrap their instructions in a <dory-context> block:
//
//   <dory-context kind="plan" task="…">
//   instructions and context for the agent
//   </dory-context>
//
//   what the user wrote
//
// The agent reads the whole text. The thread view shows a card with the kind,
// a link to what it is about and only the user's words; the context folds.
// The server writes the same format (internal/api/autotext.go).

export type AutoKind = "plan" | "implement" | "issue-tasks" | "pr-conflicts" | "pr-review" | "pr-question";
export type AutoMessage = { kind: string; attrs: Record<string, string>; context: string; body: string };

const unescape = (s: string) => s.replace(/&#34;|&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&amp;/g, "&");
const escape = (s: string) => s.replace(/&/g, "&amp;").replace(/"/g, "&#34;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

const BLOCK = /^<dory-context((?:\s+[\w-]+="[^"]*")*)>\n([\s\S]*?)\n<\/dory-context>\n*/;
// Plan messages written before <dory-context> existed.
const LEGACY = /^<dory-plan task="([^"]+)">\n([\s\S]*?)\n<\/dory-plan>\n*/;

export function autoMessage(kind: AutoKind, attrs: Record<string, string | number | undefined>, context: string, user = ""): string {
  const list = Object.entries(attrs).filter(([, v]) => v !== undefined && v !== "").sort(([a], [b]) => a.localeCompare(b));
  const head = `<dory-context kind="${kind}"${list.map(([k, v]) => ` ${k}="${escape(String(v))}"`).join("")}>`;
  const body = user.trim();
  return `${head}\n${context.trim()}\n</dory-context>${body ? `\n\n${body}` : ""}`;
}

export function parseAutoMessage(text: string): AutoMessage | undefined {
  const m = BLOCK.exec(text);
  if (m) {
    const attrs: Record<string, string> = {};
    for (const [, k, v] of m[1].matchAll(/([\w-]+)="([^"]*)"/g)) attrs[k] = unescape(v);
    const { kind = "", ...rest } = attrs;
    return { kind, attrs: rest, context: m[2], body: text.slice(m[0].length) };
  }
  const old = LEGACY.exec(text);
  return old ? { kind: "plan", attrs: { task: old[1] }, context: old[2], body: text.slice(old[0].length) } : undefined;
}

// planMessage starts a thread that defines a task (requirement 13.3). The
// task's own text, when it is not what the user just wrote, goes as context.
export const planMessage = (task: { id: string; title?: string; description?: string }, user = "") => autoMessage("plan", { task: task.id, title: task.title },
  `Modo plan. Vamos a definir la tarea de Dory ${task.id} antes de implementarla. No cambies código.\n` +
  `Haz las preguntas que necesites y escribe el plan como un documento Markdown con las herramientas del MCP de Dory: plan_read para leerlo, plan_write para escribirlo entero la primera vez y plan_edit para cambiar partes concretas. ` +
  `Yo lo veo y lo edito en el panel lateral, así que léelo de nuevo antes de cambiarlo. Usa secciones como objetivo, alcance, dónde, diseño, requisitos, validación y riesgos, y una de preguntas abiertas si las hay. ` +
  `Solo yo acepto el plan.` + (task.description !== undefined ? `\n\n# ${task.title}\n\n${task.description}` : ""), user);
