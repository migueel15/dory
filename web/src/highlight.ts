import hljs from "highlight.js/lib/core";

// Languages load on demand, one chunk each; unknown extensions render as plain text.
const loaders: Record<string, () => Promise<{ default: Parameters<typeof hljs.registerLanguage>[1] }>> = {
  bash: () => import("highlight.js/lib/languages/bash"),
  c: () => import("highlight.js/lib/languages/c"),
  cpp: () => import("highlight.js/lib/languages/cpp"),
  csharp: () => import("highlight.js/lib/languages/csharp"),
  css: () => import("highlight.js/lib/languages/css"),
  dockerfile: () => import("highlight.js/lib/languages/dockerfile"),
  go: () => import("highlight.js/lib/languages/go"),
  ini: () => import("highlight.js/lib/languages/ini"),
  java: () => import("highlight.js/lib/languages/java"),
  javascript: () => import("highlight.js/lib/languages/javascript"),
  json: () => import("highlight.js/lib/languages/json"),
  kotlin: () => import("highlight.js/lib/languages/kotlin"),
  makefile: () => import("highlight.js/lib/languages/makefile"),
  markdown: () => import("highlight.js/lib/languages/markdown"),
  php: () => import("highlight.js/lib/languages/php"),
  protobuf: () => import("highlight.js/lib/languages/protobuf"),
  python: () => import("highlight.js/lib/languages/python"),
  ruby: () => import("highlight.js/lib/languages/ruby"),
  rust: () => import("highlight.js/lib/languages/rust"),
  sql: () => import("highlight.js/lib/languages/sql"),
  swift: () => import("highlight.js/lib/languages/swift"),
  typescript: () => import("highlight.js/lib/languages/typescript"),
  xml: () => import("highlight.js/lib/languages/xml"),
  yaml: () => import("highlight.js/lib/languages/yaml"),
};

const ext: Record<string, string> = {
  sh: "bash", bash: "bash", zsh: "bash", c: "c", h: "c", cc: "cpp", cpp: "cpp", hpp: "cpp", cs: "csharp",
  css: "css", go: "go", ini: "ini", toml: "ini", java: "java", js: "javascript", jsx: "javascript",
  mjs: "javascript", cjs: "javascript", json: "json", kt: "kotlin", md: "markdown", mdx: "markdown",
  php: "php", proto: "protobuf", py: "python", rb: "ruby", rs: "rust", sql: "sql", swift: "swift",
  ts: "typescript", tsx: "typescript", html: "xml", xml: "xml", svg: "xml", yml: "yaml", yaml: "yaml",
};

// languageOf deduces the highlight.js language from a file path.
export function languageOf(path: string): string | undefined {
  const name = path.split("/").pop()!.toLowerCase();
  if (name === "dockerfile") return "dockerfile";
  if (name === "makefile") return "makefile";
  return ext[name.split(".").pop()!];
}

const alias: Record<string, string> = { shell: "bash", console: "bash", js: "javascript", ts: "typescript", py: "python", yml: "yaml", html: "xml", rs: "rust", rb: "ruby", golang: "go", "c++": "cpp", "c#": "csharp", docker: "dockerfile", toml: "ini" };

// languageByName maps a markdown fence name (ts, sh, python...) to a loadable language.
export function languageByName(name: string): string | undefined {
  const n = name.toLowerCase();
  const l = alias[n] ?? n;
  return l in loaders ? l : ext[n];
}

const loading = new Map<string, Promise<boolean>>();

// loadLanguage registers the language once and resolves true when it can be used.
export function loadLanguage(lang: string): Promise<boolean> {
  let p = loading.get(lang);
  if (!p) {
    p = loaders[lang]().then((m) => { hljs.registerLanguage(lang, m.default); return true; }, () => false);
    loading.set(lang, p);
  }
  return p;
}

// highlight returns escaped HTML with token spans, or undefined when it cannot be highlighted.
export function highlight(code: string, lang: string): string | undefined {
  if (!hljs.getLanguage(lang)) return undefined;
  try {
    return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
  } catch {
    return undefined;
  }
}
