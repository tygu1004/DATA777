import { useState } from "react";

interface Props {
  selectedCount: number;
  onApplyTag: (tag: string) => void;
  onRemoveTag: (tag: string) => void;
  onClearSelection: () => void;
}

export default function Toolbar({ selectedCount, onApplyTag, onRemoveTag, onClearSelection }: Props) {
  const [tag, setTag] = useState("");

  const submit = (apply: (tag: string) => void) => {
    const trimmed = tag.trim();
    if (!trimmed || selectedCount === 0) return;
    apply(trimmed);
    setTag("");
  };

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 12px", borderBottom: "1px solid #ddd" }}>
      <span>{selectedCount} selected</span>
      <input
        value={tag}
        onChange={(e) => setTag(e.target.value)}
        placeholder="Enter a tag"
        onKeyDown={(e) => e.key === "Enter" && submit(onApplyTag)}
        style={{ padding: 4 }}
      />
      <button disabled={selectedCount === 0 || !tag.trim()} onClick={() => submit(onApplyTag)}>
        Add tag
      </button>
      <button disabled={selectedCount === 0 || !tag.trim()} onClick={() => submit(onRemoveTag)}>
        Remove tag
      </button>
      <button disabled={selectedCount === 0} onClick={onClearSelection}>
        Clear selection
      </button>
    </div>
  );
}
