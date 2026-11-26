export type MemoryCitationEntry = {
  file: string;
  lines?: string;
  note?: string;
};

const CITATION_BLOCK_RE = /<oai-mem-citation>([\s\S]*?)<\/oai-mem-citation>/g;
const ENTRIES_BLOCK_RE = /<citation_entries>([\s\S]*?)<\/citation_entries>/g;
const PARTIAL_OPEN_RE = /<oai-mem-citation[\s\S]*$/;

function parseEntryLine(raw: string): MemoryCitationEntry | null {
  const line = raw.trim();
  if (!line) return null;

  const pipeIdx = line.indexOf("|");
  const refPart = (pipeIdx >= 0 ? line.slice(0, pipeIdx) : line).trim();
  const metaPart = pipeIdx >= 0 ? line.slice(pipeIdx + 1).trim() : "";

  let file = refPart;
  let lines: string | undefined;
  const lineMatch = refPart.match(/^(.*):(\d+(?:-\d+)?)$/);
  if (lineMatch && lineMatch[1].trim()) {
    file = lineMatch[1].trim();
    lines = lineMatch[2];
  }

  let note: string | undefined;
  if (metaPart) {
    const noteMatch = metaPart.match(/note=\[([\s\S]*?)\]/) ?? metaPart.match(/note=([^|]+)/);
    if (noteMatch?.[1]?.trim()) {
      note = noteMatch[1].trim();
    }
  }

  if (!file && !note) return null;
  return { file, lines, note };
}

// extractMemoryCitations strips Codex <oai-mem-citation> protocol blocks from
// assistant prose and returns the cleaned Markdown body plus parsed entries.
export function extractMemoryCitations(text: string, streaming = false): {
  body: string;
  citations: MemoryCitationEntry[];
} {
  if (!text.includes("<oai-mem-citation")) {
    return { body: text, citations: [] };
  }

  const citations: MemoryCitationEntry[] = [];
  let body = text.replace(CITATION_BLOCK_RE, (_match, inner: string) => {
    for (const entriesMatch of inner.matchAll(ENTRIES_BLOCK_RE)) {
      for (const line of entriesMatch[1].split(/\r?\n/)) {
        const parsed = parseEntryLine(line);
        if (parsed) citations.push(parsed);
      }
    }
    return "";
  });

  if (streaming) {
    body = body.replace(PARTIAL_OPEN_RE, "");
  }

  return { body: body.trimEnd(), citations };
}

export function MemoryCitations({ citations }: { citations: MemoryCitationEntry[] }) {
  if (!citations.length) return null;
  return (
    <aside aria-label="Citas de memoria" className="mt-3 rounded-lg border border-slate-200/80 bg-slate-50/70 px-3 py-2 text-xs text-slate-900 dark:border-border dark:bg-surface/70 dark:text-white">
      <div className="mb-1 text-[0.6875rem] font-medium uppercase tracking-wide text-slate-500 dark:text-white/70">
        Memoria citada
      </div>
      <ul className="space-y-1">
        {citations.map((c, i) => (
          <li key={`${c.file}:${c.lines ?? ""}:${i}`} className="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 text-slate-900 dark:text-white">
            {c.file && (
              <span className="font-mono font-medium text-slate-900 dark:text-white">
                {c.file}
                {c.lines ? `:${c.lines}` : ""}
              </span>
            )}
            {c.note && <span className="text-slate-900 dark:text-white">— {c.note}</span>}
          </li>
        ))}
      </ul>
    </aside>
  );
}
