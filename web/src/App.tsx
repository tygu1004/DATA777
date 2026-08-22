import { useCallback, useMemo, useState } from "react";
import * as api from "./api/client";
import CommitHistory from "./components/CommitHistory";
import ExportModal from "./components/ExportModal";
import FilterSidebar from "./components/FilterSidebar";
import PixiGrid from "./components/Grid/PixiGrid";
import Header from "./components/Header";
import SampleModal from "./components/SampleModal";
import Toolbar, { type GridSize } from "./components/Toolbar";
import {
  CHUNK_SIZE,
  useCommits,
  useInvalidateAfterMutation,
  useSampleCount,
  useSchema,
  useTagCounts,
} from "./hooks/useSamples";
import { useSelection } from "./hooks/useSelection";
import type { Filter, Job, Predicate, Sample, Stage } from "./types";

export default function App() {
  const [activeTags, setActiveTags] = useState<string[]>([]);
  const [predicates, setPredicates] = useState<Predicate[]>([]);
  const [nearSample, setNearSample] = useState<number | null>(null);

  // UI layout and view options
  const [showSidebar, setShowSidebar] = useState(true);
  const [showHistory, setShowHistory] = useState(true);
  const [gridSize, setGridSize] = useState<GridSize>("medium");
  const [showLabels, setShowLabels] = useState(true);
  const [showExport, setShowExport] = useState(false);

  const { data: fields } = useSchema();
  const { data: tags } = useTagCounts();
  const embeddingField = useMemo(() => fields?.find((f) => f.kind === "embedding")?.name, [fields]);

  const filter = useMemo<Filter | undefined>(() => {
    const stages: Stage[] = [];
    const match: Predicate[] = [];
    if (activeTags.length > 0) match.push({ field: "tags", op: "all", value: activeTags });
    match.push(...predicates);
    if (match.length > 0) stages.push({ type: "match", match });
    if (nearSample != null && embeddingField) {
      stages.push({ type: "sort", sort: { near: { field: embeddingField, sample_id: nearSample } } });
    }
    if (stages.length === 0) return undefined;
    return { stages };
  }, [activeTags, predicates, nearSample, embeddingField]);

  const { data: count = 0 } = useSampleCount(filter);
  const { data: totalCount = 0 } = useSampleCount(undefined);
  const { data: commits = [] } = useCommits();
  const invalidate = useInvalidateAfterMutation();

  const { selected, allMatching, toggle, selectRange, selectAllMatching, clear } = useSelection();
  const [chunks, setChunks] = useState<Map<number, Sample[]>>(new Map());
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [folderPath, setFolderPath] = useState("./devdata");
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
          message: `${op === "add" ? "Add tag" : "Remove tag"} "${tag}"`,
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

  const handleSingleSampleTag = useCallback(
    async (sampleId: number, tag: string, op: "add" | "remove") => {
      setBusy(true);
      try {
        const { job_id } = await api.createSetCommit({
          message: `${op === "add" ? "Add tag" : "Remove tag"} "${tag}" on sample #${sampleId}`,
          kind: "set",
          field: "tags",
          selection: { mode: "explicit", ids: [sampleId] },
          op,
          value: tag,
        });
        await api.pollJob(job_id);
        invalidate();
      } finally {
        setBusy(false);
      }
    },
    [invalidate],
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

  const currentPreviewSample = useMemo(() => {
    if (previewIndex === null) return undefined;
    const chunkOffset = Math.floor(previewIndex / CHUNK_SIZE) * CHUNK_SIZE;
    return chunks.get(chunkOffset)?.[previewIndex - chunkOffset];
  }, [chunks, previewIndex]);

  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-[#090a0f] text-slate-100 font-sans select-none">
      {/* Global Brand Header & Dataset Indexing Bar */}
      <Header
        folderPath={folderPath}
        onFolderPathChange={setFolderPath}
        onStartIndex={startIndex}
        indexJob={indexJob}
        totalCount={totalCount}
        filteredCount={count}
        hasFilter={filter !== undefined}
        selectedCount={selected.size}
        showSidebar={showSidebar}
        onToggleSidebar={() => setShowSidebar((prev) => !prev)}
        showHistory={showHistory}
        onToggleHistory={() => setShowHistory((prev) => !prev)}
        commitsCount={commits.length}
      />

      {/* Action Toolbar & Grid Controls */}
      <Toolbar
        selectedCount={selected.size}
        allMatching={allMatching}
        matchingCount={count}
        busy={busy}
        onApplyTag={(tag) => applyTag(tag, "add")}
        onRemoveTag={(tag) => applyTag(tag, "remove")}
        onSelectAllMatching={selectAllMatching}
        onClearSelection={clear}
        gridSize={gridSize}
        onGridSizeChange={setGridSize}
        showLabels={showLabels}
        onToggleShowLabels={() => setShowLabels((prev) => !prev)}
        popularTags={tags}
        onOpenExport={() => setShowExport(true)}
      />

      {/* Main Content Area */}
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Left Filter Facets Sidebar */}
        {showSidebar && (
          <FilterSidebar
            activeTags={activeTags}
            onToggleTag={toggleTag}
            onClear={() => setActiveTags([])}
            predicates={predicates}
            onAddPredicate={(p) => setPredicates((prev) => [...prev, p])}
            onRemovePredicate={(i) => setPredicates((prev) => prev.filter((_, idx) => idx !== i))}
            onClearPredicates={() => setPredicates([])}
            nearSample={nearSample}
            onClearNear={() => setNearSample(null)}
          />
        )}

        {/* Center Pixi.js WebGPU Virtual Grid */}
        <PixiGrid
          filter={filter}
          count={count}
          selected={selected}
          allMatching={allMatching}
          onSelect={handleSelect}
          onOpenPreview={setPreviewIndex}
          onChunksUpdate={setChunks}
          canFindSimilar={!!embeddingField}
          onFindSimilar={setNearSample}
          gridSize={gridSize}
          showLabels={showLabels}
        />

        {/* Right Commit History & Git Timeline */}
        {showHistory && (
          <CommitHistory commits={commits} onUndo={handleUndo} undoing={undoing} />
        )}
      </div>

      {/* Sample Detail Inspector Modal */}
      {previewIndex !== null && (
        <SampleModal
          chunks={chunks}
          count={count}
          index={previewIndex}
          onClose={() => setPreviewIndex(null)}
          onNavigate={setPreviewIndex}
          selected={selected}
          onToggleSelect={(id, idx) => handleSelect(id, idx, false)}
          canFindSimilar={!!embeddingField}
          onFindSimilar={setNearSample}
          onApplyTag={(tag, op) => {
            if (currentPreviewSample) {
              handleSingleSampleTag(currentPreviewSample.id, tag, op);
            }
          }}
        />
      )}

      {/* Export Dataset Modal */}
      <ExportModal
        isOpen={showExport}
        onClose={() => setShowExport(false)}
        selectedIds={selected}
        totalMatchingCount={count}
        chunks={chunks}
      />
    </div>
  );
}

