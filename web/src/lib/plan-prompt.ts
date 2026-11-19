// The first message of a plan-mode thread starts with a <dory-plan> block:
// the instructions for the agent and the id of the task it defines. The agent
// reads the whole text; the thread view shows a plan card with only the
// user's own words.

const OPEN = /^<dory-plan task="([^"]+)">\n[\s\S]*?\n<\/dory-plan>\n*/;

// planPrompt builds that first message for task taskId (requirement 13.3).
export const planPrompt = (taskId: string, text: string) =>
  `<dory-plan task="${taskId}">\n` +
  `Modo plan. Vamos a definir la tarea de Dory ${taskId} antes de implementarla. No cambies código.\n` +
  `Haz las preguntas que necesites y propón el plan con la herramienta propose_plan del MCP de Dory (objetivo, alcance, dónde, diseño, requisitos, validación, riesgos). ` +
  `Solo yo acepto el plan.\n` +
  `</dory-plan>\n\n${text}`;

// parsePlanPrompt finds that block; body is what follows it.
export function parsePlanPrompt(text: string): { taskId: string; body: string } | undefined {
  const m = OPEN.exec(text);
  return m ? { taskId: m[1], body: text.slice(m[0].length) } : undefined;
}
