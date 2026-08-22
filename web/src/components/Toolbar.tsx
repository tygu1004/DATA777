import {
  CheckSquare,
  Download,
  Eye,
  EyeOff,
  Grid2X2,
  Grid3X3,
  LayoutGrid,
  Loader2,
  Plus,
  Square,
  Tag as TagIcon,
  Trash2,
  X,
} from "lucide-react";
import { useState } from "react";
import type { TagCount } from "../types";

export type GridSize = "small" | "medium" | "large";

interface Props {
  selectedCount: number;
  allMatching: boolean;
  matchingCount: number;
  busy: boolean;
  onApplyTag: (tag: string) => void;
  onRemoveTag: (tag: string) => void;
  onSelectAllMatching: () => void;
  onClearSelection: () => void;
  gridSize: GridSize;
  onGridSizeChange: (size: GridSize) => void;
  showLabels: boolean;
  onToggleShowLabels: () => void;
  popularTags?: TagCount[];
  onOpenExport?: () => void;
}

export default function Toolbar({
  selectedCount,
  allMatching,
  matchingCount,
  busy,
  onApplyTag,
  onRemoveTag,
  onSelectAllMatching,
  onClearSelection,
  gridSize,
  onGridSizeChange,
  showLabels,
  onToggleShowLabels,
  popularTags = [],
  onOpenExport,
}: Props) {
  const [tag, setTag] = useState("");
  const hasSelection = allMatching || selectedCount > 0;

  const submit = (apply: (tag: string) => void, tagToApply?: string) => {
    const targetTag = (tagToApply ?? tag).trim();
    if (!targetTag || !hasSelection) return;
    apply(targetTag);
    if (!tagToApply) setTag("");
  };

  const topTags = popularTags.slice(0, 4);

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800/60 bg-[#0e111a]/90 px-4 py-2 text-xs backdrop-blur-sm z-10">
      {/* Left: Selection & Tag Actions */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Selection Status Badge */}
        <div
          className={`flex items-center gap-2 rounded-lg border px-2.5 py-1.5 transition-all ${
            hasSelection
              ? "border-indigo-500/40 bg-indigo-950/30 text-indigo-200"
              : "border-slate-800/80 bg-slate-900/40 text-slate-400"
          }`}
        >
          {allMatching ? (
            <CheckSquare className="h-4 w-4 text-indigo-400" />
          ) : selectedCount > 0 ? (
            <CheckSquare className="h-4 w-4 text-cyan-400" />
          ) : (
            <Square className="h-4 w-4 text-slate-500" />
          )}

          <span className="font-medium">
            {allMatching
              ? `All ${matchingCount.toLocaleString()} matching selected`
              : selectedCount > 0
                ? `${selectedCount.toLocaleString()} selected`
                : "No selection"}
          </span>

          {hasSelection && (
            <button
              onClick={onClearSelection}
              title="Clear selection"
              className="ml-1 rounded p-0.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>

        {/* Select All Matching Button */}
        {matchingCount > 0 && !allMatching && (
          <button
            onClick={onSelectAllMatching}
            className="flex items-center gap-1.5 rounded-lg border border-slate-800 bg-slate-900/60 px-2.5 py-1.5 text-slate-300 hover:border-slate-700 hover:bg-slate-800/80 hover:text-white transition-all active:scale-[0.98]"
          >
            <CheckSquare className="h-3.5 w-3.5 text-slate-400" />
            <span>Select all matching ({matchingCount.toLocaleString()})</span>
          </button>
        )}

        <div className="h-4 w-[1px] bg-slate-800 mx-1 hidden sm:block" />

        {/* Tag Input & Buttons */}
        <div className="flex items-center gap-1.5">
          <div className="relative">
            <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-2.5 text-slate-500">
              <TagIcon className="h-3.5 w-3.5" />
            </div>
            <input
              type="text"
              value={tag}
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") submit(onApplyTag);
              }}
              placeholder={hasSelection ? "Tag name…" : "Select items to tag"}
              disabled={!hasSelection || busy}
              className="w-36 rounded-lg border border-slate-800 bg-slate-950/70 py-1.5 pl-8 pr-2.5 text-xs text-slate-200 placeholder:text-slate-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 disabled:opacity-50 transition-all"
            />
          </div>

          <button
            disabled={!hasSelection || !tag.trim() || busy}
            onClick={() => submit(onApplyTag)}
            className="flex items-center gap-1 rounded-lg bg-indigo-600 px-2.5 py-1.5 font-medium text-white shadow-sm hover:bg-indigo-500 disabled:opacity-40 disabled:pointer-events-none transition-all active:scale-[0.98]"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>Add</span>
          </button>

          <button
            disabled={!hasSelection || !tag.trim() || busy}
            onClick={() => submit(onRemoveTag)}
            className="flex items-center gap-1 rounded-lg border border-slate-800 bg-slate-900/60 px-2.5 py-1.5 text-slate-300 hover:border-red-500/40 hover:bg-red-500/10 hover:text-red-300 disabled:opacity-40 disabled:pointer-events-none transition-all active:scale-[0.98]"
          >
            <Trash2 className="h-3.5 w-3.5" />
            <span>Remove</span>
          </button>
        </div>

        {/* Quick Tag Pills */}
        {hasSelection && topTags.length > 0 && (
          <div className="hidden lg:flex items-center gap-1.5 ml-2">
            <span className="text-[11px] text-slate-500 font-medium">Quick:</span>
            {topTags.map((t) => (
              <button
                key={t.tag}
                disabled={busy}
                onClick={() => submit(onApplyTag, t.tag)}
                className="flex items-center gap-1 rounded-md border border-slate-800 bg-slate-900/80 px-2 py-1 text-[11px] text-slate-300 hover:border-indigo-500/50 hover:bg-indigo-500/10 hover:text-indigo-200 transition-all"
              >
                <span>+{t.tag}</span>
              </button>
            ))}
          </div>
        )}

        {busy && (
          <div className="flex items-center gap-1.5 text-indigo-400 pl-2">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            <span className="text-xs">Applying changes…</span>
          </div>
        )}
      </div>

      {/* Right: Grid Size & View Controls */}
      <div className="flex items-center gap-2">
        {/* Export Dataset Button */}
        {onOpenExport && (
          <button
            onClick={onOpenExport}
            title="Export dataset to CVAT, Label Studio, COCO, YOLO"
            className="flex items-center gap-1.5 rounded-lg border border-emerald-500/30 bg-emerald-950/40 px-2.5 py-1.5 text-xs font-medium text-emerald-300 hover:bg-emerald-900/50 hover:border-emerald-500/50 transition-all shadow-sm"
          >
            <Download className="h-3.5 w-3.5" />
            <span>Export</span>
            {selectedCount > 0 && (
              <span className="rounded bg-emerald-500/20 px-1 py-0.2 text-[10px] font-mono font-bold text-emerald-300">
                {selectedCount}
              </span>
            )}
          </button>
        )}

        {/* Label Overlay Toggle */}
        <button
          onClick={onToggleShowLabels}
          title={showLabels ? "Hide bounding boxes & labels" : "Show bounding boxes & labels"}
          className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs transition-all ${
            showLabels
              ? "border-indigo-500/30 bg-indigo-500/15 text-indigo-300"
              : "border-slate-800 bg-slate-900/60 text-slate-400 hover:border-slate-700 hover:text-slate-300"
          }`}
        >
          {showLabels ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5 text-slate-500" />}
          <span className="hidden sm:inline">{showLabels ? "Labels ON" : "Labels OFF"}</span>
        </button>

        {/* Grid Size Switcher */}
        <div className="flex items-center rounded-lg border border-slate-800 bg-slate-900/60 p-0.5">
          <button
            onClick={() => onGridSizeChange("small")}
            title="Small thumbnails"
            className={`flex h-7 w-7 items-center justify-center rounded-md transition-all ${
              gridSize === "small"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/50"
            }`}
          >
            <Grid3X3 className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => onGridSizeChange("medium")}
            title="Medium thumbnails"
            className={`flex h-7 w-7 items-center justify-center rounded-md transition-all ${
              gridSize === "medium"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/50"
            }`}
          >
            <LayoutGrid className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => onGridSizeChange("large")}
            title="Large thumbnails"
            className={`flex h-7 w-7 items-center justify-center rounded-md transition-all ${
              gridSize === "large"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/50"
            }`}
          >
            <Grid2X2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}

