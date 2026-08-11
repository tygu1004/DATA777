import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import * as api from "../api/client";
import type { Filter, Sample } from "../types";

export const CHUNK_SIZE = 200;

export function filterKey(filter?: Filter): string {
  return filter?.stages?.length ? JSON.stringify(filter) : "all";
}

export function useSampleCount(filter?: Filter) {
  return useQuery({
    queryKey: ["count", filterKey(filter)],
    queryFn: () => api.countSamples(filter),
    select: (d) => d.count,
  });
}

export function useTagCounts(filter?: Filter) {
  return useQuery({
    queryKey: ["tagCounts", filterKey(filter)],
    queryFn: () => api.getTagCounts(filter),
    select: (d) => d.items,
  });
}

export function useCommits() {
  return useQuery({
    queryKey: ["commits"],
    queryFn: () => api.listCommits(0, 50),
    select: (d) => d.items,
  });
}

export function useSchema() {
  return useQuery({
    queryKey: ["schema"],
    queryFn: () => api.getSchema(),
    select: (d) => d.fields,
    staleTime: Infinity,
  });
}

// useSampleChunks fetches only the chunk offsets a caller says it currently needs (typically
// derived from a virtualizer's visible range), and evicts the rest quickly via a short gcTime
// — "distant chunks must be evictable" (architecture.md#frontend).
export function useSampleChunks(filter: Filter | undefined, neededOffsets: number[]) {
  const key = filterKey(filter);
  const results = useQueries({
    queries: neededOffsets.map((offset) => ({
      queryKey: ["samples", key, offset],
      queryFn: () => api.listSamples(filter, offset, CHUNK_SIZE),
      staleTime: 30_000,
      gcTime: 30_000,
    })),
  });

  return useMemo(() => {
    const byOffset = new Map<number, Sample[]>();
    neededOffsets.forEach((offset, i) => {
      const data = results[i]?.data;
      if (data) byOffset.set(offset, data.items);
    });
    return byOffset;
  }, [results, neededOffsets]);
}

// invalidateSamples is called after a commit/undo job settles, so the grid, tag sidebar, and
// count all reflect the new HEAD.
export function useInvalidateAfterMutation() {
  const client = useQueryClient();
  return () => {
    client.invalidateQueries({ queryKey: ["samples"] });
    client.invalidateQueries({ queryKey: ["count"] });
    client.invalidateQueries({ queryKey: ["tagCounts"] });
    client.invalidateQueries({ queryKey: ["commits"] });
  };
}
