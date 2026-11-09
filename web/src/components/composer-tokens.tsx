import { GitPullRequest, Lightning, LinkSimple, Quotes, Terminal, type Icon } from "@phosphor-icons/react";

// Composer tokens: a /command, $skill, @file or #pull request picked from the
// menu, or a quote of a message or plan text (value is the quoted text). In the text they are plain characters (what T3 receives); the UI always
// shows them as one object, and they delete as a whole.
export type TokenKind = "command" | "skill" | "path" | "ref" | "quote";
export type Token = { kind: TokenKind; value: string; title?: string };
export type Tokens = Record<string, Token>; // keyed by the exact text in the composer

const looks: Record<TokenKind, { icon: Icon; color: string }> = {
  command: { icon: Terminal, color: "text-amber-600 dark:text-amber-400" },
  skill: { icon: Lightning, color: "text-violet-600 dark:text-violet-400" },
  path: { icon: LinkSimple, color: "text-sky-600 dark:text-sky-400" },
  ref: { icon: GitPullRequest, color: "text-emerald-600 dark:text-emerald-400" },
  quote: { icon: Quotes, color: "text-indigo-600 dark:text-indigo-400" },
};

// A quote goes to the agent as a Markdown block quote headed by its label, so
// sent messages can show it again as one object.
export const quoteLabel = (text: string, max = 32) => {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > max ? `${flat.slice(0, max).trimEnd()}…` : flat;
};
export const quoteBlock = (label: string, body: string) =>
  `\n\n> «${label}»\n${body.split("\n").filter((l) => l.trim()).map((l) => `> ${l}`).join("\n")}\n\n`;
export const QUOTE_RE = /(?:^|\n)> «([^»\n]+)»\n((?:>[^\n]*(?:\n|$))*)/g;

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// tokenRanges finds where the known tokens sit in text: each must stand alone
// (start of text or after a space, and followed by a space or the end).
export function tokenRanges(text: string, tokens: Tokens): { start: number; end: number; text: string }[] {
  const keys = Object.keys(tokens).sort((a, b) => b.length - a.length);
  if (!keys.length) return [];
  const re = new RegExp(`(^|\\s)(${keys.map(escape).join("|")})(?=\\s|$)`, "g");
  const out: { start: number; end: number; text: string }[] = [];
  for (const m of text.matchAll(re)) {
    const start = (m.index ?? 0) + m[1].length;
    out.push({ start, end: start + m[2].length, text: m[2] });
  }
  return out;
}

// paintTokens renders composer text with each token as a chip. The chip keeps
// the exact characters at the same weight (the icon covers the sigil), so the
// painted layer lines up glyph for glyph with the textarea above it.
export function paintTokens(text: string, tokens: Tokens) {
  const out: React.ReactNode[] = [];
  let last = 0;
  for (const r of tokenRanges(text, tokens)) {
    const t = tokens[r.text];
    const { icon: Glyph, color } = looks[t.kind];
    out.push(text.slice(last, r.start));
    out.push(
      <span key={r.start} className={`underline decoration-current/30 underline-offset-2 ${color}`}>
        <span className="relative"><span className="text-transparent">{r.text[0]}</span><Glyph weight="bold" className="absolute right-[0.25em] top-1/2 size-[0.8em] -translate-y-1/2" /></span>{r.text.slice(1)}
      </span>,
    );
    last = r.end;
  }
  out.push(text.slice(last));
  return out;
}

// TokenChip shows a token in a sent message: icon and label, with its full
// reference on hover.
export function TokenChip({ kind, label, title }: { kind: TokenKind; label: string; title?: string }) {
  const { icon: Glyph, color } = looks[kind];
  return (
    <span title={title} className={`inline-flex items-baseline gap-1.5 font-medium ${color}`}>
      <Glyph weight="bold" aria-hidden="true" className="size-3.5 shrink-0 translate-y-0.5" />
      <span className="underline decoration-current/30 underline-offset-2">{label}</span>
    </span>
  );
}
