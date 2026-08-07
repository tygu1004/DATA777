export interface Sample {
  id: number;
  path: string;
  filename: string;
  width: number;
  height: number;
  filesize: number;
  format: string;
  tags: string[];
}

export interface Commit {
  id: number;
  parent_id: number | null;
  message: string;
  created_at: string;
  op_count: number;
  is_head: boolean;
}

export type IndexState = "idle" | "running" | "done" | "error";

export interface IndexStatus {
  status: IndexState;
  processed: number;
  error?: string;
}

export interface TagOp {
  sample_id: number;
  tag: string;
  op: "add" | "remove";
}
