import type { SectionProps } from "./settings-node";

// WorktreesSettings is the "Worktrees" section. It is a placeholder: a later phase fills it
// in. It receives the selected node ("all" or a node id), see settings-node.tsx.
export function WorktreesSettings({ node }: SectionProps) {
  return (
    <section className="space-y-3" data-node={node}>
      <h2 className="text-lg font-semibold">Worktrees</h2>
      <p className="text-sm text-slate-600 dark:text-muted">Próximamente.</p>
    </section>
  );
}
