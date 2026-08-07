import { useEffect, useRef, useState } from "react";
import { FixedSizeGrid } from "react-window";
import GridCell from "./GridCell";
import type { Sample } from "../types";

const CELL_SIZE = 160;

interface Props {
  samples: Sample[];
  selected: Set<number>;
  onSelect: (id: number, index: number, shiftKey: boolean) => void;
}

export default function ImageGrid({ samples, selected, onSelect }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setSize({ width: entry.contentRect.width, height: entry.contentRect.height });
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const columnCount = Math.max(1, Math.floor(size.width / CELL_SIZE));
  const rowCount = Math.ceil(samples.length / columnCount);

  return (
    <div ref={containerRef} style={{ flex: 1, minHeight: 0 }}>
      {size.width > 0 && size.height > 0 && (
        <FixedSizeGrid
          columnCount={columnCount}
          rowCount={rowCount}
          columnWidth={CELL_SIZE}
          rowHeight={CELL_SIZE}
          width={size.width}
          height={size.height}
          itemData={{ samples, columnCount, selected, onSelect }}
        >
          {GridCell}
        </FixedSizeGrid>
      )}
    </div>
  );
}
