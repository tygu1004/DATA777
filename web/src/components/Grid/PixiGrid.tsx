import { useVirtualizer } from "@tanstack/react-virtual";
import { Check, FolderSearch, ImageIcon, Maximize2, Sparkles, Square } from "lucide-react";
import { Application, Sprite, Texture } from "pixi.js";
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { thumbnailUrl } from "../../api/client";
import { CHUNK_SIZE, useSampleChunks } from "../../hooks/useSamples";
import type { Filter, Sample } from "../../types";
import type { GridSize } from "../Toolbar";
import { getTexture } from "./textureCache";

const GRID_DIMENSIONS: Record<GridSize, { cellSize: number; cellGap: number; thumbSize: number }> = {
  small: { cellSize: 120, cellGap: 6, thumbSize: 108 },
  medium: { cellSize: 164, cellGap: 8, thumbSize: 148 },
  large: { cellSize: 236, cellGap: 10, thumbSize: 216 },
};

function colorForLabel(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) | 0;
  return `hsl(${Math.abs(hash) % 360}, 85%, 65%)`;
}

interface Props {
  filter: Filter | undefined;
  count: number;
  selected: Set<number>;
  allMatching: boolean;
  onSelect: (id: number, index: number, shiftKey: boolean) => void;
  onOpenPreview: (index: number) => void;
  onChunksUpdate: (chunks: Map<number, Sample[]>) => void;
  canFindSimilar: boolean;
  onFindSimilar: (id: number) => void;
  gridSize?: GridSize;
  showLabels?: boolean;
}

export default function PixiGrid({
  filter,
  count,
  selected,
  allMatching,
  onSelect,
  onOpenPreview,
  onChunksUpdate,
  canFindSimilar,
  onFindSimilar,
  gridSize = "medium",
  showLabels = true,
}: Props) {
  const { cellSize, cellGap, thumbSize } = GRID_DIMENSIONS[gridSize];

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
      setColumns(Math.max(1, Math.floor(entry.contentRect.width / cellSize)));
      setViewportHeight(entry.contentRect.height);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [cellSize]);

  const rowCount = Math.max(1, Math.ceil(count / columns));

  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => cellSize,
    overscan: 3,
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

  useEffect(() => {
    if (chunks.size > 0) onChunksUpdate(chunks);
  }, [chunks, onChunksUpdate]);

  // Draw sprites for visible cells
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
        if (y < -cellSize || y > viewportHeight + cellSize) continue;

        const sample = getSample(index);
        seen.add(index);

        let sprite = pool.get(index);
        if (!sprite) {
          sprite = new Sprite(Texture.EMPTY);
          sprite.width = thumbSize;
          sprite.height = thumbSize;
          app.stage.addChild(sprite);
          pool.set(index, sprite);
        }
        sprite.x = col * cellSize + cellGap / 2;
        sprite.y = y + cellGap / 2;
        sprite.width = thumbSize;
        sprite.height = thumbSize;

        if (sample && sprite.texture === Texture.EMPTY) {
          const url = thumbnailUrl(sample.id);
          getTexture(url).then((tex) => {
            if (pool.get(index) === sprite) {
              sprite!.texture = tex;
              sprite!.width = thumbSize;
              sprite!.height = thumbSize;
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
    <div ref={outerRef} className="relative flex-1 overflow-hidden bg-[#090a0f] select-none">
      <canvas ref={canvasRef} className="pointer-events-none absolute inset-0" />

      {/* Empty States */}
      {count === 0 && (
        <div className="absolute inset-0 flex flex-col items-center justify-center p-6 text-center z-10">
          {filter && filter.stages && filter.stages.length > 0 ? (
            <div className="flex max-w-sm flex-col items-center gap-3 rounded-2xl border border-slate-800 bg-slate-900/60 p-8 backdrop-blur-md shadow-xl">
              <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
                <FolderSearch className="h-6 w-6" />
              </div>
              <h3 className="text-base font-semibold text-slate-200">No matching samples</h3>
              <p className="text-xs text-slate-400">
                No samples in the dataset match the active filter criteria. Try resetting or adjusting filters.
              </p>
            </div>
          ) : (
            <div className="flex max-w-md flex-col items-center gap-3 rounded-2xl border border-slate-800 bg-slate-900/60 p-8 backdrop-blur-md shadow-xl">
              <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-tr from-indigo-500/20 to-cyan-500/20 border border-indigo-500/30 text-indigo-300">
                <ImageIcon className="h-6 w-6" />
              </div>
              <h3 className="text-base font-semibold text-slate-200">Dataset is empty</h3>
              <p className="text-xs text-slate-400 leading-relaxed">
                Enter a local directory path (e.g. <code className="font-mono text-indigo-300">./devdata</code>) or S3
                path in the header bar and click <strong className="text-slate-300">Index Dataset</strong> to begin
                exploring.
              </p>
            </div>
          )}
        </div>
      )}

      {/* Virtual Scroll Container */}
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
                  onClick={(e) => {
                    if (e.shiftKey && sample) {
                      onSelect(sample.id, index, true);
                    } else {
                      onOpenPreview(index);
                    }
                  }}
                  onDoubleClick={() => sample && onOpenPreview(index)}
                  title={sample ? `${sample.filename} (${sample.width}×${sample.height}) — Click to view details` : undefined}
                  className={`group absolute box-border cursor-pointer overflow-hidden rounded-xl transition-all duration-150 ${
                    isSelected
                      ? "ring-2 ring-indigo-500 ring-offset-2 ring-offset-[#090a0f] shadow-lg shadow-indigo-500/20 z-10"
                      : "border border-slate-800/80 hover:border-slate-600/80 hover:shadow-md hover:shadow-black/40"
                  }`}
                  style={{
                    top: vRow.start + cellGap / 2,
                    left: col * cellSize + cellGap / 2,
                    width: thumbSize,
                    height: thumbSize,
                    backgroundColor: "rgba(18, 22, 34, 0.4)",
                  }}
                >
                  {/* Bounding Box / Keypoint Label Overlays */}
                  {showLabels && sample?.labels && (
                    <div className="pointer-events-none absolute inset-0 overflow-hidden z-10">
                      {Object.entries(sample.labels).flatMap(([fieldName, values]) =>
                        values.map((lv, i) => {
                          const label = typeof lv.label === "string" ? lv.label : "";
                          const color = colorForLabel(fieldName + label);
                          const bbox = Array.isArray(lv.bbox) ? (lv.bbox as number[]) : undefined;
                          const points = Array.isArray(lv.points) ? (lv.points as number[][]) : undefined;
                          return (
                            <Fragment key={`${fieldName}-${i}`}>
                              {bbox && (
                                <div
                                  className="absolute border-2 transition-opacity"
                                  style={{
                                    left: `${bbox[0] * 100}%`,
                                    top: `${bbox[1] * 100}%`,
                                    width: `${bbox[2] * 100}%`,
                                    height: `${bbox[3] * 100}%`,
                                    borderColor: color,
                                    backgroundColor: `${color}15`,
                                  }}
                                >
                                  {label && gridSize !== "small" && (
                                    <span
                                      className="absolute -top-4 left-0 rounded px-1 text-[9px] font-semibold text-black shadow-sm truncate max-w-[80px]"
                                      style={{ backgroundColor: color }}
                                    >
                                      {label}
                                    </span>
                                  )}
                                </div>
                              )}
                              {points?.map((pt, pi) => (
                                <div
                                  key={pi}
                                  className="absolute h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full border border-black/40 shadow-sm"
                                  style={{ left: `${pt[0] * 100}%`, top: `${pt[1] * 100}%`, backgroundColor: color }}
                                />
                              ))}
                            </Fragment>
                          );
                        }),
                      )}
                    </div>
                  )}

                  {/* Top-Left Selection Checkbox Button */}
                  <div
                    onClick={(e) => {
                      e.stopPropagation();
                      if (sample) onSelect(sample.id, index, e.shiftKey);
                    }}
                    className={`absolute left-1.5 top-1.5 z-20 flex h-6 w-6 items-center justify-center rounded-md backdrop-blur-md transition-all ${
                      isSelected
                        ? "bg-indigo-600 text-white opacity-100 shadow-md shadow-indigo-600/40"
                        : "bg-black/50 text-slate-300 opacity-0 group-hover:opacity-100 hover:bg-black/80 hover:text-white"
                    }`}
                  >
                    {isSelected ? <Check className="h-3.5 w-3.5 stroke-[3]" /> : <Square className="h-3.5 w-3.5" />}
                  </div>

                  {/* Top-Right Quick Action Buttons (Preview, Find Similar) */}
                  <div className="absolute right-1.5 top-1.5 z-20 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    {canFindSimilar && sample && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          onFindSimilar(sample.id);
                        }}
                        title="Find similar samples (Vector Search)"
                        className="flex h-6 w-6 items-center justify-center rounded-md bg-black/60 backdrop-blur-md text-purple-300 hover:bg-purple-600 hover:text-white transition-all shadow-sm"
                      >
                        <Sparkles className="h-3 w-3" />
                      </button>
                    )}

                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onOpenPreview(index);
                      }}
                      title="Open full preview"
                      className="flex h-6 w-6 items-center justify-center rounded-md bg-black/60 backdrop-blur-md text-slate-300 hover:bg-indigo-600 hover:text-white transition-all shadow-sm"
                    >
                      <Maximize2 className="h-3 w-3" />
                    </button>
                  </div>

                  {/* Bottom Metadata & Tags Overlay */}
                  {sample && (
                    <div className="absolute inset-x-0 bottom-0 z-20 flex flex-col justify-end bg-gradient-to-t from-black/90 via-black/50 to-transparent p-1.5 pt-4 text-[10px] text-slate-200 opacity-90 group-hover:opacity-100 transition-opacity">
                      <div className="flex items-center justify-between gap-1">
                        <span className="truncate font-medium text-slate-100">{sample.filename}</span>
                        {gridSize !== "small" && (
                          <span className="shrink-0 font-mono text-[9px] text-slate-400">
                            {sample.width}×{sample.height}
                          </span>
                        )}
                      </div>

                      {sample.tags.length > 0 && (
                        <div className="mt-0.5 flex flex-wrap gap-1">
                          {sample.tags.slice(0, gridSize === "small" ? 1 : 3).map((t) => (
                            <span
                              key={t}
                              className="rounded bg-indigo-500/30 px-1 py-0.2 text-[9px] font-medium text-indigo-200 border border-indigo-500/40 truncate max-w-[80px]"
                            >
                              {t}
                            </span>
                          ))}
                          {sample.tags.length > (gridSize === "small" ? 1 : 3) && (
                            <span className="text-[9px] text-slate-400 font-mono">
                              +{sample.tags.length - (gridSize === "small" ? 1 : 3)}
                            </span>
                          )}
                        </div>
                      )}
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

