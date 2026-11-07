import type { AnchorHTMLAttributes, ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowSquareOut } from "@phosphor-icons/react";
import t3Logo from "../assets/t3.png";

type Props = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href" | "target"> & {
  /** Web URL of the thread in T3 Code; also the fallback when the app is missing. */
  url: string;
  environmentId?: string;
  threadId?: string;
  children: ReactNode;
};

const isAndroid = typeof navigator !== "undefined" && /Android/i.test(navigator.userAgent);

/** Android intent that opens the T3 Code app on the thread, falling back to the web URL. */
export function threadHref(url: string, environmentId?: string, threadId?: string): string {
  if (!isAndroid || !environmentId || !threadId) return url;
  const path = `threads/${encodeURIComponent(environmentId)}/${encodeURIComponent(threadId)}`;
  return `intent://${path}#Intent;scheme=t3code;S.browser_fallback_url=${encodeURIComponent(url)};end`;
}

/** Opens T3 Code on the thread (native app on Android, the web elsewhere), with the T3 logo. */
export function OpenInT3({ url, environmentId, threadId, className = "" }: { url: string; environmentId?: string; threadId?: string; className?: string }) {
  return (
    <a href={threadHref(url, environmentId, threadId)} target="_blank" rel="noreferrer" title="Abrir en T3 Code"
      className={`inline-flex shrink-0 items-center gap-1.5 rounded-md border border-slate-200 bg-white px-2 py-1 text-xs font-medium text-slate-600 shadow-sm hover:bg-slate-50 hover:text-slate-900 dark:border-border dark:bg-surface dark:text-text-secondary dark:hover:bg-raised dark:hover:text-white ${className}`}>
      <img src={t3Logo} alt="" className="size-4 rounded" />Abrir en T3<ArrowSquareOut aria-hidden="true" className="size-3" />
    </a>
  );
}

/** Link to a thread: opens its view in Dory, where a button opens T3 Code. */
export function ThreadLink({ url, environmentId: _env, threadId, children, ...rest }: Props) {
  if (!threadId) return <a {...rest} href={url} target="_blank" rel="noreferrer">{children}</a>;
  return <Link {...(rest as object)} to="/threads/$threadId" params={{ threadId }}>{children}</Link>;
}
