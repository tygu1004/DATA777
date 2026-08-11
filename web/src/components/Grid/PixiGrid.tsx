import { useVirtualizer } from "@tanstack/react-virtual";
import { Application, Sprite, Texture } from "pixi.js";
import { useEffect, useMemo, useRef, useState } from "react";
import { thumbnailUrl } from "../../api/client";
import { CHUNK_SIZE, useSampleChunks } from "../../hooks/useSamples";
import type { Filter, Sample } from "../../types";
import { getTexture } from "./textureCache";

const CELL_SIZE = 152;
const CELL_GAP = 8;
const THUMB = 136;

interface Props {
  filter: Filter | undefined;
  count: number;
  selected: Set<number>;
  allMatching: boolean;
  onSelect: (id: number, index: number, shiftKey: boolean) => void;
  onOpenPreview: (index: number) => void;
  onChunksUpdate: (chunks: Map<number, Sample[]>) => void;
}

export default function PixiGrid({ filter, count, selected, allMatching, onSelect, onOpenPreview, onChunksUpdate }: Props) {
  const outerRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const appRef = useRef<Application | null>(null);
  const spritePoolRef = useRef<Map<number, Sprite>>(new Map());

  const [columns, setColumns] = useState(1);
  const [viewportHeight, setViewportHeight] = useState(0);
  const [, forceTick] = useState(0);

  useEffect(() => {
    const el = outerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      setColumns(Math.max(1, Math.floor(entry.contentRect.width / CELL_SIZE)));
      setViewportHeight(entry.contentRect.height);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const rowCount = Math.max(1, Math.ceil(count / columns));

  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => CELL_SIZE,
    overscan: 4,
  });

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    let raf = 0;
    const onScroll = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => forceTick((t) => t + 1));
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      el.removeEventListener("scroll", onScroll);
      cancelAnimationFrame(raf);
    };
  }, []);

  useEffect(() => {
    if (!canvasRef.current || !outerRef.current) return;
    let disposed = false;
    const app = new Application();
    app
      .init({
        canvas: canvasRef.current,
        backgroundAlpha: 0,
        antialias: true,
        resizeTo: outerRef.current,
        preference: "webgpu",
      })
      .then(() => {
        if (disposed) {
          app.destroy(true, { children: true });
          return;
        }
        appRef.current = app;
        forceTick((t) => t + 1);
      });
    return () => {
      disposed = true;
      if (appRef.current) {
        appRef.current.destroy(true, { children: true });
        appRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const virtualItems = virtualizer.getVirtualItems();

  const neededOffsets = useMemo(() => {
    if (virtualItems.length === 0) return [];
    const firstIndex = virtualItems[0].index * columns;
    const lastIndex = Math.min(count - 1, (virtualItems[virtualItems.length - 1].index + 1) * columns - 1);
    const startChunk = Math.floor(firstIndex / CHUNK_SIZE) * CHUNK_SIZE;
    const endChunk = Math.floor(lastIndex / CHUNK_SIZE) * CHUNK_SIZE;
    const offsets: number[] = [];
    for (let o = startChunk; o <= endChunk; o += CHUNK_SIZE) offsets.push(o);
    return offsets;
  }, [virtualItems, columns, count]);

  const chunks = useSampleChunks(filter, neededOffsets);

  const getSample = useMemo(() => {
    return (index: number): Sample | undefined => {
      const chunkOffset = Math.floor(index / CHUNK_SIZE) * CHUNK_SIZE;
      const chunk = chunks.get(chunkOffset);
      return chunk?.[index - chunkOffset];
    };
  }, [chunks]);

  // Report currently loaded chunks upward so the lightbox and range-selection can resolve ids
  // near where the user is looking, without App ever holding the whole dataset in memory.
  useEffect(() => {
    if (chunks.size > 0) onChunksUpdate(chunks);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chunks]);

  // Draw sprites for visible cells. Runs on every render (scroll tick, data arriving, resize).
  useEffect(() => {
    const app = appRef.current;
    if (!app) return;

    const scrollTop = scrollRef.current?.scrollTop ?? 0;
    const pool = spritePoolRef.current;
    const seen = new Set<number>();

    for (const vRow of virtualItems) {
      for (let col = 0; col < columns; col++) {
        const index = vRow.index * columns + col;
        if (index >= count) continue;
        const y = vRow.start - scrollTop;
        if (y < -CELL_SIZE || y > viewportHeight + CELL_SIZE) continue; // fully offscreen

        const sample = getSample(index);
        seen.add(index);

        let sprite = pool.get(index);
        if (!sprite) {
          sprite = new Sprite(Texture.EMPTY);
          sprite.width = THUMB;
          sprite.height = THUMB;
          app.stage.addChild(sprite);
          pool.set(index, sprite);
        }
        sprite.x = col * CELL_SIZE + CELL_GAP / 2;
        sprite.y = y + CELL_GAP / 2;

        if (sample && sprite.texture === Texture.EMPTY) {
          const url = thumbnailUrl(sample.id);
          getTexture(url).then((tex) => {
            if (pool.get(index) === sprite) {
              sprite!.texture = tex;
              sprite!.width = THUMB;
              sprite!.height = THUMB;
            }
          });
        }
      }
    }

    for (const [index, sprite] of pool) {
      if (!seen.has(index)) {
        app.stage.removeChild(sprite);
        sprite.destroy();
        pool.delete(index);
      }
    }
  });

  return (
    <div ref={outerRef} className="relative flex-1 overflow-hidden bg-neutral-950">
      <canvas ref={canvasRef} className="pointer-events-none absolute inset-0" />
      <div ref={scrollRef} className="absolute inset-0 overflow-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualItems.map((vRow) =>
            Array.from({ length: columns }, (_, col) => {
              const index = vRow.index * columns + col;
              if (index >= count) return null;
              const sample = getSample(index);
              const isSelected = allMatching || (sample ? selected.has(sample.id) : false);
              return (
                <div
                  key={index}
                  onClick={(e) => sample && onSelect(sample.id, index, e.shiftKey)}
                  onDoubleClick={() => sample && onOpenPreview(index)}
                  title={sample?.filename}
                  className="absolute box-border cursor-pointer select-none rounded"
                  style={{
                    top: vRow.start,
                    left: col * CELL_SIZE,
                    width: CELL_SIZE - CELL_GAP,
                    height: CELL_SIZE - CELL_GAP,
                    border: isSelected ? "3px solid #3b82f6" : "1px solid rgba(255,255,255,0.08)",
                  }}
                >
                  {sample && sample.tags.length > 0 && (
                    <div className="pointer-events-none absolute inset-x-0 bottom-0 truncate bg-black/60 px-1 py-0.5 text-[10px] text-white">
                      {sample.tags.join(", ")}
                    </div>
                  )}
                </div>
              );
            }),
          )}
        </div>
      </div>
    </div>
  );
}
