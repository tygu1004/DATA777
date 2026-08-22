"""COCO/YOLO/Label Studio/HuggingFace/Parquet/JSONL export — pure client-side transformation over the ordinary read API
(docs/sdk.md#export-is-client-side-not-a-server-endpoint).
"""

from __future__ import annotations

import json
import os
from typing import TYPE_CHECKING, Any

from .models import Detection

if TYPE_CHECKING:
    from .view import View


def _default_label_field(view: "View") -> str:
    schema = view._client._schema_map()  # noqa: SLF001 — export lives next to Client, same package
    for field in schema.values():
        if field.kind == "labels" and field.type == "detection":
            return field.name
    raise ValueError("no detection-type labels field found; pass label_field explicitly")


def export_coco(view: "View", path: str, label_field: str | None = None) -> None:
    field = label_field or _default_label_field(view)
    categories: dict[str, int] = {}
    images = []
    annotations = []
    ann_id = 1

    for sample in view.samples():
        images.append(
            {"id": sample.id, "file_name": sample.filename, "width": sample.width, "height": sample.height}
        )
        for value in sample.labels.get(field, []):
            if not isinstance(value, Detection):
                continue
            if value.label not in categories:
                categories[value.label] = len(categories) + 1
            x, y, w, h = value.bbox
            annotations.append(
                {
                    "id": ann_id,
                    "image_id": sample.id,
                    "category_id": categories[value.label],
                    "bbox": [x * sample.width, y * sample.height, w * sample.width, h * sample.height],
                    "area": (w * sample.width) * (h * sample.height),
                    "iscrowd": 0,
                }
            )
            ann_id += 1

    coco = {
        "images": images,
        "annotations": annotations,
        "categories": [{"id": i, "name": name} for name, i in categories.items()],
    }
    os.makedirs(os.path.dirname(os.path.abspath(path)) or ".", exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(coco, f, indent=2)


def export_label_studio(
    view: "View", path: str, label_field: str | None = None, storage_prefix: str = ""
) -> None:
    """Exports samples to Label Studio Tasks format (.json) with pre-annotated bounding boxes and tags.
    Supports S3/RustFS Zero-Copy storage prefix (e.g. 's3://my-bucket/')."""
    tasks = []
    field = label_field
    prefix = storage_prefix.rstrip("/")

    for sample in view.samples():
        results = []

        # Convert tags to choices
        if sample.tags:
            results.append({
                "from_name": "tag",
                "to_name": "image",
                "type": "choices",
                "value": {"choices": sample.tags},
            })

        # Convert detections to rectanglelabels
        if field and field in sample.labels:
            for i, val in enumerate(sample.labels[field]):
                if not isinstance(val, Detection):
                    continue
                x, y, w, h = val.bbox
                results.append({
                    "id": f"result_{sample.id}_{i}",
                    "from_name": "label",
                    "to_name": "image",
                    "type": "rectanglelabels",
                    "original_width": sample.width,
                    "original_height": sample.height,
                    "image_rotation": 0,
                    "value": {
                        "rotation": 0,
                        "x": x * 100,
                        "y": y * 100,
                        "width": w * 100,
                        "height": h * 100,
                        "rectanglelabels": [val.label],
                    },
                })

        image_uri = f"{prefix}/{sample.path.lstrip('/')}" if prefix else sample.path
        task = {
            "id": sample.id,
            "data": {
                "image": image_uri,
                "filename": sample.filename,
                "width": sample.width,
                "height": sample.height,
                "filesize": sample.filesize,
                "format": sample.format,
            },
            "annotations": [{"result": results}] if results else [],
        }
        tasks.append(task)

    os.makedirs(os.path.dirname(os.path.abspath(path)) or ".", exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(tasks, f, indent=2)


def export_huggingface(
    view: "View", path: str, label_field: str | None = None, storage_prefix: str = ""
) -> None:
    """Exports dataset to Hugging Face ImageFolder metadata format (metadata.jsonl).
    Supports S3/RustFS Zero-Copy storage prefix (e.g. 's3://my-bucket/')."""
    os.makedirs(os.path.dirname(os.path.abspath(path)) or ".", exist_ok=True)
    field = label_field
    prefix = storage_prefix.rstrip("/")

    with open(path, "w", encoding="utf-8") as f:
        for sample in view.samples():
            image_uri = f"{prefix}/{sample.path.lstrip('/')}" if prefix else sample.path
            record: dict[str, Any] = {
                "file_name": sample.filename,
                "image_path": image_uri,
                "width": sample.width,
                "height": sample.height,
                "tags": sample.tags,
            }
            if field and field in sample.labels:
                bboxes = []
                categories = []
                for val in sample.labels[field]:
                    if isinstance(val, Detection):
                        bboxes.append([
                            val.bbox[0] * sample.width,
                            val.bbox[1] * sample.height,
                            val.bbox[2] * sample.width,
                            val.bbox[3] * sample.height,
                        ])
                        categories.append(val.label)
                record["objects"] = {"bbox": bboxes, "category": categories}
            f.write(json.dumps(record) + "\n")


def to_hf_dataset(view: "View", label_field: str | None = None) -> Any:
    """Converts the view directly into a Hugging Face datasets.Dataset object for model training or pushing to Hub."""
    try:
        from datasets import Dataset  # type: ignore[import-untyped]
    except ImportError as e:
        raise ImportError('Hugging Face datasets conversion needs datasets: pip install "datasets"') from e

    records = []
    field = label_field
    for sample in view.samples():
        rec: dict[str, Any] = {
            "image": sample.path,
            "filename": sample.filename,
            "width": sample.width,
            "height": sample.height,
            "tags": sample.tags,
        }
        if field and field in sample.labels:
            rec["objects"] = {
                "bbox": [v.bbox for v in sample.labels[field] if isinstance(v, Detection)],
                "category": [v.label for v in sample.labels[field] if isinstance(v, Detection)],
            }
        records.append(rec)
    return Dataset.from_list(records)


def export_yolo(view: "View", path: str, label_field: str | None = None) -> None:
    field = label_field or _default_label_field(view)
    os.makedirs(path, exist_ok=True)

    categories: dict[str, int] = {}
    samples = list(view.samples())
    for sample in samples:
        for value in sample.labels.get(field, []):
            if isinstance(value, Detection) and value.label not in categories:
                categories[value.label] = len(categories)

    with open(os.path.join(path, "classes.txt"), "w", encoding="utf-8") as f:
        for name in categories:
            f.write(name + "\n")

    for sample in samples:
        stem, _ = os.path.splitext(sample.filename)
        lines = []
        for value in sample.labels.get(field, []):
            if not isinstance(value, Detection):
                continue
            x, y, w, h = value.bbox
            cx, cy = x + w / 2, y + h / 2
            lines.append(f"{categories[value.label]} {cx:.6f} {cy:.6f} {w:.6f} {h:.6f}")
        with open(os.path.join(path, f"{stem}.txt"), "w", encoding="utf-8") as f:
            f.write("\n".join(lines))


def export_jsonl(view: "View", path: str) -> None:
    os.makedirs(os.path.dirname(os.path.abspath(path)) or ".", exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        for sample in view.samples():
            line = sample.model_dump() if hasattr(sample, "model_dump") else sample.__dict__
            f.write(json.dumps(line) + "\n")


def export_parquet(view: "View", path: str, label_field: str | None = None) -> None:
    try:
        import pyarrow as pa
        import pyarrow.parquet as pq
    except ImportError as e:
        raise ImportError('parquet export needs pyarrow: pip install "data777[parquet]"') from e

    ids, filenames, widths, heights, tags_col, labels_col = [], [], [], [], [], []
    for sample in view.samples():
        ids.append(sample.id)
        filenames.append(sample.filename)
        widths.append(sample.width)
        heights.append(sample.height)
        tags_col.append(sample.tags)
        if label_field:
            values = sample.labels.get(label_field, [])
            labels_col.append(json.dumps([v.model_dump() if hasattr(v, "model_dump") else v for v in values]))
        else:
            labels_col.append(None)

    table = pa.table(
        {
            "id": ids,
            "filename": filenames,
            "width": widths,
            "height": heights,
            "tags": tags_col,
            "labels": labels_col,
        }
    )
    pq.write_table(table, path)
