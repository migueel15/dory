import type { AnchorHTMLAttributes, ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowSquareOut } from "@phosphor-icons/react";

type Props = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href" | "target"> & {
  /** Web URL of the thread in T3 Code; also the fallback when the app is missing. */
  url: string;
  environmentId?: string;
  threadId?: string;
  /** Show the small "open in T3" icon next to the link (default true). */
  external?: boolean;
  /** Classes of the wrapper that holds the link and the icon. */
  wrapClassName?: string;
  children: ReactNode;
};

const isAndroid = typeof navigator !== "undefined" && /Android/i.test(navigator.userAgent);

/** Android intent that opens the T3 Code app on the thread, falling back to the web URL. */
export function threadHref(url: string, environmentId?: string, threadId?: string): string {
  if (!isAndroid || !environmentId || !threadId) return url;
  const path = `threads/${encodeURIComponent(environmentId)}/${encodeURIComponent(threadId)}`;
  return `intent://${path}#Intent;scheme=t3code;S.browser_fallback_url=${encodeURIComponent(url)};end`;
}

const stop = (e: { stopPropagation: () => void }) => e.stopPropagation();

/** Opens T3 Code on the thread: the native app on Android, the web elsewhere. */
export function OpenInT3({ url, environmentId, threadId, className = "" }: { url: string; environmentId?: string; threadId?: string; className?: string }) {
  return (
    <a href={threadHref(url, environmentId, threadId)} target="_blank" rel="noreferrer" title="Abrir en T3 Code" aria-label="Abrir en T3 Code"
      onClick={stop} onPointerDown={stop}
      className={`grid size-6 shrink-0 place-items-center rounded text-slate-400 hover:bg-slate-100 hover:text-slate-900 dark:hover:bg-raised dark:hover:text-white ${className}`}>
      <ArrowSquareOut aria-hidden="true" className="size-3.5" />
    </a>
  );
}

/** Link to a thread: opens its view in Dory, with a secondary icon that opens T3 Code. */
export function ThreadLink({ url, environmentId, threadId, external = true, wrapClassName, children, ...rest }: Props) {
  if (!threadId) return <a {...rest} href={url} target="_blank" rel="noreferrer">{children}</a>;
  const link = <Link {...(rest as object)} to="/threads/$threadId" params={{ threadId }}>{children}</Link>;
  if (!external) return link;
  return (
    <span className={wrapClassName ?? "inline-flex min-w-0 max-w-full items-center gap-0.5 align-middle"}>
      {link}
      <OpenInT3 url={url} environmentId={environmentId} threadId={threadId} />
    </span>
  );
}
