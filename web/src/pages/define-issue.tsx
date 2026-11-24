import { DictationField } from "../features/composer/dictation-field";
import { useDictationInProgress } from "../features/composer/local-dictation";
import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { api, repoName } from "../api/client";
import { Button, ErrorText, Input, Label, Modal } from "../components/ui";
import { useLaunchForm } from "../features/composer/launch-picker";
import { autoMessage } from "../lib/auto-message";
import { qk } from "../api/queries/keys";
import { useRepositories } from "../api/queries/hooks";

// DefineIssueDialog turns an issue into tasks, by hand or with an agent that
// splits it with the MCP, or asks an agent to resolve a pull request's
// conflicts (resolve).
export function DefineIssueDialog({ resolve = false, repo, number, onClose }: { resolve?: boolean; repo: string; number: number; onClose: () => void }) {
  const navigate = useNavigate();
  const dictating = useDictationInProgress();
  const repos = useRepositories();
  const registered = repos.data?.find((r) => repoName(r.remote_url).toLowerCase() === repo.toLowerCase());
  const form = useLaunchForm({ section: resolve ? "resolve" : "define-issue" });
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const manual = useMutation({
    mutationFn: () => api.issueTask(repo, number, { title, description }),
    onSuccess: (t) => navigate({ to: "/tasks", search: { task: t.id } as never }),
  });
  const agent = useMutation({
    mutationFn: () => {
      form.remember();
      const context = resolve
        ? `Resuelve los conflictos de merge de la pull request #${number} de ${repo}. Usa \`gh pr checkout ${number}\` en un worktree aparte, fusiona la rama base, resuelve los conflictos, ejecuta los tests y sube la rama.`
        : `Analiza la issue #${number} de ${repo} (\`gh issue view ${number} --repo ${repo}\`) y el repositorio. Pártela en tareas pequeñas y bien definidas con la herramienta save_task del MCP de Dory: cada una con qué cambiar y cómo probarlo, issue_number ${number}, el project_id del proyecto y depends_on cuando una deba ir antes que otra. No implementes nada.`;
      const text = resolve ? autoMessage("pr-conflicts", { repo, pr: number }, context) : autoMessage("issue-tasks", { repo, issue: number }, context);
      return api.createThread({ ...form.value, project_id: registered?.project_id ?? null, repository_id: registered?.id, title: resolve ? `Conflictos de ${repo}#${number}` : `Tareas de ${repo}#${number}`, text });
    },
    onSuccess: (r) => navigate({ to: "/threads/$threadId", params: { threadId: r.thread_id } }),
  });
  return (
    <Modal title={resolve ? `Resolver conflictos de #${number}` : `Tareas de la issue #${number}`} onClose={onClose}>
      <div className="space-y-4">
        {!registered && repos.isSuccess && <p className="text-sm text-amber-700 dark:text-amber-300">El repositorio no está en ningún proyecto de Dory: añádelo para que las tareas queden ligadas.</p>}
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">{resolve ? "Con un agente" : "Que un agente la parta en tareas"}</h3>
          {form.fields}
          <Button onClick={() => agent.mutate()} disabled={!form.ready || agent.isPending}>{resolve ? "Lanzar" : "Definir tareas"}</Button>
          <ErrorText error={agent.error ?? form.error} />
        </section>
        {!resolve && registered && (
          <section className="space-y-2 border-t border-slate-200 pt-3 dark:border-border">
            <h3 className="text-sm font-semibold">O crea una tarea a mano</h3>
            <Label text="Título"><Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={`Issue #${number}`} /></Label>
            <DictationField label="Descripción" rows={3} value={description} onValue={setDescription} />
            <Button variant="ghost" onClick={() => manual.mutate()} disabled={dictating || manual.isPending}>Crear tarea</Button>
            <ErrorText error={manual.error} />
          </section>
        )}
      </div>
    </Modal>
  );
}
