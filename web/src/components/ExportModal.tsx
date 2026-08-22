import {
  Check,
  CheckCircle2,
  Copy,
  Database,
  Download,
  FileCode2,
  FileJson,
  Flame,
  HardDrive,
  Layers,
  Share2,
  Sparkles,
  Tag,
  Terminal,
  X,
} from "lucide-react";
import { useMemo, useState } from "react";
import type { Sample } from "../types";

export type ExportFormat = "label_studio" | "pytorch_stream" | "huggingface" | "coco" | "yolo" | "jsonl";

interface Props {
  isOpen: boolean;
  onClose: () => void;
  selectedIds: Set<number>;
  totalMatchingCount: number;
  chunks: Map<number, Sample[]>;
}

export default function ExportModal({
  isOpen,
  onClose,
  selectedIds,
  totalMatchingCount,
  chunks,
}: Props) {
  const [scope, setScope] = useState<"selected" | "loaded">("selected");
  const [format, setFormat] = useState<ExportFormat>("label_studio");
  const [storagePrefix, setStoragePrefix] = useState("s3://rustfs-bucket");
  const [copied, setCopied] = useState(false);

  // Collect loaded samples
  const allLoadedSamples = useMemo(() => {
    const list: Sample[] = [];
    chunks.forEach((chunk) => {
      list.push(...chunk);
    });
    return list;
  }, [chunks]);

  // Target samples based on scope
  const targetSamples = useMemo(() => {
    if (scope === "selected") {
      return allLoadedSamples.filter((s) => selectedIds.has(s.id));
    }
    return allLoadedSamples;
  }, [allLoadedSamples, scope, selectedIds]);

  // Generate URI with Storage Prefix
  const getObjectUri = (path: string) => {
    const cleanPrefix = storagePrefix.trim().replace(/\/+$/, "");
    if (!cleanPrefix) return path;
    const cleanPath = path.replace(/^\.?\/+/, "");
    return `${cleanPrefix}/${cleanPath}`;
  };

  // Generate Export Output
  const exportData = useMemo(() => {
    if (targetSamples.length === 0) {
      return { content: "", filename: "export.json", mimeType: "application/json", instructions: "", isCode: false };
    }

    if (format === "label_studio") {
      const tasks = targetSamples.map((sample) => {
        const results: Array<Record<string, unknown>> = [];

        // Tags as Choices
        if (sample.tags && sample.tags.length > 0) {
          results.push({
            from_name: "tag",
            to_name: "image",
            type: "choices",
            value: { choices: sample.tags },
          });
        }

        // Bounding boxes as rectanglelabels
        if (sample.labels) {
          Object.entries(sample.labels).forEach(([fieldName, values]) => {
            values.forEach((v, idx) => {
              const label = typeof v.label === "string" ? v.label : fieldName;
              const bbox = Array.isArray(v.bbox) ? (v.bbox as number[]) : undefined;
              if (bbox && bbox.length === 4) {
                results.push({
                  id: `res_${sample.id}_${fieldName}_${idx}`,
                  from_name: "label",
                  to_name: "image",
                  type: "rectanglelabels",
                  original_width: sample.width,
                  original_height: sample.height,
                  image_rotation: 0,
                  value: {
                    rotation: 0,
                    x: bbox[0] * 100,
                    y: bbox[1] * 100,
                    width: bbox[2] * 100,
                    height: bbox[3] * 100,
                    rectanglelabels: [label],
                  },
                });
              }
            });
          });
        }

        return {
          id: sample.id,
          data: {
            image: getObjectUri(sample.path),
            filename: sample.filename,
            width: sample.width,
            height: sample.height,
            filesize: sample.filesize,
            format: sample.format,
          },
          annotations: results.length > 0 ? [{ result: results }] : [],
        };
      });

      return {
        content: JSON.stringify(tasks, null, 2),
        filename: `label_studio_s3_tasks_${targetSamples.length}samples.json`,
        mimeType: "application/json",
        instructions: `# 1. Configure S3/RustFS Source Storage in Label Studio Project Settings\n# 2. Import this Zero-Copy JSON task pointer without uploading files`,
        isCode: false,
      };
    }

    if (format === "pytorch_stream") {
      const codeSnippet = `import torch
from torch.utils.data import DataLoader
from torchvision import transforms
import data777
import boto3

# 1. Connect to DATA777
client = data777.connect("http://localhost:8777")
view = client.view()

# 2. Configure S3 / RustFS client for Zero-Copy Streaming
s3 = boto3.client(
    "s3",
    endpoint_url="http://rustfs.internal:9000", # your RustFS / MinIO endpoint
    aws_access_key_id="rustfs_key",
    aws_secret_access_key="rustfs_secret",
)

# 3. Create Zero-Copy Streaming Dataset (0 disk copies)
dataset = view.to_pytorch(
    transform=transforms.Compose([
        transforms.Resize((224, 224)),
        transforms.ToTensor(),
    ]),
    storage_prefix="${storagePrefix || "s3://rustfs-bucket"}",
    s3_client=s3,
)

# 4. Stream directly into training loop
loader = DataLoader(dataset, batch_size=32, num_workers=4, shuffle=True)
for images, metadata in loader:
    # images: [32, 3, 224, 224] Tensor directly in memory
    print(f"Loaded batch with {len(images)} images, Sample IDs: {metadata['id']}")
`;

      return {
        content: codeSnippet,
        filename: "train_stream.py",
        mimeType: "text/x-python",
        instructions: `# Run directly in your training loop: pip install "data777[torch]" boto3 torchvision`,
        isCode: true,
      };
    }

    if (format === "huggingface") {
      const records = targetSamples.map((sample) => {
        const record: Record<string, unknown> = {
          file_name: sample.filename,
          image_url: getObjectUri(sample.path),
          width: sample.width,
          height: sample.height,
          tags: sample.tags || [],
        };

        if (sample.labels) {
          const bboxes: number[][] = [];
          const categories: string[] = [];
          Object.entries(sample.labels).forEach(([fieldName, values]) => {
            values.forEach((v) => {
              const label = typeof v.label === "string" ? v.label : fieldName;
              const bbox = Array.isArray(v.bbox) ? (v.bbox as number[]) : undefined;
              if (bbox && bbox.length === 4) {
                bboxes.push([
                  bbox[0] * sample.width,
                  bbox[1] * sample.height,
                  bbox[2] * sample.width,
                  bbox[3] * sample.height,
                ]);
                categories.push(label);
              }
            });
          });
          if (bboxes.length > 0) {
            record.objects = { bbox: bboxes, category: categories };
          }
        }
        return JSON.stringify(record);
      });

      return {
        content: records.join("\n"),
        filename: `metadata.jsonl`,
        mimeType: "application/x-ndjson",
        instructions: `from datasets import load_dataset\n# Zero-Copy Streaming: does not download full dataset to disk\nds = load_dataset("json", data_files="metadata.jsonl", streaming=True)`,
        isCode: false,
      };
    }

    if (format === "coco") {
      const categories: Record<string, number> = {};
      const images: Array<Record<string, unknown>> = [];
      const annotations: Array<Record<string, unknown>> = [];
      let annId = 1;

      targetSamples.forEach((sample) => {
        images.push({
          id: sample.id,
          file_name: sample.filename,
          s3_url: getObjectUri(sample.path),
          width: sample.width,
          height: sample.height,
        });

        if (sample.labels) {
          Object.entries(sample.labels).forEach(([fieldName, values]) => {
            values.forEach((v) => {
              const label = typeof v.label === "string" ? v.label : fieldName;
              if (!categories[label]) {
                categories[label] = Object.keys(categories).length + 1;
              }
              const bbox = Array.isArray(v.bbox) ? (v.bbox as number[]) : undefined;
              if (bbox && bbox.length === 4) {
                const px = bbox[0] * sample.width;
                const py = bbox[1] * sample.height;
                const pw = bbox[2] * sample.width;
                const ph = bbox[3] * sample.height;
                annotations.push({
                  id: annId++,
                  image_id: sample.id,
                  category_id: categories[label],
                  bbox: [px, py, pw, ph],
                  area: pw * ph,
                  iscrowd: 0,
                });
              }
            });
          });
        }
      });

      const coco = {
        images,
        annotations,
        categories: Object.entries(categories).map(([name, id]) => ({ id, name })),
      };

      return {
        content: JSON.stringify(coco, null, 2),
        filename: `cvat_coco_${targetSamples.length}samples.json`,
        mimeType: "application/json",
        instructions: `# Import into CVAT: Task Actions -> Upload Annotations -> COCO 1.0`,
        isCode: false,
      };
    }

    if (format === "yolo") {
      const categories: Record<string, number> = {};
      targetSamples.forEach((s) => {
        if (s.labels) {
          Object.entries(s.labels).forEach(([fieldName, values]) => {
            values.forEach((v) => {
              const label = typeof v.label === "string" ? v.label : fieldName;
              if (categories[label] === undefined) {
                categories[label] = Object.keys(categories).length;
              }
            });
          });
        }
      });

      let yoloDoc = `# Ultralytics YOLO dataset configuration (data.yaml)\n`;
      yoloDoc += `# Storage root: ${storagePrefix || "s3://rustfs-bucket"}\n`;
      yoloDoc += `nc: ${Object.keys(categories).length}\n`;
      yoloDoc += `names: [${Object.keys(categories).map((c) => `'${c}'`).join(", ")}]\n\n`;
      yoloDoc += `# Annotations format: <class_index> <cx> <cy> <width> <height>\n`;

      targetSamples.forEach((sample) => {
        yoloDoc += `\n# --- Sample #${sample.id} (${getObjectUri(sample.path)}) ---\n`;
        if (sample.labels) {
          Object.entries(sample.labels).forEach(([fieldName, values]) => {
            values.forEach((v) => {
              const label = typeof v.label === "string" ? v.label : fieldName;
              const catIdx = categories[label] ?? 0;
              const bbox = Array.isArray(v.bbox) ? (v.bbox as number[]) : undefined;
              if (bbox && bbox.length === 4) {
                const cx = bbox[0] + bbox[2] / 2;
                const cy = bbox[1] + bbox[3] / 2;
                yoloDoc += `${catIdx} ${cx.toFixed(6)} ${cy.toFixed(6)} ${bbox[2].toFixed(6)} ${bbox[3].toFixed(6)}\n`;
              }
            });
          });
        }
      });

      return {
        content: yoloDoc,
        filename: `yolo_${targetSamples.length}samples.txt`,
        mimeType: "text/plain",
        instructions: `# Ultralytics YOLO training dataset annotations and S3 URI mapping`,
        isCode: false,
      };
    }

    // Default: jsonl manifest
    const jsonl = targetSamples.map((s) => JSON.stringify({ ...s, s3_uri: getObjectUri(s.path) })).join("\n");
    return {
      content: jsonl,
      filename: `manifest_zero_copy_${targetSamples.length}samples.jsonl`,
      mimeType: "application/x-ndjson",
      instructions: `# Zero-Copy Manifest: Lightweight pointers to Object Storage files`,
      isCode: false,
    };
  }, [format, targetSamples, storagePrefix]);

  if (!isOpen) return null;

  const handleDownload = () => {
    if (!exportData.content) return;
    const blob = new Blob([exportData.content], { type: exportData.mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = exportData.filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const handleCopy = () => {
    if (!exportData.content) return;
    navigator.clipboard.writeText(exportData.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 backdrop-blur-md animate-in fade-in duration-200"
      onClick={onClose}
    >
      <div
        className="relative flex h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-slate-800 bg-[#0c0e17] shadow-2xl shadow-black/90"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-slate-800 bg-slate-950/70 px-5 backdrop-blur-md">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
              <Database className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-slate-100 flex items-center gap-2">
                Zero-Copy Dataset Export
                <span className="flex items-center gap-1 rounded-md bg-emerald-500/15 px-2 py-0.5 text-[10px] font-mono text-emerald-300 border border-emerald-500/30">
                  <Sparkles className="h-2.5 w-2.5" />
                  Zero Media Duplication
                </span>
              </h2>
              <p className="text-[11px] text-slate-400">
                Stream pointers directly to RustFS/S3 without duplicating massive media files on disk
              </p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-800 bg-slate-900/60 text-slate-400 hover:bg-slate-800 hover:text-white transition-all"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Content Body */}
        <div className="flex min-h-0 flex-1 flex-col md:flex-row overflow-hidden">
          {/* Left Controls Column */}
          <div className="w-full md:w-84 border-b md:border-b-0 md:border-r border-slate-800 bg-slate-900/20 p-4 sm:p-5 space-y-5 overflow-y-auto">
            {/* Scope Selection */}
            <div>
              <label className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
                Export Target Scope
              </label>
              <div className="mt-2 grid grid-cols-2 gap-2">
                <button
                  onClick={() => setScope("selected")}
                  disabled={selectedIds.size === 0}
                  className={`flex flex-col items-center justify-center rounded-xl border p-2.5 text-center transition-all ${
                    scope === "selected"
                      ? "border-emerald-500/50 bg-emerald-500/10 text-emerald-300 shadow-sm"
                      : "border-slate-800 bg-slate-900/50 text-slate-400 hover:bg-slate-800 hover:text-slate-200 disabled:opacity-30 disabled:pointer-events-none"
                  }`}
                >
                  <span className="text-xs font-semibold">Selected Only</span>
                  <span className="text-[10px] font-mono text-slate-400 mt-0.5">
                    {selectedIds.size} samples
                  </span>
                </button>

                <button
                  onClick={() => setScope("loaded")}
                  className={`flex flex-col items-center justify-center rounded-xl border p-2.5 text-center transition-all ${
                    scope === "loaded"
                      ? "border-emerald-500/50 bg-emerald-500/10 text-emerald-300 shadow-sm"
                      : "border-slate-800 bg-slate-900/50 text-slate-400 hover:bg-slate-800 hover:text-slate-200"
                  }`}
                >
                  <span className="text-xs font-semibold">All Matching</span>
                  <span className="text-[10px] font-mono text-slate-400 mt-0.5">
                    {totalMatchingCount} samples
                  </span>
                </button>
              </div>
            </div>

            {/* Storage URI Prefix Config */}
            <div>
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase flex items-center gap-1.5">
                  <HardDrive className="h-3 w-3 text-indigo-400" />
                  Object Storage URI Prefix
                </label>
              </div>
              <p className="text-[10px] text-slate-500 mt-0.5">
                Target RustFS, MinIO, or S3 bucket path for Zero-Copy linking
              </p>
              <input
                type="text"
                value={storagePrefix}
                onChange={(e) => setStoragePrefix(e.target.value)}
                placeholder="s3://rustfs-bucket or rustfs://lake"
                className="mt-2 w-full rounded-lg border border-slate-700/80 bg-slate-950 px-3 py-2 text-xs font-mono text-slate-200 placeholder-slate-600 focus:border-emerald-500 focus:outline-none transition-colors"
              />
            </div>

            {/* Target Platform / Format */}
            <div>
              <label className="text-[11px] font-semibold tracking-wider text-slate-400 uppercase">
                Destination Format
              </label>
              <div className="mt-2 space-y-1.5">
                {[
                  {
                    id: "label_studio",
                    name: "Label Studio S3/RustFS Sync",
                    desc: "Zero-copy task pointers to Cloud Storage files",
                    icon: Tag,
                    color: "text-orange-400",
                  },
                  {
                    id: "pytorch_stream",
                    name: "PyTorch Streaming DataLoader",
                    desc: "Stream batches from RustFS directly into GPU",
                    icon: Flame,
                    color: "text-red-400",
                  },
                  {
                    id: "huggingface",
                    name: "🤗 Hugging Face Streaming Dataset",
                    desc: "metadata.jsonl format with streaming=True support",
                    icon: Share2,
                    color: "text-amber-400",
                  },
                  {
                    id: "coco",
                    name: "CVAT / COCO 1.0 JSON",
                    desc: "Standard COCO instances with S3 object references",
                    icon: Layers,
                    color: "text-blue-400",
                  },
                  {
                    id: "yolo",
                    name: "Ultralytics YOLO (TXT + YAML)",
                    desc: "Bounding box annotations with S3 URI manifest",
                    icon: Sparkles,
                    color: "text-purple-400",
                  },
                  {
                    id: "jsonl",
                    name: "Zero-Copy JSONL Manifest",
                    desc: "Lightweight URI & metadata pointers for Ray / custom pipelines",
                    icon: FileCode2,
                    color: "text-slate-400",
                  },
                ].map((fmt) => {
                  const Icon = fmt.icon;
                  const active = format === fmt.id;
                  return (
                    <button
                      key={fmt.id}
                      onClick={() => setFormat(fmt.id as ExportFormat)}
                      className={`w-full flex items-start gap-2.5 rounded-xl border p-2.5 text-left transition-all ${
                        active
                          ? "border-emerald-500/50 bg-emerald-500/10 shadow-sm"
                          : "border-slate-800/80 bg-slate-900/40 hover:bg-slate-800/50 hover:border-slate-700"
                      }`}
                    >
                      <div
                        className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border border-slate-700/50 bg-slate-800/80 ${fmt.color}`}
                      >
                        <Icon className="h-3.5 w-3.5" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between">
                          <span
                            className={`text-xs font-semibold ${
                              active ? "text-emerald-200" : "text-slate-200"
                            }`}
                          >
                            {fmt.name}
                          </span>
                          {active && <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" />}
                        </div>
                        <p className="text-[10px] text-slate-400 mt-0.5 line-clamp-1">{fmt.desc}</p>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          {/* Right Preview & Action Column */}
          <div className="flex flex-1 flex-col min-h-0 bg-[#08090d] p-4 sm:p-5">
            <div className="flex items-center justify-between pb-3">
              <div className="flex items-center gap-2">
                {exportData.isCode ? (
                  <Terminal className="h-4 w-4 text-emerald-400" />
                ) : (
                  <FileJson className="h-4 w-4 text-slate-400" />
                )}
                <span className="font-mono text-xs text-slate-300 font-semibold truncate max-w-[280px]">
                  {exportData.filename}
                </span>
              </div>
              <span className="text-[11px] font-mono text-slate-500">
                {(new Blob([exportData.content]).size / 1024).toFixed(1)} KB (Zero Media Copied)
              </span>
            </div>

            {/* Code / Content Preview Window */}
            <div className="flex-1 min-h-0 rounded-xl border border-slate-800/80 bg-slate-950 p-4 font-mono text-xs text-slate-300 overflow-auto">
              <pre className="whitespace-pre font-mono">{exportData.content || "// No samples selected"}</pre>
            </div>

            {/* Quick snippet hint */}
            {exportData.instructions && (
              <div className="mt-3 rounded-lg border border-slate-800 bg-slate-900/60 p-2.5 text-[11px] font-mono text-slate-400">
                <span className="text-emerald-400 font-bold"># Zero-Copy Workflow: </span>
                <span className="whitespace-pre-line">{exportData.instructions}</span>
              </div>
            )}

            {/* Bottom Actions */}
            <div className="mt-3 flex items-center justify-between pt-2 border-t border-slate-800/80">
              <span className="text-[11px] text-slate-400">
                Targeting <strong className="text-slate-200">{targetSamples.length}</strong> items in{" "}
                <code className="text-emerald-400">{storagePrefix || "S3"}</code>
              </span>

              <div className="flex items-center gap-2">
                <button
                  onClick={handleCopy}
                  disabled={targetSamples.length === 0}
                  className="flex items-center gap-1.5 rounded-xl border border-slate-700 bg-slate-800/80 px-3 py-2 text-xs font-medium text-slate-300 hover:bg-slate-700 hover:text-white transition-all disabled:opacity-30 disabled:pointer-events-none"
                >
                  {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                  {copied ? "Copied!" : exportData.isCode ? "Copy Code" : "Copy Manifest"}
                </button>

                <button
                  onClick={handleDownload}
                  disabled={targetSamples.length === 0}
                  className="flex items-center gap-2 rounded-xl bg-emerald-600 px-4 py-2 text-xs font-semibold text-white shadow-lg shadow-emerald-950/60 hover:bg-emerald-500 transition-all disabled:opacity-30 disabled:pointer-events-none"
                >
                  <Download className="h-3.5 w-3.5" />
                  {exportData.isCode ? "Download Script" : "Download Pointer"}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
