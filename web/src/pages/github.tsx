import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type GithubEntry } from "../api/client";
import { Badge, Button, Card, ErrorText } from "../components/ui";

function lineColor(l: string) {
  if (l.startsWith("+") && !l.startsWith("+++")) return "bg-green-100 dark:bg-green-950";
  if (l.startsWith("-") && !l.startsWith("---")) return "bg-red-100 dark:bg-red-950";
  if (l.startsWith("@@") || l.startsWith("diff ")) return "text-slate-500 dark:text-slate-400";
  return "";
}

function Diff({ entry }: { entry: GithubEntry }) {
  const diff = useQuery({ queryKey: ["github-diff", entry.repo, entry.number], queryFn: () => api.githubDiff(entry.repo, entry.number) });
  return (
    <>
      <ErrorText error={diff.error} />
      <pre className="mt-2 max-h-[70vh] overflow-auto text-xs">{diff.data?.diff.split("\n").map((l, i) => <div key={i} className={lineColor(l)}>{l || " "}</div>)}</pre>
    </>
  );
}

export function Github() {
  const [open, setOpen] = useState<string | null>(null);
  const entries = useQuery({ queryKey: ["github"], queryFn: api.github, refetchInterval: 60_000, retry: false });
  return (
    <div className="space-y-3">
      <h1 className="text-xl font-bold">GitHub</h1>
      <ErrorText error={entries.error} />
      {entries.data?.map((e) => {
        const key = `${e.repo}#${e.number}`;
        return (
          <Card key={key}>
            <div className="flex items-start justify-between gap-2">
              <a href={e.url} target="_blank" rel="noreferrer" className="min-w-0 break-words font-medium">{e.title}</a>
              {e.kind === "pull" && <Button variant="ghost" onClick={() => setOpen(open === key ? null : key)}>Diff</Button>}
            </div>
            <div className="flex flex-wrap gap-1"><Badge>{key}</Badge><Badge>{e.kind === "pull" ? (e.draft ? "PR draft" : "PR") : "Issue"}</Badge><Badge>{e.author}</Badge></div>
            {open === key && <Diff entry={e} />}
          </Card>
        );
      })}
      {entries.data?.length === 0 && <p className="text-sm text-slate-500 dark:text-slate-400">Nada abierto.</p>}
    </div>
  );
}
