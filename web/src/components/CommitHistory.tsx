import type { Commit } from "../types";

interface Props {
  commits: Commit[];
  onUndo: () => void;
}

export default function CommitHistory({ commits, onUndo }: Props) {
  const head = commits.find((c) => c.is_head);

  return (
    <div style={{ width: 260, borderLeft: "1px solid #ddd", padding: 12, overflowY: "auto" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
        <strong>커밋 히스토리</strong>
        <button disabled={!head} onClick={onUndo}>
          Undo
        </button>
      </div>
      {commits.length === 0 && <p style={{ color: "#888", fontSize: 13 }}>아직 커밋이 없습니다.</p>}
      <ul style={{ listStyle: "none", padding: 0, margin: 0, fontSize: 13 }}>
        {commits.map((c) => (
          <li
            key={c.id}
            style={{
              padding: "6px 8px",
              marginBottom: 4,
              borderRadius: 4,
              background: c.is_head ? "#dbeafe" : "transparent",
            }}
          >
            <div>
              #{c.id} {c.is_head && <strong>(HEAD)</strong>}
            </div>
            <div style={{ color: "#555" }}>{c.message || "(no message)"}</div>
            <div style={{ color: "#999" }}>
              {c.op_count} ops · {new Date(c.created_at).toLocaleTimeString()}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
