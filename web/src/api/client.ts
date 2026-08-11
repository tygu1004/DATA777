import type { Commit, FieldDef, Filter, Job, Sample, Selection, TagCount } from "../types";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `${res.status} ${res.statusText}`);
  }
  if (res.status === 204 || res.status === 202) {
    return (await res.json().catch(() => undefined)) as T;
  }
  return res.json() as Promise<T>;
}

// base64url with no padding. api.md's "Encoding in query strings" note suggests sorting keys
// for canonical output (HTTP cache friendliness) — skipped here: JSON.stringify's array-form
// replacer applies its key allowlist to every nested object, not just the top level, so a
// naive `Object.keys(filter).sort()` replacer silently strips every nested field (type,
// match, field, op, value...) down to `{}`. Not worth a recursive key-sorter for a cache
// nicety; plain JSON.stringify is correct, just not guaranteed byte-identical across clients.
export function encodeFilter(filter: Filter): string {
  const json = JSON.stringify(filter);
  const bytes = new TextEncoder().encode(json);
  let binary = "";
  bytes.forEach((b) => (binary += String.fromCharCode(b)));
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function filterParam(filter?: Filter): string {
  if (!filter || !filter.stages || filter.stages.length === 0) return "";
  return `filter=${encodeFilter(filter)}`;
}

export function thumbnailUrl(id: number): string {
  return `/api/thumbnails/${id}`;
}

export function previewUrl(id: number): string {
  return `/api/previews/${id}`;
}

export function getSchema(): Promise<{ fields: FieldDef[] }> {
  return request("/api/schema");
}

export interface ListSamplesResult {
  items: Sample[];
  next_cursor?: string;
  seed?: number;
}

export function listSamples(filter: Filter | undefined, offset: number, limit: number): Promise<ListSamplesResult> {
  const params = [filterParam(filter), `offset=${offset}`, `limit=${limit}`].filter(Boolean).join("&");
  return request(`/api/samples?${params}`);
}

export function countSamples(filter?: Filter): Promise<{ count: number }> {
  const params = filterParam(filter);
  return request(`/api/samples/count${params ? `?${params}` : ""}`);
}

export function getTagCounts(filter?: Filter): Promise<{ items: TagCount[] }> {
  const params = filterParam(filter);
  return request(`/api/tags${params ? `?${params}` : ""}`);
}

export function startIndex(path: string): Promise<{ job_id: string }> {
  return request("/api/index", { method: "POST", body: JSON.stringify({ path }) });
}

export interface CreateSetCommitRequest {
  message: string;
  kind: "set";
  field: "tags";
  selection: Selection;
  op: "add" | "remove";
  value: string;
}

export function createSetCommit(req: CreateSetCommitRequest): Promise<{ job_id: string }> {
  return request("/api/commits", { method: "POST", body: JSON.stringify(req) });
}

export function listCommits(offset = 0, limit = 50): Promise<{ items: Commit[] }> {
  return request(`/api/commits?offset=${offset}&limit=${limit}`);
}

export function undo(expectedHead: number | null): Promise<{ job_id: string }> {
  return request("/api/undo", {
    method: "POST",
    body: JSON.stringify(expectedHead == null ? {} : { expected_head: expectedHead }),
  });
}

export function getJob<T = unknown>(id: string, waitSeconds = 0): Promise<Job<T>> {
  return request(`/api/jobs/${id}${waitSeconds > 0 ? `?wait=${waitSeconds}` : ""}`);
}

// pollJob long-polls a job to a terminal state, using the server's own ?wait= support so most
// jobs resolve in one round trip instead of a client-side setInterval loop.
export async function pollJob<T = unknown>(id: string, onProgress?: (job: Job<T>) => void): Promise<Job<T>> {
  for (;;) {
    const job = await getJob<T>(id, 3);
    onProgress?.(job);
    if (job.status === "succeeded" || job.status === "failed" || job.status === "canceled") {
      return job;
    }
  }
}
