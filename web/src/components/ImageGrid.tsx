import { AutoSizer } from "react-virtualized-auto-sizer";
import { FixedSizeGrid } from "react-window";
import GridCell from "./GridCell";
import type { Sample } from "../types";

const CELL_SIZE = 160;

interface Props {
  samples: Sample[];
  selected: Set<number>;
  onSelect: (id: number, index: number, shiftKey: boolean) => void;
  onOpenPreview: (index: number) => void;
}

export default function ImageGrid({ samples, selected, onSelect, onOpenPreview }: Props) {
  return (
    <div style={{ flex: 1, minHeight: 0 }}>
      <AutoSizer
        renderProp={({ width, height }) => {
          if (!width || !height) return null;
          const columnCount = Math.max(1, Math.floor(width / CELL_SIZE));
          const rowCount = Math.ceil(samples.length / columnCount);
          return (
            <FixedSizeGrid
              columnCount={columnCount}
              rowCount={rowCount}
              columnWidth={CELL_SIZE}
              rowHeight={CELL_SIZE}
              width={width}
              height={height}
              itemData={{ samples, columnCount, selected, onSelect, onOpenPreview }}
            >
              {GridCell}
            </FixedSizeGrid>
          );
        }}
      />
    </div>
  );
}
