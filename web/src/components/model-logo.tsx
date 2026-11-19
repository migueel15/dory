// Provider logos for model pickers, from @lobehub/icons-static-svg (MIT).
// Monochrome logos use currentColor, so they follow the theme.
const files = import.meta.glob("../assets/model-logos/*.svg", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const logo = (name: string) => files[`../assets/model-logos/${name}.svg`];

// Ordered: the first pattern that matches the provider (driver), then the model, wins.
const rules: [RegExp, string][] = [
  [/antigravity/, "antigravity"],
  [/opencode/, "opencode"],
  [/codex/, "openai"],
  [/claude|anthropic|opus|sonnet|haiku|fable/, "claude"],
  [/gpt|openai|o[1-9]\b|o[1-9]-/, "openai"],
  [/gemini|gemma|google/, "gemini"],
  [/deepseek/, "deepseek"],
  [/mistral|codestral|devstral|magistral/, "mistral"],
  [/grok|xai/, "grok"],
  [/qwen|alibaba/, "qwen"],
  [/llama|meta/, "meta"],
  [/kimi|moonshot/, "kimi"],
  [/glm|zhipu|z\.ai/, "zhipu"],
  [/minimax/, "minimax"],
  [/copilot/, "githubcopilot"],
  [/cursor/, "cursor"],
  [/ollama/, "ollama"],
];

const match = (s: string) => rules.find(([re]) => re.test(s.toLowerCase()))?.[1];

// ModelLogo shows the logo of the provider running a model (Claude, OpenAI for Codex, OpenCode…);
// the model only decides when the provider is unknown. Falls back to the initial.
export function ModelLogo({ driver, model = "", compact = false }: { driver: string; model?: string; compact?: boolean }) {
  const svg = logo(match(driver) ?? match(model) ?? "");
  const box = `grid shrink-0 place-items-center ${compact ? "size-3.5" : "size-5"}`;
  if (!svg) return <span className={`${box} rounded bg-slate-500 text-[0.625rem] font-bold text-white ${compact ? "leading-none" : ""}`}>{driver.slice(0, 1).toUpperCase() || "?"}</span>;
  return <span aria-hidden="true" className={`${box} text-slate-900 dark:text-white ${compact ? "[&>svg]:size-3.5" : "[&>svg]:size-4"}`} dangerouslySetInnerHTML={{ __html: svg }} />;
}
