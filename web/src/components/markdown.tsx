import { isValidElement, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Check, Copy } from "@phosphor-icons/react";
import Markdown, { defaultUrlTransform, type Options } from "react-markdown";
import remarkGfm from "remark-gfm";
import { highlight, languageByName, loadLanguage } from "../highlight";
import { RefCode, RefImg, RefLink } from "./refs";
import { copyText, prose } from "./ui";

export { defaultUrlTransform };

// Md renders GitHub-flavored markdown. With onToggle, task checkboxes can be
// clicked and report the source line of their item.
// keepLocal lets file:// image paths through (react-markdown drops unknown schemes).
const keepLocal = (url: string) => (url.startsWith("file://") ? url : defaultUrlTransform(url));

// CodeBlock renders fenced code with syntax colors and a copy button.
function CodeBlock({ children }: { children?: ReactNode }) {
  const el = isValidElement<{ className?: string; children?: ReactNode }>(children) ? children : undefined;
  const raw = el?.props.children;
  const code = (typeof raw === "string" ? raw : Array.isArray(raw) ? raw.join("") : "").replace(/\n$/, "");
  const lang = /language-([^\s]+)/.exec(el?.props.className ?? "")?.[1];
  const resolved = lang ? languageByName(lang) : undefined;
  const [ready, setReady] = useState<string>();
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (resolved) loadLanguage(resolved).then((ok) => { if (ok) setReady(resolved); });
  }, [resolved]);
  const html = useMemo(() => (resolved && ready === resolved ? highlight(code, resolved) : undefined), [code, resolved, ready]);
  const copy = async () => {
    if (await copyText(code)) { setCopied(true); setTimeout(() => setCopied(false), 1500); }
  };
  return (
    <div className="group relative">
      <pre>{html !== undefined ? <code className="hljs" dangerouslySetInnerHTML={{ __html: html }} /> : <code>{code}</code>}</pre>
      <button type="button" onClick={copy} aria-label={copied ? "Copiado" : "Copiar"} title={copied ? "Copiado" : "Copiar"} className="absolute right-2 top-2 grid size-7 place-items-center rounded-md border border-slate-200 bg-white/80 text-slate-500 opacity-100 hover:text-slate-900 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100 dark:border-border dark:bg-surface/80 dark:text-muted dark:hover:text-white">
        {copied ? <Check weight="regular" aria-hidden="true" className="size-4" /> : <Copy weight="regular" aria-hidden="true" className="size-4" />}
      </button>
    </div>
  );
}

// Formulas load the first time a message has one: remark-math, rehype-katex
// and KaTeX's styles stay out of the main bundle.
type MathPlugins = { remark: NonNullable<Options["remarkPlugins"]>[number]; rehype: NonNullable<Options["rehypePlugins"]>[number] };
let math: MathPlugins | undefined;
let mathLoading: Promise<MathPlugins> | undefined;
const loadMath = () => (mathLoading ??= Promise.all([import("remark-math"), import("rehype-katex"), import("katex/dist/katex.min.css")])
  .then(([rm, rk]) => (math = { remark: rm.default, rehype: rk.default })));
const hasMath = (t: string) => /\$[^$\n]+\$|\$\$|\\\(|\\\[/.test(t);
// The harnesses write formulas as \( \) and \[ \]; remark-math reads $ and $$.
// A $$…$$ alone on its line is a centered formula, as Claude shows it.
const dollars = (t: string) => t.replace(/\\\[([\s\S]+?)\\\]/g, (_, x) => `$$${x}$$`).replace(/\\\(([\s\S]+?)\\\)/g, (_, x) => `$${x}$`)
  .replace(/^[ \t]*\$\$([^\n]+?)\$\$[ \t]*$/gm, (_, x) => `$$\n${x}\n$$`);

// Md renders GitHub-flavored markdown with formulas. serif gives an answer the
// reading look of Claude; with onToggle, task checkboxes can be clicked and
// report the source line of their item.
export function Md({ children, components, urlTransform, onToggle, serif = false, rehypePlugins = [] }: { children: string; onToggle?: (line: number) => void; serif?: boolean } & Pick<Options, "components" | "urlTransform" | "rehypePlugins">) {
  const wantsMath = hasMath(children);
  const [, setLoaded] = useState(!!math);
  useEffect(() => { if (wantsMath && !math) loadMath().then(() => setLoaded(true), () => {}); }, [wantsMath]);
  const tasks: Options["components"] = onToggle && {
    li: ({ node, children: content, ...rest }) => {
      const line = node?.position?.start.line;
      return <li {...rest} onClick={(e) => { if (line && (e.target as HTMLElement).matches("input[type=checkbox]") && e.target === (e.currentTarget as HTMLElement).querySelector(":scope > input[type=checkbox]")) onToggle(line); }}>{content}</li>;
    },
    input: ({ node: _, disabled: __, ...rest }) => <input {...rest} readOnly className="cursor-pointer" />,
  };
  const withMath = wantsMath && math;
  return (
    <div className={serif ? `${prose} ${reading}` : prose}>
      <Markdown remarkPlugins={withMath ? [remarkGfm, math!.remark] : [remarkGfm]} rehypePlugins={withMath ? [math!.rehype, ...(rehypePlugins ?? [])] : rehypePlugins ?? []}
        components={{ a: RefLink, code: RefCode, pre: CodeBlock, img: RefImg, ...tasks, ...components }} urlTransform={urlTransform ?? keepLocal}>{withMath ? dollars(children) : children}</Markdown>
    </div>
  );
}

// reading is the look of an answer, as in Claude: a serif at a comfortable
// size and line height, with sans-serif code.
const reading = "font-reading !text-[1rem] !leading-[1.7] text-slate-800 dark:text-[#e8e6e3] [&_h1]:!border-0 [&_h2]:!border-0 [&_h1]:!text-[1.5em] [&_h2]:!text-[1.25em] [&_h3]:!text-[1.1em] [&_h1,&_h2,&_h3,&_h4]:!font-semibold [&_li]:!my-1 [&_strong]:font-semibold [&_pre]:!text-[0.8125rem] [&_table]:text-[0.9375rem] [&_.katex-display]:my-3 [&_.katex-display]:overflow-x-auto [&_.katex-display]:overflow-y-hidden";
