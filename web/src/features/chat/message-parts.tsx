import { defaultUrlTransform, Md } from "../../components/markdown";
import { TokenChip, tokenize, type TokenKind } from "../composer/composer-tokens";
import { splitMessageQuotes } from "./message-quotes";
import { CodeRefChip } from "./code-ref";

// Objects of a sent message (quotes, files, skills, commands) show as chips.
export const keepToken = (url: string) => (url.startsWith("tok:") || url.startsWith("file://") ? url : defaultUrlTransform(url));
export const tokenLinks = {
  a: ({ href, children, node: _n, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { node?: unknown }) => {
    if (!href?.startsWith("tok:")) return <a href={href} target="_blank" rel="noreferrer" {...rest}>{children}</a>;
    const [, kind, raw] = href.split(":");
    const value = decodeURIComponent(raw ?? "");
    if (kind === "quote") {
      const q = (() => { try { return JSON.parse(value) as { label: string; body: string }; } catch { return { label: value, body: value }; } })();
      return <TokenChip kind="quote" label={q.label} title={q.body} />;
    }
    const label = kind === "path" ? value.replace(/\/+$/, "").split("/").pop() || value : value;
    return <TokenChip kind={kind as TokenKind} label={label} title={value} />;
  },
};

// MessageParts renders a user message, with its quotes as a bar on the left.
export function MessageParts({ text }: { text: string }) {
  return <>{splitMessageQuotes(text).map((part, index) => part.kind === "code" ? <CodeRefChip key={index} data={part.ref} /> : part.kind === "quote" ? (
    <blockquote key={index} className="min-w-0 whitespace-pre-wrap break-words border-l-2 border-slate-300 pl-2.5 text-sm leading-relaxed text-slate-600 dark:border-border-strong dark:text-muted">
      {part.text}
    </blockquote>
  ) : (
    // Same size and line height as the answers.
    <div key={index} className="[&>div]:!text-[1rem] [&>div]:!leading-[1.7]"><Md urlTransform={keepToken} components={tokenLinks}>{tokenize(part.text)}</Md></div>
  ))}</>;
}

