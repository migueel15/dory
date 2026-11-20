import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy } from "@phosphor-icons/react";
import { api, type GithubDeviceCode } from "../api/client";
import { Button, ErrorText } from "./ui";

// GithubLogin links this node to a GitHub account with the device flow: it
// shows the code, opens github.com/login/device and waits for the token,
// which stays on the node.
export function GithubLogin() {
  const qc = useQueryClient();
  const [code, setCode] = useState<GithubDeviceCode | null>(null);
  const [copied, setCopied] = useState(false);
  const start = useMutation({
    mutationFn: api.githubDeviceStart,
    onSuccess: (c) => { setCode(c); window.open(c.verification_uri, "_blank", "noopener"); },
  });
  const state = useQuery({ queryKey: ["github-device"], queryFn: api.githubDeviceState, enabled: !!code, refetchInterval: (q) => (q.state.data?.state === "pending" || !q.state.data ? 2_000 : false), gcTime: 0 });
  const done = state.data?.state === "done";
  useEffect(() => {
    if (!done) return;
    setCode(null);
    qc.removeQueries({ queryKey: ["github-device"] });
    qc.invalidateQueries({ queryKey: ["github-auth"] });
    qc.invalidateQueries({ queryKey: ["github"] });
  }, [done, qc]);
  const failed = code && state.data && state.data.state !== "pending" && state.data.state !== "done" ? state.data.error : "";
  const copy = async () => {
    if (!code) return;
    try { await navigator.clipboard.writeText(code.user_code); setCopied(true); setTimeout(() => setCopied(false), 1500); } catch { /* clipboard unavailable */ }
  };
  const begin = () => { qc.removeQueries({ queryKey: ["github-device"] }); start.mutate(); };
  if (!code || failed) {
    return (
      <div className="space-y-2">
        <Button onClick={begin} disabled={start.isPending}>{start.isPending ? "Conectando…" : "Iniciar sesión con GitHub"}</Button>
        {failed && <p className="text-sm text-red-600 dark:text-red-400">{failed}</p>}
        <ErrorText error={start.error} />
      </div>
    );
  }
  return (
    <div className="space-y-3">
      <p className="text-sm text-slate-600 dark:text-muted">Introduce este código en GitHub para autorizar este nodo:</p>
      <div className="flex items-center gap-2">
        <code className="rounded-md border border-slate-200 px-3 py-1.5 font-mono text-lg tracking-widest dark:border-border">{code.user_code}</code>
        <Button variant="ghost" onClick={copy} aria-label="Copiar código"><Copy className="size-4" aria-hidden="true" />{copied ? "Copiado" : "Copiar"}</Button>
      </div>
      <p className="text-sm text-slate-600 dark:text-muted">
        Si no se abrió, ve a <a href={code.verification_uri} target="_blank" rel="noopener noreferrer" className="underline">{code.verification_uri}</a>. Esperando la autorización…
      </p>
      <Button variant="ghost" onClick={() => setCode(null)}>Cancelar</Button>
    </div>
  );
}
