import type { AnchorHTMLAttributes, ReactNode } from "react";

type Props = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href" | "target"> & {
  /** Web URL of the thread; also the fallback when the app is missing. */
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

/** Link to a T3 Code thread: the native app on Android, the web elsewhere. */
export function ThreadLink({ url, environmentId, threadId, children, ...rest }: Props) {
  return <a {...rest} href={threadHref(url, environmentId, threadId)} target="_blank" rel="noreferrer">{children}</a>;
}
