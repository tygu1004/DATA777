import { CheckCircle2, CircleDot, GitCommit, History, Loader2, RotateCcw } from "lucide-react";
import type { Commit } from "../types";

interface Props {
  commits: Commit[];
  onUndo: () => void;
  undoing: boolean;
}

export default function CommitHistory({ commits, onUndo, undoing }: Props) {
  const head = commits.find((c) => c.is_head);

  return (
    <aside className="flex w-72 shrink-0 flex-col border-l border-slate-800/80 bg-[#0b0d15] text-slate-200 select-none z-10">
      {/* Header */}
      <div className="flex h-11 items-center justify-between border-b border-slate-800/60 px-4">
        <div className="flex items-center gap-2">
          <History className="h-3.5 w-3.5 text-indigo-400" />
          <span className="text-xs font-semibold uppercase tracking-wider text-slate-300">History</span>
          {commits.length > 0 && (
            <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-slate-800 px-1 text-[10px] font-mono text-slate-400">
              {commits.length}
            </span>
          )}
        </div>

        <button
          disabled={!head || undoing}
          onClick={onUndo}
          title="Undo latest commit (revert to parent)"
          className="flex items-center gap-1.5 rounded-lg border border-slate-800 bg-slate-900/80 px-2 py-1 text-xs font-medium text-slate-200 hover:border-indigo-500/50 hover:bg-indigo-600/20 hover:text-indigo-200 disabled:opacity-40 disabled:pointer-events-none transition-all active:scale-[0.98]"
        >
          {undoing ? (
            <>
              <Loader2 className="h-3 w-3 animate-spin text-indigo-400" />
              <span>Undoing…</span>
            </>
          ) : (
            <>
              <RotateCcw className="h-3 w-3 text-indigo-400" />
              <span>Undo HEAD</span>
            </>
          )}
        </button>
      </div>

      {/* Commit List / Timeline */}
      <div className="flex-1 overflow-y-auto p-3">
        {commits.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-center text-slate-500">
            <GitCommit className="h-8 w-8 stroke-[1.5] text-slate-700 mb-2" />
            <div className="text-xs font-medium">No commits yet</div>
            <div className="text-[11px] text-slate-600 mt-0.5">Tagging or editing will create commits</div>
          </div>
        ) : (
          <div className="relative pl-3 space-y-3 before:absolute before:left-[19px] before:top-2 before:bottom-3 before:w-[2px] before:bg-slate-800/80">
            {commits.map((c) => {
              const isHead = c.is_head;

              return (
                <div key={c.id} className="relative flex items-start gap-2.5">
                  {/* Timeline Node Icon */}
                  <div className="relative z-10 mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[#0b0d15]">
                    {isHead ? (
                      <CircleDot className="h-4 w-4 text-indigo-400 animate-pulse" />
                    ) : (
                      <CheckCircle2 className="h-3.5 w-3.5 text-slate-600" />
                    )}
                  </div>

                  {/* Commit Card */}
                  <div
                    className={`flex-1 rounded-xl border p-2.5 transition-all ${
                      isHead
                        ? "border-indigo-500/40 bg-gradient-to-b from-indigo-950/30 to-slate-900/40 shadow-sm"
                        : "border-slate-800/70 bg-slate-900/30 hover:border-slate-700/80"
                    }`}
                  >
                    <div className="flex items-center justify-between gap-1 mb-1">
                      <div className="flex items-center gap-1.5">
                        <span className="font-mono text-[11px] font-semibold text-slate-400">#{c.id}</span>
                        <span
                          className={`rounded px-1.5 py-0.2 text-[9px] font-semibold uppercase tracking-wider ${
                            c.kind === "set"
                              ? "bg-indigo-500/15 text-indigo-300 border border-indigo-500/20"
                              : "bg-emerald-500/15 text-emerald-300 border border-emerald-500/20"
                          }`}
                        >
                          {c.kind}
                        </span>
                      </div>

                      {isHead && (
                        <span className="flex items-center gap-1 rounded bg-indigo-500/20 px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider text-indigo-300 border border-indigo-500/30">
                          HEAD
                        </span>
                      )}
                    </div>

                    <div className="text-xs font-medium text-slate-200 break-words mb-1">
                      {c.message || "(no message)"}
                    </div>

                    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-slate-500 font-mono">
                      <span>{c.field}</span>
                      <span>·</span>
                      <span className="text-slate-400">{c.affected_count.toLocaleString()} affected</span>
                      <span>·</span>
                      <span>{new Date(c.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</span>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </aside>
  );
}

