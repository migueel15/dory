import type { SectionProps } from "./settings-node";

// ThreadsSettings is the "Hilos" section. It is a placeholder: a later phase fills it
// in. It receives the selected node ("all" or a node id), see settings-node.tsx.
export function ThreadsSettings({ node }: SectionProps) {
  return (
    <section className="space-y-3" data-node={node}>
      <h2 className="text-lg font-semibold">Hilos</h2>
      <p className="text-sm text-slate-600 dark:text-muted">Próximamente.</p>
    </section>
  );
}
