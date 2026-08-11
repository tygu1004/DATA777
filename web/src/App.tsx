import { useCallback, useMemo, useState } from "react";
import * as api from "./api/client";
import CommitHistory from "./components/CommitHistory";
import FilterSidebar from "./components/FilterSidebar";
import PixiGrid from "./components/Grid/PixiGrid";
import Lightbox from "./components/Lightbox";
import Toolbar from "./components/Toolbar";
import { useCommits, useInvalidateAfterMutation, useSampleCount } from "./hooks/useSamples";
import { useSelection } from "./hooks/useSelection";
import type { Filter, Job, Sample } from "./types";

export default function App() {
  const [activeTags, setActiveTags] = useState<string[]>([]);
  const filter = useMemo<Filter | undefined>(() => {
    if (activeTags.length === 0) return undefined;
    return { stages: [{ type: "match", match: [{ field: "tags", op: "all", value: activeTags }] }] };
  }, [activeTags]);

  const { data: count = 0 } = useSampleCount(filter);
  const { data: commits = [] } = useCommits();
  const invalidate = useInvalidateAfterMutation();

  const { selected, allMatching, toggle, selectRange, selectAllMatching, clear } = useSelection();
  const [chunks, setChunks] = useState<Map<number, Sample[]>>(new Map());
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [folderPath, setFolderPath] = useState("");
  const [indexJob, setIndexJob] = useState<Job | null>(null);

  const orderedIds = useMemo(() => {
    const ids: number[] = [];
    for (const [offset, items] of chunks) items.forEach((s, i) => (ids[offset + i] = s.id));
    return ids;
  }, [chunks]);

  const handleSelect = useCallback(
    (id: number, index: number, shiftKey: boolean) => {
      if (shiftKey) selectRange(orderedIds, index);
      else toggle(id, index);
    },
    [orderedIds, selectRange, toggle],
  );

  const toggleTag = useCallback((tag: string) => {
    setActiveTags((prev) => (prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag]));
  }, []);

  const startIndex = useCallback(async () => {
    if (!folderPath.trim()) return;
    const { job_id } = await api.startIndex(folderPath.trim());
    const job = await api.pollJob(job_id, setIndexJob);
    setIndexJob(job);
    if (job.status === "succeeded") invalidate();
  }, [folderPath, invalidate]);

  const applyTag = useCallback(
    async (tag: string, op: "add" | "remove") => {
      const selection = allMatching
        ? { mode: "filter" as const, filter: filter ?? { stages: [] } }
        : { mode: "explicit" as const, ids: Array.from(selected) };
      if (selection.mode === "explicit" && selection.ids.length === 0) return;

      setBusy(true);
      try {
        const { job_id } = await api.createSetCommit({
          message: `${op} "${tag}"`,
          kind: "set",
          field: "tags",
          selection,
          op,
          value: tag,
        });
        await api.pollJob(job_id);
        invalidate();
        clear();
      } finally {
        setBusy(false);
      }
    },
    [allMatching, filter, selected, invalidate, clear],
  );

  const [undoing, setUndoing] = useState(false);
  const handleUndo = useCallback(async () => {
    const head = commits.find((c) => c.is_head);
    if (!head || undoing) return;
    setUndoing(true);
    try {
      const { job_id } = await api.undo(head.id);
      await api.pollJob(job_id);
      invalidate();
    } finally {
      setUndoing(false);
    }
  }, [commits, undoing, invalidate]);

  return (
    <div className="flex h-full flex-col bg-neutral-950 text-neutral-100">
      <div className="flex items-center gap-2 border-b border-white/10 px-3 py-2">
        <strong>data777</strong>
        <input
          value={folderPath}
          onChange={(e) => setFolderPath(e.target.value)}
          placeholder="/absolute/path/to/image/folder"
          className="flex-1 rounded border border-white/10 bg-white/5 px-2 py-1 text-sm placeholder:text-neutral-500"
        />
        <button onClick={startIndex} className="rounded bg-blue-600 px-2 py-1 text-xs text-white">
          Start indexing
        </button>
        <span className="text-xs text-neutral-400">
          {indexJob && indexJob.status !== "succeeded" && `${indexJob.status} (${indexJob.progress.processed} indexed)`}
          {` · ${count} sample(s)`}
        </span>
      </div>

      <Toolbar
        selectedCount={selected.size}
        allMatching={allMatching}
        matchingCount={count}
        busy={busy}
        onApplyTag={(tag) => applyTag(tag, "add")}
        onRemoveTag={(tag) => applyTag(tag, "remove")}
        onSelectAllMatching={selectAllMatching}
        onClearSelection={clear}
      />

      <div className="flex min-h-0 flex-1">
        <FilterSidebar activeTags={activeTags} onToggleTag={toggleTag} onClear={() => setActiveTags([])} />
        <PixiGrid
          filter={filter}
          count={count}
          selected={selected}
          allMatching={allMatching}
          onSelect={handleSelect}
          onOpenPreview={setPreviewIndex}
          onChunksUpdate={setChunks}
        />
        <CommitHistory commits={commits} onUndo={handleUndo} undoing={undoing} />
      </div>

      {previewIndex !== null && (
        <Lightbox chunks={chunks} count={count} index={previewIndex} onClose={() => setPreviewIndex(null)} onNavigate={setPreviewIndex} />
      )}
    </div>
  );
}
