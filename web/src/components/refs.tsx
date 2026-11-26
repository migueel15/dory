import { createContext, useContext, type AnchorHTMLAttributes, type HTMLAttributes, type ImgHTMLAttributes } from "react";
import { Link } from "@tanstack/react-router";
import { storage, storageKeys } from "../lib/storage";

// GitHub pull requests and issues open inside Dory, which shows them.
const GH = /^https?:\/\/github\.com\/([\w.-]+)\/([\w.-]+)\/(pull|issues)\/(\d+)(?:[/?#].*)?$/;
export const githubRoute = (href = "") => {
  const m = GH.exec(href);
  return m ? { owner: m[1], repo: m[2], kind: m[3] === "pull" ? "pulls" : "issues", number: m[4] } : undefined;
};

// A file reference: path, path:12, path:12-20, path#L12 or path#L12-L20, or a
// GitHub blob link to a file. The path needs a folder or an extension.
export type FileRef = { path: string; line?: number; end?: number };
const BLOB = /^https?:\/\/github\.com\/[\w.-]+\/[\w.-]+\/blob\/[^/]+\/(.+)$/;
const REF = /^(?:\.\/)?([\w@.-]+(?:\/[\w@.-]+)*\.[A-Za-z]\w*|[\w@.-]+(?:\/[\w@.-]+)+)(?:(?::|#L)(\d+)(?:-L?(\d+))?)?$/;
export const parseFileRef = (raw: string): FileRef | undefined => {
  const text = raw.trim().replace(BLOB, "$1");
  if (/^[a-z]+:\/\//i.test(text)) return undefined;
  const m = REF.exec(text);
  if (!m) return undefined;
  const line = m[2] ? Number(m[2]) : undefined;
  return { path: m[1], line, end: m[3] ? Number(m[3]) : line };
};

// FileRefContext, when a view provides it, makes file references clickable:
// the pull request page opens its diff on that line.
export const FileRefContext = createContext<((ref: FileRef) => void) | null>(null);

// ThreadBrowserContext opens external links in the thread's integrated browser
// when the user preference is set to "integrated" (default).
export const ThreadBrowserContext = createContext<((url: string) => void) | null>(null);

const refCls = "cursor-pointer text-blue-700 underline decoration-current/30 underline-offset-2 hover:decoration-current dark:text-accent-text";

// RefLink renders Markdown links: GitHub PRs and issues go to Dory's page,
// file references to the host view, the rest open in the integrated or system
// browser according to the user's preference.
export function RefLink({ href, children, node: _n, onClick, ...rest }: AnchorHTMLAttributes<HTMLAnchorElement> & { node?: unknown }) {
  const openRef = useContext(FileRefContext);
  const openInBrowser = useContext(ThreadBrowserContext);
  const gh = githubRoute(href);
  if (gh) return <Link to="/github/$owner/$repo/$kind/$number" params={gh} className={rest.className}>{children}</Link>;
  const file = openRef && href ? parseFileRef(href) : undefined;
  if (file && openRef) return <button type="button" className={refCls} onClick={() => openRef(file)} title={`Abrir ${file.path}${file.line ? `:${file.line}` : ""} en el diff`}>{children}</button>;
  const isExternal = !!href && /^https?:\/\//i.test(href);
  const handleClick: AnchorHTMLAttributes<HTMLAnchorElement>["onClick"] = (e) => {
    onClick?.(e);
    if (e.defaultPrevented || !isExternal || !openInBrowser) return;
    if (e.shiftKey || e.altKey || e.button !== 0) return;
    const pref = storage.get(storageKeys.linkOpenTarget) || "integrated";
    const invert = e.ctrlKey || e.metaKey;
    const useIntegrated = invert ? pref !== "integrated" : pref === "integrated";
    if (useIntegrated) {
      e.preventDefault();
      openInBrowser(href);
    }
  };
  return <a href={href} target={isExternal ? "_blank" : rest.target} rel={isExternal ? "noreferrer" : rest.rel} onClick={handleClick} {...rest}>{children}</a>;
}

// RefCode makes inline code that names a file (`a/b.go:12`) clickable when the
// view handles file references.
export function RefCode({ children, className, node: _n, ...rest }: HTMLAttributes<HTMLElement> & { node?: unknown }) {
  const openRef = useContext(FileRefContext);
  const text = typeof children === "string" ? children : "";
  const file = openRef && !className && text && !text.includes("\n") ? parseFileRef(text) : undefined;
  if (file && openRef) return <button type="button" onClick={() => openRef(file)} title="Abrir en el diff" className={`${refCls} font-mono text-[0.9em]`}><code {...rest}>{children}</code></button>;
  return <code className={className} {...rest}>{children}</code>;
}

// RefImg renders Markdown images as a link to the full-size image.
export function RefImg({ src, alt, node: _n, ...rest }: ImgHTMLAttributes<HTMLImageElement> & { node?: unknown }) {
  const url = typeof src === "string" ? src : "";
  return (
    <a href={url} target="_blank" rel="noreferrer" className="inline-block max-w-full">
      <img src={url} alt={alt ?? ""} loading="lazy" {...rest} className="max-h-[32rem] max-w-full rounded-lg border border-slate-200 dark:border-border" />
    </a>
  );
}
