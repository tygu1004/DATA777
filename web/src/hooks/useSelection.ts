import { useCallback, useRef, useState } from "react";

export function useSelection() {
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const anchorIndexRef = useRef<number | null>(null);

  const toggle = useCallback((id: number, index: number) => {
    anchorIndexRef.current = index;
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectRange = useCallback((orderedIds: number[], index: number) => {
    const anchor = anchorIndexRef.current ?? index;
    const [start, end] = anchor <= index ? [anchor, index] : [index, anchor];
    setSelected((prev) => {
      const next = new Set(prev);
      for (let i = start; i <= end; i++) {
        if (orderedIds[i] !== undefined) next.add(orderedIds[i]);
      }
      return next;
    });
  }, []);

  const selectAll = useCallback((orderedIds: number[]) => {
    setSelected(new Set(orderedIds));
  }, []);

  const clear = useCallback(() => {
    setSelected(new Set());
    anchorIndexRef.current = null;
  }, []);

  return { selected, toggle, selectRange, selectAll, clear };
}
