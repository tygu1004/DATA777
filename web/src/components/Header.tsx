import {
  Database,
  FolderOpen,
  History,
  Layers,
  Loader2,
  PanelLeftClose,
  PanelLeftOpen,
  Sparkles,
} from "lucide-react";
import type { Job } from "../types";

interface Props {
  folderPath: string;
  onFolderPathChange: (path: string) => void;
  onStartIndex: () => void;
  indexJob: Job | null;
  totalCount: number;
  filteredCount: number;
  hasFilter: boolean;
  selectedCount: number;
  showSidebar: boolean;
  onToggleSidebar: () => void;
  showHistory: boolean;
  onToggleHistory: () => void;
  commitsCount: number;
}

export default function Header({
  folderPath,
  onFolderPathChange,
  onStartIndex,
  indexJob,
  totalCount,
  filteredCount,
  hasFilter,
  selectedCount,
  showSidebar,
  onToggleSidebar,
  showHistory,
  onToggleHistory,
  commitsCount,
}: Props) {
  const isIndexing = indexJob && (indexJob.status === "running" || indexJob.status === "queued");

  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-slate-800/80 bg-[#0c0e17]/95 px-4 backdrop-blur-md z-20">
      {/* Left: Brand Logo & Sidebar Toggle */}
      <div className="flex items-center gap-3">
        <button
          onClick={onToggleSidebar}
          title={showSidebar ? "Collapse Filters" : "Expand Filters"}
          className={`flex h-8 w-8 items-center justify-center rounded-lg border transition-all ${
            showSidebar
              ? "border-slate-700 bg-slate-800/60 text-indigo-400 hover:bg-slate-700/80"
              : "border-slate-800 bg-slate-900/60 text-slate-400 hover:border-slate-700 hover:text-slate-200"
          }`}
        >
          {showSidebar ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeftOpen className="h-4 w-4" />}
        </button>

        <div className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-tr from-indigo-600 to-cyan-500 shadow-md shadow-indigo-500/20">
            <Database className="h-4 w-4 text-white" />
          </div>
          <div className="flex items-baseline gap-1.5">
            <span className="bg-gradient-to-r from-white via-slate-200 to-slate-400 bg-clip-text text-base font-bold tracking-tight text-transparent">
              data777
            </span>
            <span className="rounded bg-indigo-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-indigo-400 border border-indigo-500/20">
              v2
            </span>
          </div>
        </div>
      </div>

      {/* Center: Indexing Directory Input */}
      <div className="flex max-w-xl flex-1 items-center gap-2 px-6">
        <div className="relative flex-1">
          <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-slate-500">
            <FolderOpen className="h-4 w-4" />
          </div>
          <input
            type="text"
            value={folderPath}
            onChange={(e) => onFolderPathChange(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && onStartIndex()}
            placeholder="Enter dataset path (e.g. /path/to/images or ./devdata)"
            className="w-full rounded-lg border border-slate-800 bg-slate-950/70 py-1.5 pl-9 pr-3 text-xs text-slate-200 placeholder:text-slate-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 transition-all font-mono"
          />
        </div>

        <button
          onClick={onStartIndex}
          disabled={!folderPath.trim() || Boolean(isIndexing)}
          className="flex items-center gap-1.5 rounded-lg bg-gradient-to-r from-indigo-600 to-indigo-500 px-3 py-1.5 text-xs font-medium text-white shadow-sm shadow-indigo-600/30 hover:from-indigo-500 hover:to-indigo-400 disabled:opacity-50 disabled:pointer-events-none transition-all active:scale-[0.98]"
        >
          {isIndexing ? (
            <>
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              <span>Indexing…</span>
            </>
          ) : (
            <>
              <Sparkles className="h-3.5 w-3.5" />
              <span>Index Dataset</span>
            </>
          )}
        </button>

        {isIndexing && (
          <div className="flex items-center gap-2 rounded-lg border border-indigo-500/30 bg-indigo-950/40 px-2.5 py-1 text-xs text-indigo-300 animate-pulse">
            <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" />
            <span>
              {indexJob.progress.processed} {indexJob.progress.total ? `/ ${indexJob.progress.total}` : "indexed"}
            </span>
          </div>
        )}
      </div>

      {/* Right: Real-time Stats & History Toggle */}
      <div className="flex items-center gap-2.5">
        {/* Stats Pills */}
        <div className="hidden sm:flex items-center gap-2 rounded-lg border border-slate-800/80 bg-slate-900/60 p-1 text-xs">
          <div className="flex items-center gap-1.5 px-2 py-0.5 text-slate-300" title="Total samples in dataset">
            <Layers className="h-3.5 w-3.5 text-slate-400" />
            <span className="font-mono font-medium">{totalCount.toLocaleString()}</span>
            <span className="text-[11px] text-slate-500">samples</span>
          </div>

          {hasFilter && (
            <div
              className="flex items-center gap-1.5 rounded-md bg-indigo-500/15 border border-indigo-500/30 px-2 py-0.5 text-indigo-300"
              title="Samples matching active filters"
            >
              <span className="font-mono font-semibold">{filteredCount.toLocaleString()}</span>
              <span className="text-[11px] text-indigo-400">matched</span>
            </div>
          )}

          {selectedCount > 0 && (
            <div
              className="flex items-center gap-1.5 rounded-md bg-cyan-500/15 border border-cyan-500/30 px-2 py-0.5 text-cyan-300"
              title="Explicitly selected samples"
            >
              <span className="font-mono font-semibold">{selectedCount.toLocaleString()}</span>
              <span className="text-[11px] text-cyan-400">selected</span>
            </div>
          )}
        </div>

        {/* History Toggle Button */}
        <button
          onClick={onToggleHistory}
          title={showHistory ? "Hide Commit History" : "Show Commit History"}
          className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition-all ${
            showHistory
              ? "border-slate-700 bg-slate-800/80 text-indigo-300"
              : "border-slate-800 bg-slate-900/60 text-slate-400 hover:border-slate-700 hover:text-slate-200"
          }`}
        >
          <History className="h-3.5 w-3.5" />
          <span className="hidden md:inline">History</span>
          {commitsCount > 0 && (
            <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-slate-800 px-1 text-[10px] font-mono text-slate-300">
              {commitsCount}
            </span>
          )}
        </button>
      </div>
    </header>
  );
}
