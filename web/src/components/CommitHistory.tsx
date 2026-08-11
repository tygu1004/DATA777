import type { Commit } from "../types";

interface Props {
  commits: Commit[];
  onUndo: () => void;
  undoing: boolean;
}

export default function CommitHistory({ commits, onUndo, undoing }: Props) {
  const head = commits.find((c) => c.is_head);

  return (
    <div className="w-64 shrink-0 overflow-y-auto border-l border-white/10 p-3">
      <div className="mb-2 flex items-center justify-between">
        <strong className="text-sm text-neutral-200">Commit history</strong>
        <button
          disabled={!head || undoing}
          onClick={onUndo}
          className="rounded border border-white/10 px-2 py-1 text-xs text-neutral-200 hover:bg-white/5 disabled:opacity-40"
        >
          {undoing ? "Undoing…" : "Undo"}
        </button>
      </div>
      {commits.length === 0 && <p className="text-xs text-neutral-500">No commits yet.</p>}
      <ul className="space-y-1 text-sm">
        {commits.map((c) => (
          <li key={c.id} className={`rounded px-2 py-1.5 ${c.is_head ? "bg-blue-500/15" : ""}`}>
            <div className="flex items-center gap-1 text-neutral-200">
              #{c.id}
              <span className="rounded bg-white/10 px-1 text-[10px] uppercase text-neutral-400">{c.kind}</span>
              {c.is_head && <strong className="text-blue-400">HEAD</strong>}
            </div>
            <div className="text-neutral-400">{c.message || "(no message)"}</div>
            <div className="text-xs text-neutral-500">
              {c.field} · {c.affected_count} affected · {new Date(c.created_at).toLocaleTimeString()}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
