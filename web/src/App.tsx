import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as api from "./api/client";
import CommitHistory from "./components/CommitHistory";
import ImageGrid from "./components/ImageGrid";
import Toolbar from "./components/Toolbar";
import { useSamples } from "./hooks/useSamples";
import { useSelection } from "./hooks/useSelection";
import type { Commit, IndexStatus, TagOp } from "./types";

export default function App() {
  const { samples, loading, reload, applyTagsLocally } = useSamples();
  const { selected, toggle, selectRange, selectAll, clear } = useSelection();
  const sampleIds = useMemo(() => samples.map((s) => s.id), [samples]);

  const handleSelect = useCallback(
    (id: number, index: number, shiftKey: boolean) => {
      if (shiftKey) selectRange(sampleIds, index);
      else toggle(id, index);
    },
    [sampleIds, selectRange, toggle],
  );

  const [folderPath, setFolderPath] = useState("");
  const [indexStatus, setIndexStatus] = useState<IndexStatus | null>(null);
  const [commits, setCommits] = useState<Commit[]>([]);
  const pollRef = useRef<number | null>(null);

  const reloadCommits = useCallback(async () => {
    const res = await api.listCommits();
    setCommits(res.items);
  }, []);

  useEffect(() => {
    reloadCommits();
  }, [reloadCommits]);

  const startIndex = useCallback(async () => {
    if (!folderPath.trim()) return;
    await api.startIndex(folderPath.trim());
    if (pollRef.current) window.clearInterval(pollRef.current);
    pollRef.current = window.setInterval(async () => {
      const status = await api.getIndexStatus();
      setIndexStatus(status);
      if (status.status === "done" || status.status === "error") {
        if (pollRef.current) window.clearInterval(pollRef.current);
        if (status.status === "done") reload();
      }
    }, 500);
  }, [folderPath, reload]);

  const applyTag = useCallback(
    async (tag: string, op: "add" | "remove") => {
      const ops: TagOp[] = Array.from(selected).map((sample_id) => ({ sample_id, tag, op }));
      if (ops.length === 0) return;
      await api.createCommit(`${op} "${tag}" on ${ops.length} sample(s)`, ops);
      applyTagsLocally(ops);
      await reloadCommits();
    },
    [selected, applyTagsLocally, reloadCommits],
  );

  const handleUndo = useCallback(async () => {
    await api.undo();
    await Promise.all([reload(), reloadCommits()]);
  }, [reload, reloadCommits]);

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 12px", borderBottom: "1px solid #ddd" }}>
        <strong>data777</strong>
        <input
          value={folderPath}
          onChange={(e) => setFolderPath(e.target.value)}
          placeholder="/absolute/path/to/image/folder"
          style={{ flex: 1, padding: 4 }}
        />
        <button onClick={startIndex}>Start indexing</button>
        <span style={{ fontSize: 13, color: "#666" }}>
          {indexStatus ? `${indexStatus.status} (${indexStatus.processed} indexed)` : ""}
          {loading ? " · loading samples..." : ""}
          {` · ${samples.length} total`}
        </span>
      </div>

      <Toolbar
        selectedCount={selected.size}
        totalCount={samples.length}
        onApplyTag={(tag) => applyTag(tag, "add")}
        onRemoveTag={(tag) => applyTag(tag, "remove")}
        onSelectAll={() => selectAll(sampleIds)}
        onClearSelection={clear}
      />

      <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
        <ImageGrid samples={samples} selected={selected} onSelect={handleSelect} />
        <CommitHistory commits={commits} onUndo={handleUndo} />
      </div>
    </div>
  );
}
