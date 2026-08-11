import { useState } from "react";

interface Props {
  selectedCount: number;
  allMatching: boolean;
  matchingCount: number;
  busy: boolean;
  onApplyTag: (tag: string) => void;
  onRemoveTag: (tag: string) => void;
  onSelectAllMatching: () => void;
  onClearSelection: () => void;
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
}: Props) {
  const [tag, setTag] = useState("");
  const hasSelection = allMatching || selectedCount > 0;

  const submit = (apply: (tag: string) => void) => {
    const trimmed = tag.trim();
    if (!trimmed || !hasSelection) return;
    apply(trimmed);
    setTag("");
  };

  return (
    <div className="flex items-center gap-2 border-b border-white/10 px-3 py-2">
      <span className="text-sm text-neutral-300">
        {allMatching ? `all ${matchingCount} matching selected` : `${selectedCount} selected`}
      </span>
      <span className="text-xs text-neutral-500">(shift-click for a range)</span>
      <button
        disabled={matchingCount === 0 || allMatching}
        onClick={onSelectAllMatching}
        className="rounded border border-white/10 px-2 py-1 text-xs text-neutral-200 hover:bg-white/5 disabled:opacity-40"
      >
        Select all matching ({matchingCount})
      </button>
      <input
        value={tag}
        onChange={(e) => setTag(e.target.value)}
        placeholder="Enter a tag"
        onKeyDown={(e) => e.key === "Enter" && submit(onApplyTag)}
        className="rounded border border-white/10 bg-white/5 px-2 py-1 text-sm text-neutral-100 placeholder:text-neutral-500"
      />
      <button
        disabled={!hasSelection || !tag.trim() || busy}
        onClick={() => submit(onApplyTag)}
        className="rounded bg-blue-600 px-2 py-1 text-xs text-white disabled:opacity-40"
      >
        Add tag
      </button>
      <button
        disabled={!hasSelection || !tag.trim() || busy}
        onClick={() => submit(onRemoveTag)}
        className="rounded border border-white/10 px-2 py-1 text-xs text-neutral-200 hover:bg-white/5 disabled:opacity-40"
      >
        Remove tag
      </button>
      <button
        disabled={!hasSelection}
        onClick={onClearSelection}
        className="rounded border border-white/10 px-2 py-1 text-xs text-neutral-200 hover:bg-white/5 disabled:opacity-40"
      >
        Clear selection
      </button>
      {busy && <span className="text-xs text-blue-400">working…</span>}
    </div>
  );
}
