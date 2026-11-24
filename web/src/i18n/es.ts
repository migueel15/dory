// The texts of the system messages the backend sends as a code with params.
// A message without code (an older one, or an agent's own text) shows its text.
export type MessageParams = Record<string, unknown>;

type Text = string | ((p: MessageParams) => string);

export const messages: Record<string, Text> = {
  "turn.interrupted": "Turno interrumpido",
  "turn.no_response": "el agente terminó sin respuesta",
  "turn.failed": "El turno falló",
  "node.restarted": "El nodo se reinició mientras el agente trabajaba; el turno se interrumpió.",
  "node.restarted_waiting": "El nodo se reinició mientras el agente esperaba una respuesta; el turno se interrumpió.",
  "context.cleared": "Contexto limpiado",
  "model.change_failed": (p) => `No se pudo cambiar el modelo: ${p.error ?? ""}`,
  "question.title": "Pregunta",
};

// t is the text of a code, or undefined when the web has none.
export function t(code: string | undefined, params: MessageParams = {}): string | undefined {
  const text = code ? messages[code] : undefined;
  return text === undefined ? undefined : typeof text === "function" ? text(params) : text;
}

// messageText is what a message shows: its code translated, else its own text.
export function messageText(m: { code?: string; params?: MessageParams; text: string }): string {
  return t(m.code, m.params) ?? m.text;
}
