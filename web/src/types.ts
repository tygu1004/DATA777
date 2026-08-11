export type FieldKind = "scalar" | "tags" | "labels" | "embedding";

export interface FieldDef {
  name: string;
  kind: FieldKind;
  type?: string;
  dims?: number;
  metric?: string;
}

export type LabelValue = Record<string, unknown>;

export interface Sample {
  id: number;
  path: string;
  filename: string;
  width: number;
  height: number;
  filesize: number;
  format: string;
  media_type: string;
  parent_id: number;
  group_id: number;
  t: number;
  slice: string;
  duration: number;
  fps: number;
  tags: string[];
  labels?: Record<string, LabelValue[]>;
}

export type CommitKind = "set" | "patch";

export interface Commit {
  id: number;
  parent_id: number | null;
  message: string;
  kind: CommitKind;
  field: string;
  created_at: string;
  affected_count: number;
  op_count: number;
  is_head: boolean;
}

export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "canceled";

export interface Job<TResult = unknown> {
  id: string;
  kind: string;
  status: JobStatus;
  progress: { processed: number; total?: number };
  created_at: string;
  started_at?: string;
  finished_at?: string;
  error?: string;
  result?: TResult;
}

export interface TagCount {
  tag: string;
  count: number;
}

// --- Filter / Selection descriptors, mirroring docs/api.md ---

export interface Predicate {
  field: string;
  op: string;
  value: unknown;
}

export interface SortSpec {
  field?: string;
  dir?: "asc" | "desc";
  near?: { field: string; sample_id?: number; vector?: number[] };
}

export interface Stage {
  type: "match" | "sort" | "sample" | "rollup";
  match?: Predicate[];
  sort?: SortSpec;
  size?: number;
  balance?: { field: string };
  seed?: number;
  by?: string;
}

export interface Filter {
  stages?: Stage[];
}

export type Selection =
  | { mode: "explicit"; ids: number[] }
  | { mode: "filter"; filter: Filter; excluded?: number[] };
