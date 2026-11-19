import { createHighlighterCore, type HighlighterCore } from "shiki/core";
import { createJavaScriptRegexEngine } from "shiki/engine/javascript";

// Syntax highlighting with Shiki: the TextMate grammars and the default
// themes of VS Code (Light+ and Dark+), so code looks as it does in the
// editor. Each token carries both colours: the light one as its colour and
// the dark one in --sd, which index.css applies on dark (and .code-dark) boxes.
// Languages load on demand, one chunk each; unknown extensions render as plain text.
type Grammar = { default: unknown };
const loaders: Record<string, () => Promise<Grammar>> = {
  bash: () => import("shiki/langs/bash.mjs"),
  c: () => import("shiki/langs/c.mjs"),
  cpp: () => import("shiki/langs/cpp.mjs"),
  csharp: () => import("shiki/langs/csharp.mjs"),
  css: () => import("shiki/langs/css.mjs"),
  dockerfile: () => import("shiki/langs/dockerfile.mjs"),
  go: () => import("shiki/langs/go.mjs"),
  html: () => import("shiki/langs/html.mjs"),
  ini: () => import("shiki/langs/ini.mjs"),
  java: () => import("shiki/langs/java.mjs"),
  javascript: () => import("shiki/langs/javascript.mjs"),
  json: () => import("shiki/langs/json.mjs"),
  jsx: () => import("shiki/langs/jsx.mjs"),
  kotlin: () => import("shiki/langs/kotlin.mjs"),
  make: () => import("shiki/langs/make.mjs"),
  markdown: () => import("shiki/langs/markdown.mjs"),
  php: () => import("shiki/langs/php.mjs"),
  proto: () => import("shiki/langs/proto.mjs"),
  python: () => import("shiki/langs/python.mjs"),
  ruby: () => import("shiki/langs/ruby.mjs"),
  rust: () => import("shiki/langs/rust.mjs"),
  sql: () => import("shiki/langs/sql.mjs"),
  swift: () => import("shiki/langs/swift.mjs"),
  toml: () => import("shiki/langs/toml.mjs"),
  tsx: () => import("shiki/langs/tsx.mjs"),
  typescript: () => import("shiki/langs/typescript.mjs"),
  xml: () => import("shiki/langs/xml.mjs"),
  yaml: () => import("shiki/langs/yaml.mjs"),
};

const ext: Record<string, string> = {
  sh: "bash", bash: "bash", zsh: "bash", c: "c", h: "c", cc: "cpp", cpp: "cpp", hpp: "cpp", cs: "csharp",
  css: "css", go: "go", ini: "ini", toml: "toml", java: "java", js: "javascript", jsx: "jsx",
  mjs: "javascript", cjs: "javascript", json: "json", kt: "kotlin", md: "markdown", mdx: "markdown",
  php: "php", proto: "proto", py: "python", rb: "ruby", rs: "rust", sql: "sql", swift: "swift",
  ts: "typescript", tsx: "tsx", mts: "typescript", html: "html", xml: "xml", svg: "xml", yml: "yaml", yaml: "yaml",
};

// languageOf deduces the language from a file path.
export function languageOf(path: string): string | undefined {
  const name = path.split("/").pop()!.toLowerCase();
  if (name === "dockerfile") return "dockerfile";
  if (name === "makefile") return "make";
  return ext[name.split(".").pop()!];
}

const alias: Record<string, string> = { shell: "bash", sh: "bash", console: "bash", zsh: "bash", js: "javascript", ts: "typescript", py: "python", yml: "yaml", rs: "rust", rb: "ruby", golang: "go", "c++": "cpp", "c#": "csharp", docker: "dockerfile", makefile: "make", protobuf: "proto" };

// languageByName maps a markdown fence name (ts, sh, python...) to a loadable language.
export function languageByName(name: string): string | undefined {
  const n = name.toLowerCase();
  const l = alias[n] ?? n;
  return l in loaders ? l : ext[n];
}

let highlighter: Promise<HighlighterCore> | undefined;
let ready: HighlighterCore | undefined;
const getHighlighter = () => (highlighter ??= createHighlighterCore({
  themes: [import("shiki/themes/light-plus.mjs"), import("shiki/themes/dark-plus.mjs")],
  langs: [],
  engine: createJavaScriptRegexEngine(),
}).then((h) => (ready = h)));

const loading = new Map<string, Promise<boolean>>();

// loadLanguage prepares the language once and resolves true when it can be used.
export function loadLanguage(lang: string): Promise<boolean> {
  let p = loading.get(lang);
  if (!p) {
    const load = loaders[lang];
    p = load ? Promise.all([getHighlighter(), load()]).then(async ([h, m]) => { await h.loadLanguage(m.default as never); return true; }, () => false) : Promise.resolve(false);
    loading.set(lang, p);
  }
  return p;
}

const escape = (s: string) => s.replace(/[&<>"]/g, (c) => (c === "&" ? "&amp;" : c === "<" ? "&lt;" : c === ">" ? "&gt;" : "&quot;"));

// highlight returns escaped HTML with a coloured span per token, or undefined
// when the language is not loaded yet (loadLanguage first).
export function highlight(code: string, lang: string): string | undefined {
  if (!ready || !ready.getLoadedLanguages().includes(lang)) return undefined;
  try {
    const lines = ready.codeToTokensWithThemes(code, { lang, themes: { light: "light-plus", dark: "dark-plus" } });
    return lines.map((line) => line.map((t) => {
      const light = t.variants.light, dark = t.variants.dark;
      const italic = (light.fontStyle ?? 0) & 1 ? "font-style:italic;" : "";
      return `<span style="color:${light.color};--sd:${dark.color};${italic}">${escape(t.content)}</span>`;
    }).join("")).join("\n");
  } catch {
    return undefined;
  }
}
