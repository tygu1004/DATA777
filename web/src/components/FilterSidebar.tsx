import {
  Check,
  Filter as FilterIcon,
  Plus,
  Search,
  SlidersHorizontal,
  Sparkles,
  Tag as TagIcon,
  X,
} from "lucide-react";
import { useMemo, useState } from "react";
import { useSchema, useTagCounts } from "../hooks/useSamples";
import type { Predicate } from "../types";

interface Props {
  activeTags: string[];
  onToggleTag: (tag: string) => void;
  onClear: () => void;
  predicates: Predicate[];
  onAddPredicate: (p: Predicate) => void;
  onRemovePredicate: (index: number) => void;
  onClearPredicates: () => void;
  nearSample: number | null;
  onClearNear: () => void;
}

const STRING_OPS = [
  { label: "equals (=)", value: "eq" },
  { label: "not equals (≠)", value: "neq" },
  { label: "contains", value: "contains" },
  { label: "in list", value: "in" },
  { label: "not in list", value: "not_in" },
] as const;

const NUMERIC_OPS = [
  { label: "equals (=)", value: "eq" },
  { label: "not equals (≠)", value: "neq" },
  { label: "less than (<)", value: "lt" },
  { label: "less or equal (≤)", value: "lte" },
  { label: "greater than (>)", value: "gt" },
  { label: "greater or equal (≥)", value: "gte" },
  { label: "in list", value: "in" },
  { label: "not in list", value: "not_in" },
] as const;

function opsFor(type?: string) {
  return type === "string" ? STRING_OPS : NUMERIC_OPS;
}

function parseValue(raw: string, type: string | undefined, op: string): unknown {
  const toTyped = (s: string) => (type === "string" ? s : Number(s));
  if (op === "in" || op === "not_in") {
    return raw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .map(toTyped);
  }
  return toTyped(raw.trim());
}

function formatPredicate(p: Predicate): string {
  const value = Array.isArray(p.value) ? p.value.join(", ") : String(p.value);
  const opMap: Record<string, string> = {
    eq: "=",
    neq: "≠",
    lt: "<",
    lte: "≤",
    gt: ">",
    gte: "≥",
    contains: "contains",
    in: "in",
    not_in: "not in",
  };
  return `${p.field} ${opMap[p.op] ?? p.op} ${value}`;
}

export default function FilterSidebar({
  activeTags,
  onToggleTag,
  onClear,
  predicates,
  onAddPredicate,
  onRemovePredicate,
  onClearPredicates,
  nearSample,
  onClearNear,
}: Props) {
  const { data: tags, isLoading } = useTagCounts();
  const { data: fields } = useSchema();
  const scalarFields = useMemo(() => fields?.filter((f) => f.kind === "scalar") ?? [], [fields]);

  const [field, setField] = useState("");
  const [op, setOp] = useState<string>("gte");
  const [value, setValue] = useState("");
  const [tagQuery, setTagQuery] = useState("");

  const activeField = scalarFields.find((f) => f.name === field) ?? scalarFields[0];
  const ops = opsFor(activeField?.type);

  const addPredicate = () => {
    if (!activeField || !value.trim()) return;
    onAddPredicate({ field: activeField.name, op, value: parseValue(value, activeField.type, op) });
    setValue("");
  };

  const hasAnyFilter = activeTags.length > 0 || predicates.length > 0 || nearSample != null;

  const filteredTags = useMemo(() => {
    if (!tags) return [];
    if (!tagQuery.trim()) return tags;
    const q = tagQuery.toLowerCase();
    return tags.filter((t) => t.tag.toLowerCase().includes(q));
  }, [tags, tagQuery]);

  const maxTagCount = useMemo(() => {
    if (!tags || tags.length === 0) return 1;
    return Math.max(...tags.map((t) => t.count), 1);
  }, [tags]);

  return (
    <aside className="flex w-72 shrink-0 flex-col border-r border-slate-800/80 bg-[#0b0d15] text-slate-200 select-none z-10">
      {/* Sidebar Header */}
      <div className="flex h-11 items-center justify-between border-b border-slate-800/60 px-4">
        <div className="flex items-center gap-2">
          <FilterIcon className="h-3.5 w-3.5 text-indigo-400" />
          <span className="text-xs font-semibold uppercase tracking-wider text-slate-300">Filters</span>
          {hasAnyFilter && (
            <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-indigo-500/20 px-1 text-[10px] font-mono text-indigo-300">
              {activeTags.length + predicates.length + (nearSample != null ? 1 : 0)}
            </span>
          )}
        </div>

        {hasAnyFilter && (
          <button
            onClick={() => {
              onClear();
              onClearPredicates();
              onClearNear();
            }}
            className="flex items-center gap-1 text-[11px] font-medium text-slate-400 hover:text-indigo-300 transition-colors"
          >
            <X className="h-3 w-3" />
            <span>Reset</span>
          </button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto p-3 space-y-4">
        {/* Similarity Search Active Banner */}
        {nearSample != null && (
          <div className="rounded-lg border border-purple-500/30 bg-purple-950/20 p-2.5 shadow-sm">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5 text-xs font-medium text-purple-300">
                <Sparkles className="h-3.5 w-3.5 text-purple-400" />
                <span>Similarity Search</span>
              </div>
              <button
                onClick={onClearNear}
                className="rounded p-0.5 text-purple-400 hover:bg-purple-900/40 hover:text-purple-200 transition-colors"
                title="Clear similarity search"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="mt-1 text-[11px] text-purple-300/80 font-mono">
              Sorted near sample <span className="font-semibold text-purple-200">#{nearSample}</span>
            </div>
          </div>
        )}

        {/* Active Filter Chips */}
        {predicates.length > 0 && (
          <div className="space-y-1.5">
            <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Active Rules</div>
            <div className="space-y-1">
              {predicates.map((p, i) => (
                <div
                  key={i}
                  className="group flex items-center justify-between gap-1.5 rounded-lg border border-slate-800 bg-slate-900/70 px-2.5 py-1.5 text-xs text-slate-200 hover:border-slate-700 transition-all"
                >
                  <span className="truncate font-mono text-[11px] text-indigo-300">{formatPredicate(p)}</span>
                  <button
                    onClick={() => onRemovePredicate(i)}
                    className="rounded p-0.5 text-slate-500 hover:bg-slate-800 hover:text-red-300 transition-colors"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Predicate / Metadata Filter Builder */}
        <div className="rounded-xl border border-slate-800/80 bg-slate-900/40 p-3 space-y-2.5">
          <div className="flex items-center gap-1.5 text-xs font-medium text-slate-300">
            <SlidersHorizontal className="h-3.5 w-3.5 text-slate-400" />
            <span>Add Metadata Rule</span>
          </div>

          <div className="space-y-1.5">
            {/* Field Select */}
            <select
              value={activeField?.name ?? ""}
              onChange={(e) => {
                setField(e.target.value);
                const nextField = scalarFields.find((f) => f.name === e.target.value);
                setOp(opsFor(nextField?.type)[0].value);
              }}
              className="w-full rounded-lg border border-slate-800 bg-slate-950/80 px-2.5 py-1.5 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none transition-all font-mono"
            >
              {scalarFields.map((f) => (
                <option key={f.name} value={f.name}>
                  {f.name} ({f.type ?? "scalar"})
                </option>
              ))}
            </select>

            {/* Operator Select & Value Input */}
            <div className="flex gap-1.5">
              <select
                value={op}
                onChange={(e) => setOp(e.target.value)}
                className="w-28 shrink-0 rounded-lg border border-slate-800 bg-slate-950/80 px-2 py-1.5 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none transition-all font-mono text-[11px]"
              >
                {ops.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>

              <input
                value={value}
                onChange={(e) => setValue(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && addPredicate()}
                placeholder={op === "in" || op === "not_in" ? "val1, val2" : "Value"}
                className="min-w-0 flex-1 rounded-lg border border-slate-800 bg-slate-950/80 px-2.5 py-1.5 text-xs text-slate-200 placeholder:text-slate-500 focus:border-indigo-500 focus:outline-none transition-all font-mono"
              />
            </div>

            <button
              onClick={addPredicate}
              disabled={!activeField || !value.trim()}
              className="flex w-full items-center justify-center gap-1.5 rounded-lg bg-slate-800 px-2.5 py-1.5 text-xs font-medium text-slate-200 hover:bg-indigo-600 hover:text-white disabled:opacity-40 disabled:pointer-events-none transition-all"
            >
              <Plus className="h-3.5 w-3.5" />
              <span>Apply Rule</span>
            </button>
          </div>
        </div>

        {/* Tag Facets Section */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-slate-400">
              <TagIcon className="h-3.5 w-3.5 text-slate-400" />
              <span>Tags</span>
            </div>
            {tags && tags.length > 0 && (
              <span className="text-[11px] text-slate-500 font-mono">{tags.length} total</span>
            )}
          </div>

          {/* Tag Search Input */}
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
            <input
              type="text"
              value={tagQuery}
              onChange={(e) => setTagQuery(e.target.value)}
              placeholder="Search tags…"
              className="w-full rounded-lg border border-slate-800 bg-slate-950/70 py-1.5 pl-8 pr-2.5 text-xs text-slate-200 placeholder:text-slate-500 focus:border-indigo-500 focus:outline-none transition-all"
            />
            {tagQuery && (
              <button
                onClick={() => setTagQuery("")}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>

          {/* Tags List */}
          {isLoading && (
            <div className="py-4 text-center text-xs text-slate-500 animate-pulse">Loading tags…</div>
          )}

          {!isLoading && tags?.length === 0 && (
            <div className="rounded-lg border border-dashed border-slate-800/80 p-4 text-center text-xs text-slate-500">
              No tags in dataset.
            </div>
          )}

          {!isLoading && filteredTags.length === 0 && tagQuery && (
            <div className="py-3 text-center text-xs text-slate-500">No matching tags.</div>
          )}

          <div className="space-y-1 max-h-96 overflow-y-auto pr-1">
            {filteredTags.map((t) => {
              const active = activeTags.includes(t.tag);
              const percentage = Math.round((t.count / maxTagCount) * 100);

              return (
                <button
                  key={t.tag}
                  onClick={() => onToggleTag(t.tag)}
                  className={`group relative flex w-full items-center justify-between overflow-hidden rounded-lg px-2.5 py-1.5 text-left text-xs transition-all ${
                    active
                      ? "border border-indigo-500/40 bg-indigo-950/40 text-indigo-200"
                      : "border border-transparent hover:border-slate-800 hover:bg-slate-900/60 text-slate-300"
                  }`}
                >
                  {/* Subtle Background Progress Bar */}
                  <div
                    className={`absolute inset-y-0 left-0 opacity-15 transition-all ${
                      active ? "bg-indigo-500" : "bg-slate-700 group-hover:opacity-25"
                    }`}
                    style={{ width: `${percentage}%` }}
                  />

                  <div className="relative z-10 flex items-center gap-2 min-w-0">
                    <div
                      className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded border transition-colors ${
                        active
                          ? "border-indigo-400 bg-indigo-500 text-white"
                          : "border-slate-700 bg-slate-950/60 group-hover:border-slate-600"
                      }`}
                    >
                      {active && <Check className="h-2.5 w-2.5 stroke-[3]" />}
                    </div>
                    <span className="truncate font-medium">{t.tag}</span>
                  </div>

                  <span
                    className={`relative z-10 font-mono text-[11px] ${
                      active ? "text-indigo-300 font-semibold" : "text-slate-500 group-hover:text-slate-400"
                    }`}
                  >
                    {t.count.toLocaleString()}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      </div>
    </aside>
  );
}

