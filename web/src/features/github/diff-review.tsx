import { GithubLogo } from "@phosphor-icons/react";
import { Button, ErrorText } from "../../components/ui";
import { Modal } from "../../components/ui";
import { useLaunchForm } from "../composer/launch-picker";
import type { LaunchChoice } from "../../api/client";
import { useArm } from "./github-ui";
import { formAction } from "./diff-parts";

// ReviewLaunch picks the agent of the pull request's review thread; the first
// question creates it.
export function ReviewLaunch({ pr, busy, error, onClose, onLaunch }: { repo?: string; pr: number; busy: boolean; error: unknown; onClose: () => void; onLaunch: (l: LaunchChoice) => void }) {
  const form = useLaunchForm({ section: "pull-review" });
  return (
    <Modal title={`Crear el hilo de revisión de #${pr}`} onClose={onClose}>
      {form.fields}
      <p className="text-xs text-slate-500 dark:text-muted">Es la primera pregunta sobre esta PR: se crea un hilo para revisarla, sin modificar nada salvo que lo pidas, y recibe tu pregunta. Las siguientes van al mismo hilo.</p>
      <ErrorText error={form.error ?? error} />
      <Button disabled={!form.ready || busy} onClick={() => { form.remember(); onLaunch(form.value); }}>{busy ? "Creando…" : "Crear y preguntar"}</Button>
    </Modal>
  );
}

// PublishButton posts to GitHub only on a second click: the first arms it and
// changes its label, and it disarms by itself after a few seconds.
export function PublishButton({ disabled, onPublish }: { disabled: boolean; onPublish: () => void }) {
  const [armed, click] = useArm(onPublish);
  return (
    <button type="button" disabled={disabled} onClick={click} title="Publica el comentario en GitHub; pide confirmación"
      className={`${formAction} ${armed ? "!text-amber-700 dark:!text-amber-400" : ""}`}><GithubLogo weight="light" className="size-4" />{armed ? "Confirmar publicación" : "Publicar en GitHub"}</button>
  );
}

