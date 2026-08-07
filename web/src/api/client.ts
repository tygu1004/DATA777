import type { Commit, IndexStatus, Sample, TagOp } from "../types";

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
    return undefined as T;
  }
  return res.json() as Promise<T>;
}

export function startIndex(path: string): Promise<void> {
  return request("/api/index", { method: "POST", body: JSON.stringify({ path }) });
}

export function getIndexStatus(): Promise<IndexStatus> {
  return request("/api/index/status");
}

export function listSamples(offset: number, limit: number): Promise<{ total: number; items: Sample[] }> {
  return request(`/api/samples?offset=${offset}&limit=${limit}`);
}

export function thumbnailUrl(id: number): string {
  return `/api/thumbnails/${id}`;
}

export function createCommit(message: string, ops: TagOp[]): Promise<Commit> {
  return request("/api/commits", { method: "POST", body: JSON.stringify({ message, ops }) });
}

export function listCommits(offset = 0, limit = 50): Promise<{ items: Commit[] }> {
  return request(`/api/commits?offset=${offset}&limit=${limit}`);
}

export function undo(): Promise<{ head_commit_id: number | null }> {
  return request("/api/undo", { method: "POST" });
}
