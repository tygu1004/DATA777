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
      <span>{selectedCount}개 선택됨</span>
      <input
        value={tag}
        onChange={(e) => setTag(e.target.value)}
        placeholder="태그 입력"
        onKeyDown={(e) => e.key === "Enter" && submit(onApplyTag)}
        style={{ padding: 4 }}
      />
      <button disabled={selectedCount === 0 || !tag.trim()} onClick={() => submit(onApplyTag)}>
        태그 추가
      </button>
      <button disabled={selectedCount === 0 || !tag.trim()} onClick={() => submit(onRemoveTag)}>
        태그 제거
      </button>
      <button disabled={selectedCount === 0} onClick={onClearSelection}>
        선택 해제
      </button>
    </div>
  );
}
