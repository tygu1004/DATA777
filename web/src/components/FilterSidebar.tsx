import { useTagCounts } from "../hooks/useSamples";

interface Props {
  activeTags: string[];
  onToggleTag: (tag: string) => void;
  onClear: () => void;
}

// Tag counts are always computed against the whole dataset, not narrowed by the active
// filter — a simpler-to-reason-about facet list than one that reflows as you select, at the
// cost of not showing "remaining count within the current view" per tag.
export default function FilterSidebar({ activeTags, onToggleTag, onClear }: Props) {
  const { data: tags, isLoading } = useTagCounts();

  return (
    <div className="w-56 shrink-0 overflow-y-auto border-r border-white/10 p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-semibold text-neutral-200">Tags</span>
        {activeTags.length > 0 && (
          <button onClick={onClear} className="text-xs text-blue-400 hover:underline">
            Clear
          </button>
        )}
      </div>
      {isLoading && <p className="text-xs text-neutral-500">Loading…</p>}
      {tags?.length === 0 && <p className="text-xs text-neutral-500">No tags yet.</p>}
      <ul className="space-y-0.5">
        {tags?.map((t) => {
          const active = activeTags.includes(t.tag);
          return (
            <li key={t.tag}>
              <button
                onClick={() => onToggleTag(t.tag)}
                className={`flex w-full items-center justify-between rounded px-2 py-1 text-left text-sm ${
                  active ? "bg-blue-500/20 text-blue-300" : "text-neutral-300 hover:bg-white/5"
                }`}
              >
                <span className="truncate">{t.tag}</span>
                <span className="text-xs text-neutral-500">{t.count}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
