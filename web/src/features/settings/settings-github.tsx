import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { GithubLogin } from "../../components/github-login";
import { Button, ErrorText } from "../../components/ui";
import { qk } from "../../api/queries/keys";

const sources = { env: "la variable DORY_GITHUB_TOKEN", ui: "el inicio de sesión de Dory", gh: "la CLI gh", none: "" };

// GithubSettings shows which account this node uses for GitHub and links or
// unlinks it. The token never reaches the interface.
export function GithubSettings() {
  const qc = useQueryClient();
  const auth = useQuery({ queryKey: qk.githubAuth(), queryFn: api.githubAuth });
  const logout = useMutation({
    mutationFn: api.githubLogout,
    onSuccess: () => { qc.invalidateQueries({ queryKey: qk.githubAuth() }); qc.invalidateQueries({ queryKey: qk.github() }); },
  });
  const a = auth.data;
  return (
    <section aria-labelledby="github-title" className="space-y-3">
      <h2 id="github-title" className="text-lg font-semibold">GitHub</h2>
      <p className="text-sm text-slate-600 dark:text-muted">Dory lee issues y pull requests con tu cuenta de GitHub. El token se guarda solo en este nodo.</p>
      {a && a.source !== "none" && (
        <p className="text-sm">
          Conectado{a.login ? <> como <strong>@{a.login}</strong></> : ""} mediante {sources[a.source]}.
          {a.source === "env" && " Para cambiarlo, quita la variable y reinicia el nodo."}
        </p>
      )}
      {a?.source === "none" && <p className="text-sm">Sin conectar.</p>}
      {a?.source === "ui" && <Button variant="danger" onClick={() => logout.mutate()} disabled={logout.isPending}>Cerrar sesión</Button>}
      {a && (a.source === "none" || a.source === "gh") && <GithubLogin />}
      <ErrorText error={auth.error ?? logout.error} />
    </section>
  );
}
