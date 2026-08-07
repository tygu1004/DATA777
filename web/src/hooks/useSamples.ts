import { useCallback, useEffect, useState } from "react";
import { listSamples } from "../api/client";
import type { Sample, TagOp } from "../types";

const PAGE_SIZE = 500;

export function useSamples() {
  const [samples, setSamples] = useState<Sample[]>([]);
  const [loading, setLoading] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const first = await listSamples(0, PAGE_SIZE);
      let items = first.items;

      const pageOffsets: number[] = [];
      for (let offset = items.length; offset < first.total; offset += PAGE_SIZE) {
        pageOffsets.push(offset);
      }
      const pages = await Promise.all(pageOffsets.map((offset) => listSamples(offset, PAGE_SIZE)));
      for (const page of pages) items = items.concat(page.items);

      setSamples(items);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  // Optimistically applies just-committed tag ops to local state so the grid updates
  // instantly instead of waiting on a full refetch.
  const applyTagsLocally = useCallback((ops: TagOp[]) => {
    setSamples((prev) =>
      prev.map((sample) => {
        const relevant = ops.filter((op) => op.sample_id === sample.id);
        if (relevant.length === 0) return sample;
        const tags = new Set(sample.tags);
        for (const op of relevant) {
          if (op.op === "add") tags.add(op.tag);
          else tags.delete(op.tag);
        }
        return { ...sample, tags: Array.from(tags) };
      }),
    );
  }, []);

  return { samples, loading, reload, applyTagsLocally };
}
