import type { CSSProperties } from "react";
import { thumbnailUrl } from "../api/client";
import type { Sample } from "../types";

export interface CellData {
  samples: Sample[];
  columnCount: number;
  selected: Set<number>;
  onSelect: (id: number, index: number, shiftKey: boolean) => void;
  onOpenPreview: (index: number) => void;
}

interface Props {
  columnIndex: number;
  rowIndex: number;
  style: CSSProperties;
  data: CellData;
}

const THUMB_SIZE = 144;

export default function GridCell({ columnIndex, rowIndex, style, data }: Props) {
  const { samples, columnCount, selected, onSelect, onOpenPreview } = data;
  const index = rowIndex * columnCount + columnIndex;
  const sample = samples[index];
  if (!sample) return <div style={style} />;

  const isSelected = selected.has(sample.id);

  return (
    <div style={{ ...style, padding: 8, boxSizing: "border-box" }}>
      <div
        onClick={(e) => onSelect(sample.id, index, e.shiftKey)}
        onDoubleClick={() => onOpenPreview(index)}
        title={`${sample.filename} (double-click to preview)`}
        style={{
          width: THUMB_SIZE,
          height: THUMB_SIZE,
          border: isSelected ? "3px solid #2563eb" : "1px solid #ddd",
          borderRadius: 4,
          overflow: "hidden",
          cursor: "pointer",
          position: "relative",
          userSelect: "none", // shift-click for range selection would otherwise also text-select the page
        }}
      >
        <img
          src={thumbnailUrl(sample.id)}
          loading="lazy"
          width={THUMB_SIZE}
          height={THUMB_SIZE}
          alt={sample.filename}
          style={{ objectFit: "cover", display: "block" }}
        />
        {sample.tags.length > 0 && (
          <div
            style={{
              position: "absolute",
              bottom: 0,
              left: 0,
              right: 0,
              background: "rgba(0,0,0,0.6)",
              color: "white",
              fontSize: 10,
              padding: "2px 4px",
              whiteSpace: "nowrap",
              overflow: "hidden",
              textOverflow: "ellipsis",
            }}
          >
            {sample.tags.join(", ")}
          </div>
        )}
      </div>
    </div>
  );
}
