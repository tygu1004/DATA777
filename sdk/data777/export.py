"""COCO/YOLO/Parquet export — pure client-side transformation over the ordinary read API
(docs/sdk.md#export-is-client-side-not-a-server-endpoint). Only detection-style label fields
are supported for COCO/YOLO, since both formats are bounding-box formats by construction.
"""

from __future__ import annotations

import json
import os
from typing import TYPE_CHECKING

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
    with open(path, "w") as f:
        json.dump(coco, f)


def export_yolo(view: "View", path: str, label_field: str | None = None) -> None:
    field = label_field or _default_label_field(view)
    os.makedirs(path, exist_ok=True)

    categories: dict[str, int] = {}
    # first pass: collect category names so classes.txt order is stable regardless of write order
    samples = list(view.samples())
    for sample in samples:
        for value in sample.labels.get(field, []):
            if isinstance(value, Detection) and value.label not in categories:
                categories[value.label] = len(categories)

    with open(os.path.join(path, "classes.txt"), "w") as f:
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
        with open(os.path.join(path, f"{stem}.txt"), "w") as f:
            f.write("\n".join(lines))


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
