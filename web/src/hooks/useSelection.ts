import { useCallback, useRef, useState } from "react";

// Explicit ids for interactive clicking (small, dev-scale) plus a separate "every sample
// matching the current filter" flag — selecting all never explodes into an id list
// (api.md#selection: "Its size does not depend on the match count").
export function useSelection() {
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [allMatching, setAllMatching] = useState(false);
  const anchorIndexRef = useRef<number | null>(null);

  const toggle = useCallback((id: number, index: number) => {
    setAllMatching(false);
    anchorIndexRef.current = index;
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectRange = useCallback((orderedIds: number[], index: number) => {
    setAllMatching(false);
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

  const selectAllMatching = useCallback(() => {
    setSelected(new Set());
    setAllMatching(true);
  }, []);

  const clear = useCallback(() => {
    setSelected(new Set());
    setAllMatching(false);
    anchorIndexRef.current = null;
  }, []);

  return { selected, allMatching, toggle, selectRange, selectAllMatching, clear };
}
