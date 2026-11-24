import { useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { Button } from "../../components/ui";
import { markSeen, markUnseen } from "../../lib/terminal/activity";
import { TerminalClient, terminalApi, type TerminalStatus } from "../../lib/terminal/client";
import { terminalFont, terminalTheme } from "../../lib/terminal/theme";
import { events } from "../../lib/store";

// A creation in flight per tab, so remounting never opens a second shell.
const creating = new Map<string, Promise<{ id: string }>>();

// TerminalPanel shows a real shell of the thread's node on an xterm. The
// shell lives on the node, not here: leaving the thread or closing the panel
// only drops the connection, and coming back replays the recent output.
export function TerminalPanel({ threadId, tab, id, active, onId }: { threadId: string; tab: string; id?: string; active: boolean; onId: (id: string | undefined) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const activeRef = useRef(active);
  activeRef.current = active;
  const [status, setStatus] = useState<TerminalStatus>("connecting");
  const [background, setBackground] = useState(() => terminalTheme().background);

  useEffect(() => {
    const el = host.current!;
    const term = new Terminal({
      cursorBlink: true, scrollback: 5000, fontSize: 13, fontFamily: terminalFont(), theme: terminalTheme(),
      allowProposedApi: false, macOptionIsMeta: true, rightClickSelectsWord: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);
    // The panel's own hover focus (see SidePanel) lands on the keyboard input.
    term.textarea?.setAttribute("data-panel-focus", "");
    const refit = () => { if (el.clientWidth > 0 && el.clientHeight > 0) { try { fit.fit(); } catch { /* not laid out yet */ } } };
    refit();

    // With the terminal focused every key belongs to the shell: nothing reaches Dory's shortcuts.
    const keep = (e: KeyboardEvent) => e.stopPropagation();
    el.addEventListener("keydown", keep);
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown" || !e.ctrlKey || !e.shiftKey) return true;
      if (e.code === "KeyC") {
        e.preventDefault();
        const text = term.getSelection();
        if (text) void navigator.clipboard.writeText(text);
        return false;
      }
      if (e.code === "KeyV") {
        e.preventDefault();
        void navigator.clipboard.readText().then((t) => term.paste(t)).catch(() => {});
        return false;
      }
      return true;
    });

    const offFonts = events.fontsChanged.on(() => { term.options.fontFamily = terminalFont(); refit(); });
    let raf = 0;
    const observer = new ResizeObserver(() => { cancelAnimationFrame(raf); raf = requestAnimationFrame(refit); });
    observer.observe(el);
    const offTheme = events.themeChanged.on(() => { term.options.theme = terminalTheme(); setBackground(terminalTheme().background); });

    let client: TerminalClient | null = null;
    let disposed = false;
    if (id) {
      client = new TerminalClient(threadId, id, {
        data: (chunk) => { term.write(chunk); if (!activeRef.current) markUnseen(threadId, tab); },
        reset: () => term.reset(),
        status: setStatus,
      });
      client.resize(term.cols, term.rows);
      term.onData((d) => client!.send(d));
      term.onBinary((d) => client!.send(d, true));
      term.onResize(({ cols, rows }) => client!.resize(cols, rows));
      client.start();
    } else {
      setStatus("connecting");
      const key = `${threadId}|${tab}`;
      const pending = creating.get(key) ?? terminalApi.open(threadId, { cols: term.cols, rows: term.rows });
      creating.set(key, pending);
      pending.then((info) => { creating.delete(key); onId(info.id); }, () => { creating.delete(key); if (!disposed) setStatus("gone"); });
    }
    return () => {
      disposed = true;
      client?.stop();
      cancelAnimationFrame(raf);
      observer.disconnect();
      offTheme();
      offFonts();
      el.removeEventListener("keydown", keep);
      term.dispose();
    };
  }, [threadId, tab, id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { if (active) markSeen(threadId, tab); }, [active, threadId, tab]);

  const reopen = () => {
    if (id) void terminalApi.close(threadId, id).catch(() => {});
    onId(undefined);
  };
  return (
    <div className="relative h-full" style={{ background }}>
      <div ref={host} className="absolute inset-0 px-2 pt-2" />
      {(status === "exited" || status === "gone") && (
        <div className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-3 border-t border-slate-200 bg-white/90 px-3 py-2 text-sm backdrop-blur dark:border-border dark:bg-surface/90">
          <span className="text-slate-600 dark:text-text-secondary">{status === "gone" ? "Terminal cerrada" : "La shell ha terminado"}</span>
          <Button variant="ghost" onClick={reopen}>Abrir otra</Button>
        </div>
      )}
    </div>
  );
}
