// Company logos for model pickers, from @lobehub/icons-static-svg (MIT).
// Monochrome logos use currentColor, so they follow the theme.
const files = import.meta.glob("../assets/model-logos/*.svg", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const logo = (name: string) => files[`../assets/model-logos/${name}.svg`];

// Ordered: the first pattern that matches the model, then the driver, wins.
const rules: [RegExp, string][] = [
  [/claude|anthropic|opus|sonnet|haiku|fable/, "claude"],
  [/gpt|codex|openai|o[1-9]\b|o[1-9]-/, "openai"],
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

// ModelLogo shows the logo of the company behind a model, or its initial when unknown.
export function ModelLogo({ driver, model = "" }: { driver: string; model?: string }) {
  const svg = logo(match(model) ?? match(driver) ?? "");
  if (!svg) return <span className="grid size-5 shrink-0 place-items-center rounded bg-slate-500 text-[10px] font-bold text-white">{driver.slice(0, 1).toUpperCase() || "?"}</span>;
  return <span aria-hidden="true" className="grid size-5 shrink-0 place-items-center text-slate-900 dark:text-white [&>svg]:size-4" dangerouslySetInnerHTML={{ __html: svg }} />;
}
