import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckSquare,
  Copy,
  Database,
  Eye,
  EyeOff,
  FileImage,
  Layers,
  Plus,
  Sparkles,
  Square,
  Tag as TagIcon,
  X,
} from "lucide-react";
import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import { previewUrl } from "../api/client";
import { CHUNK_SIZE } from "../hooks/useSamples";
import type { Sample } from "../types";

interface Props {
  chunks: Map<number, Sample[]>;
  count: number;
  index: number;
  onClose: () => void;
  onNavigate: (index: number) => void;
  selected: Set<number>;
  onToggleSelect: (id: number, index: number) => void;
  canFindSimilar: boolean;
  onFindSimilar: (id: number) => void;
  onApplyTag?: (tag: string, op: "add" | "remove") => void;
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function colorForLabel(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) | 0;
  return `hsl(${Math.abs(hash) % 360}, 85%, 65%)`;
}

export default function SampleModal({
  chunks,
  count,
  index,
  onClose,
  onNavigate,
  selected,
  onToggleSelect,
  canFindSimilar,
  onFindSimilar,
  onApplyTag,
}: Props) {
  const [showLabels, setShowLabels] = useState(true);
  const [newTag, setNewTag] = useState("");
  const [copied, setCopied] = useState(false);

  // Resolve current sample from chunks
  const sample = useMemo(() => {
    const chunkOffset = Math.floor(index / CHUNK_SIZE) * CHUNK_SIZE;
    return chunks.get(chunkOffset)?.[index - chunkOffset];
  }, [chunks, index]);

  const isSelected = sample ? selected.has(sample.id) : false;

  // Keyboard navigation: ArrowLeft, ArrowRight, Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;

      if (e.key === "Escape") {
        onClose();
      } else if (e.key === "ArrowLeft" && index > 0) {
        onNavigate(index - 1);
      } else if (e.key === "ArrowRight" && index < count - 1) {
        onNavigate(index + 1);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [index, count, onClose, onNavigate]);

  const handleCopyPath = useCallback(() => {
    if (!sample) return;
    navigator.clipboard.writeText(sample.path);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [sample]);

  const handleAddTag = useCallback(() => {
    const trimmed = newTag.trim();
    if (!trimmed || !sample || !onApplyTag) return;
    onApplyTag(trimmed, "add");
    setNewTag("");
  }, [newTag, sample, onApplyTag]);

  const handleRemoveTag = useCallback(
    (tag: string) => {
      if (!sample || !onApplyTag) return;
      onApplyTag(tag, "remove");
    },
    [sample, onApplyTag],
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-3 sm:p-6 backdrop-blur-md animate-in fade-in duration-200"
      onClick={onClose}
    >
      {/* Modal Dialog Window */}
      <div
        className="relative flex h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-2xl border border-slate-800 bg-[#0c0e17] shadow-2xl shadow-black/90"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Top Header Bar */}
        <div className="flex h-13 shrink-0 items-center justify-between border-b border-slate-800/80 bg-slate-950/70 px-4 backdrop-blur-md">
          {/* Left: Sample Title & Index */}
          <div className="flex items-center gap-2.5 min-w-0">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
              <FileImage className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="truncate text-xs font-semibold text-slate-100 max-w-[280px] sm:max-w-md">
                  {sample?.filename ?? "Loading sample…"}
                </span>
                {sample && (
                  <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] font-mono text-slate-400 border border-slate-700/50 uppercase">
                    {sample.format || "img"}
                  </span>
                )}
              </div>
              <div className="text-[11px] text-slate-400 font-mono">
                Sample <span className="font-semibold text-slate-300">#{index + 1}</span> of {count.toLocaleString()}
              </div>
            </div>
          </div>

          {/* Center: Prev / Next Navigation Controls */}
          <div className="flex items-center gap-1.5 rounded-lg border border-slate-800 bg-slate-900/60 p-1">
            <button
              onClick={() => onNavigate(index - 1)}
              disabled={index <= 0}
              title="Previous sample (Left Arrow)"
              className="flex h-7 w-7 items-center justify-center rounded-md text-slate-300 hover:bg-slate-800 hover:text-white disabled:opacity-30 disabled:pointer-events-none transition-all"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <span className="px-2 font-mono text-xs text-slate-400">
              {index + 1} / {count}
            </span>
            <button
              onClick={() => onNavigate(index + 1)}
              disabled={index >= count - 1}
              title="Next sample (Right Arrow)"
              className="flex h-7 w-7 items-center justify-center rounded-md text-slate-300 hover:bg-slate-800 hover:text-white disabled:opacity-30 disabled:pointer-events-none transition-all"
            >
              <ArrowRight className="h-4 w-4" />
            </button>
          </div>

          {/* Right: View Controls & Close */}
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowLabels((prev) => !prev)}
              title={showLabels ? "Hide Labels" : "Show Labels"}
              className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs transition-all ${
                showLabels
                  ? "border-indigo-500/40 bg-indigo-500/15 text-indigo-300"
                  : "border-slate-800 bg-slate-900/60 text-slate-400 hover:text-slate-200"
              }`}
            >
              {showLabels ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
              <span className="hidden md:inline">{showLabels ? "Labels ON" : "Labels OFF"}</span>
            </button>

            <button
              onClick={onClose}
              title="Close inspector (Esc)"
              className="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-800 bg-slate-900/60 text-slate-400 hover:bg-slate-800 hover:text-white transition-all"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Main Content Area (2-Column: Image Viewer + Inspector Sidebar) */}
        <div className="flex min-h-0 flex-1 flex-col lg:flex-row overflow-hidden">
          {/* Left: Image Viewer with Label Overlays */}
          <div className="relative flex min-h-[350px] flex-1 items-center justify-center bg-[#07080c] p-4 overflow-hidden">
            {sample ? (
              <div className="relative flex max-h-full max-w-full items-center justify-center">
                <img
                  src={previewUrl(sample.id)}
                  alt={sample.filename}
                  className="max-h-[calc(90vh-80px)] max-w-full object-contain rounded-lg shadow-2xl"
                />

                {/* Bounding Box / Keypoint Overlays on Preview */}
                {showLabels && sample.labels && (
                  <div className="pointer-events-none absolute inset-0 overflow-hidden">
                    {Object.entries(sample.labels).flatMap(([fieldName, values]) =>
                      values.map((lv, i) => {
                        const label = typeof lv.label === "string" ? lv.label : "";
                        const color = colorForLabel(fieldName + label);
                        const bbox = Array.isArray(lv.bbox) ? (lv.bbox as number[]) : undefined;
                        const points = Array.isArray(lv.points) ? (lv.points as number[][]) : undefined;
                        return (
                          <Fragment key={`${fieldName}-${i}`}>
                            {bbox && (
                              <div
                                className="absolute border-2 shadow-sm"
                                style={{
                                  left: `${bbox[0] * 100}%`,
                                  top: `${bbox[1] * 100}%`,
                                  width: `${bbox[2] * 100}%`,
                                  height: `${bbox[3] * 100}%`,
                                  borderColor: color,
                                  backgroundColor: `${color}1a`,
                                }}
                              >
                                {label && (
                                  <span
                                    className="absolute -top-5 left-0 rounded px-1.5 py-0.2 text-[10px] font-bold text-black shadow-md truncate max-w-[120px]"
                                    style={{ backgroundColor: color }}
                                  >
                                    {label}
                                  </span>
                                )}
                              </div>
                            )}
                            {points?.map((pt, pi) => (
                              <div
                                key={pi}
                                className="absolute h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-black shadow-md"
                                style={{ left: `${pt[0] * 100}%`, top: `${pt[1] * 100}%`, backgroundColor: color }}
                              />
                            ))}
                          </Fragment>
                        );
                      }),
                    )}
                  </div>
                )}
              </div>
            ) : (
              <div className="text-center text-slate-500 animate-pulse text-xs">Loading image preview…</div>
            )}
          </div>

          {/* Right: Inspector Sidebar (Properties, Tags, Labels, Actions) */}
          <div className="flex w-full lg:w-84 shrink-0 flex-col border-t lg:border-t-0 lg:border-l border-slate-800/80 bg-[#0b0d15] text-slate-200 overflow-y-auto p-4 space-y-4">
            {/* Quick Action Buttons */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Actions</div>
              <div className="grid grid-cols-2 gap-2">
                {/* Select Toggle Button */}
                <button
                  onClick={() => sample && onToggleSelect(sample.id, index)}
                  className={`flex items-center justify-center gap-1.5 rounded-lg border px-3 py-2 text-xs font-medium transition-all ${
                    isSelected
                      ? "border-indigo-500/50 bg-indigo-600/20 text-indigo-300"
                      : "border-slate-800 bg-slate-900/70 text-slate-300 hover:border-slate-700 hover:bg-slate-800/60"
                  }`}
                >
                  {isSelected ? (
                    <>
                      <CheckSquare className="h-3.5 w-3.5 text-indigo-400" />
                      <span>Selected</span>
                    </>
                  ) : (
                    <>
                      <Square className="h-3.5 w-3.5 text-slate-400" />
                      <span>Select</span>
                    </>
                  )}
                </button>

                {/* Similarity Search Button */}
                {canFindSimilar && (
                  <button
                    onClick={() => {
                      if (!sample) return;
                      onFindSimilar(sample.id);
                      onClose();
                    }}
                    title="Find similar images using embeddings"
                    className="flex items-center justify-center gap-1.5 rounded-lg border border-purple-500/30 bg-purple-950/30 px-3 py-2 text-xs font-medium text-purple-300 hover:border-purple-500/50 hover:bg-purple-900/40 transition-all"
                  >
                    <Sparkles className="h-3.5 w-3.5 text-purple-400" />
                    <span>Find Similar</span>
                  </button>
                )}
              </div>
            </div>

            {/* Properties Card */}
            <div className="rounded-xl border border-slate-800/80 bg-slate-900/40 p-3 space-y-2.5">
              <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-slate-400">
                <Database className="h-3.5 w-3.5 text-slate-400" />
                <span>Image Properties</span>
              </div>

              {sample ? (
                <div className="grid grid-cols-2 gap-2 text-xs font-mono">
                  <div className="rounded-lg bg-slate-950/60 p-2 border border-slate-800/50">
                    <span className="text-[10px] text-slate-500 uppercase block">Resolution</span>
                    <span className="text-slate-200 font-semibold">
                      {sample.width} × {sample.height}
                    </span>
                  </div>
                  <div className="rounded-lg bg-slate-950/60 p-2 border border-slate-800/50">
                    <span className="text-[10px] text-slate-500 uppercase block">File Size</span>
                    <span className="text-slate-200 font-semibold">{formatBytes(sample.filesize)}</span>
                  </div>
                  <div className="rounded-lg bg-slate-950/60 p-2 border border-slate-800/50">
                    <span className="text-[10px] text-slate-500 uppercase block">Format</span>
                    <span className="text-slate-200 font-semibold uppercase">{sample.format || "image"}</span>
                  </div>
                  <div className="rounded-lg bg-slate-950/60 p-2 border border-slate-800/50">
                    <span className="text-[10px] text-slate-500 uppercase block">Sample ID</span>
                    <span className="text-indigo-400 font-semibold">#{sample.id}</span>
                  </div>
                </div>
              ) : (
                <div className="text-xs text-slate-500">Loading properties…</div>
              )}

              {/* Path copy row */}
              {sample && (
                <div className="flex items-center justify-between gap-1.5 rounded-lg bg-slate-950/70 p-2 text-xs border border-slate-800/60">
                  <span className="truncate font-mono text-[11px] text-slate-400" title={sample.path}>
                    {sample.path}
                  </span>
                  <button
                    onClick={handleCopyPath}
                    title="Copy full path"
                    className="flex shrink-0 items-center gap-1 rounded p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
                  >
                    {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                  </button>
                </div>
              )}
            </div>

            {/* Tags Section */}
            <div className="rounded-xl border border-slate-800/80 bg-slate-900/40 p-3 space-y-2.5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-slate-400">
                  <TagIcon className="h-3.5 w-3.5 text-slate-400" />
                  <span>Tags</span>
                </div>
                <span className="text-[11px] font-mono text-slate-500">{sample?.tags.length ?? 0}</span>
              </div>

              {/* Tag Badges List */}
              <div className="flex flex-wrap gap-1.5 min-h-[32px]">
                {sample && sample.tags.length > 0 ? (
                  sample.tags.map((t) => (
                    <div
                      key={t}
                      className="group flex items-center gap-1 rounded-lg border border-indigo-500/30 bg-indigo-950/40 px-2 py-1 text-xs text-indigo-200"
                    >
                      <span>{t}</span>
                      {onApplyTag && (
                        <button
                          onClick={() => handleRemoveTag(t)}
                          title={`Remove tag "${t}"`}
                          className="text-indigo-400 hover:text-red-300 opacity-60 group-hover:opacity-100 transition-opacity"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      )}
                    </div>
                  ))
                ) : (
                  <div className="text-xs text-slate-500 italic py-1">No tags assigned.</div>
                )}
              </div>

              {/* Add Tag Input Form */}
              {onApplyTag && (
                <div className="flex gap-1.5 pt-1">
                  <input
                    type="text"
                    value={newTag}
                    onChange={(e) => setNewTag(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && handleAddTag()}
                    placeholder="Add tag to sample…"
                    className="min-w-0 flex-1 rounded-lg border border-slate-800 bg-slate-950/80 px-2.5 py-1.5 text-xs text-slate-200 placeholder:text-slate-500 focus:border-indigo-500 focus:outline-none transition-all"
                  />
                  <button
                    onClick={handleAddTag}
                    disabled={!newTag.trim()}
                    className="flex items-center gap-1 rounded-lg bg-indigo-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-indigo-500 disabled:opacity-40 disabled:pointer-events-none transition-all"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    <span>Add</span>
                  </button>
                </div>
              )}
            </div>

            {/* Labels & Detections Section */}
            {sample?.labels && Object.keys(sample.labels).length > 0 && (
              <div className="rounded-xl border border-slate-800/80 bg-slate-900/40 p-3 space-y-2.5">
                <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-slate-400">
                  <Layers className="h-3.5 w-3.5 text-slate-400" />
                  <span>Detected Objects</span>
                </div>

                <div className="space-y-1.5 max-h-48 overflow-y-auto pr-1">
                  {Object.entries(sample.labels).map(([field, values]) => (
                    <div key={field} className="space-y-1">
                      <div className="text-[10px] font-semibold text-slate-500 font-mono uppercase">{field}</div>
                      {values.map((v, idx) => {
                        const labelText = typeof v.label === "string" ? v.label : "object";
                        const color = colorForLabel(field + labelText);
                        const conf = typeof v.confidence === "number" ? (v.confidence * 100).toFixed(0) + "%" : "";

                        return (
                          <div
                            key={idx}
                            className="flex items-center justify-between rounded-lg border border-slate-800 bg-slate-950/60 px-2.5 py-1.5 text-xs"
                          >
                            <div className="flex items-center gap-2">
                              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: color }} />
                              <span className="font-medium text-slate-200">{labelText}</span>
                            </div>
                            {conf && <span className="font-mono text-[10px] text-slate-400">{conf}</span>}
                          </div>
                        );
                      })}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
