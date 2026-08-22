"""A View pins a filter to a commit — docs/sdk.md: "so a long export isn't disturbed by
curation that happens while it runs" (api.md#get-apisamples' `at_commit`)."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Iterator

from pydantic import BaseModel

if TYPE_CHECKING:
    from .client import Client
    from .models import Sample


class View:
    def __init__(self, client: "Client", filter: BaseModel | dict | None = None, at_commit: int | None = None):
        self._client = client
        self.filter = filter
        self.at_commit = at_commit

    def samples(self) -> Iterator["Sample"]:
        return self._client.samples(self.filter, at_commit=self.at_commit)

    def count(self) -> int:
        return self._client.count(self.filter, at_commit=self.at_commit)

    def to_hf_dataset(self, label_field: str | None = None) -> Any:
        """Converts view to a Hugging Face Dataset object (requires `pip install datasets`)."""
        from . import export as _export

        return _export.to_hf_dataset(self, label_field=label_field)

    def to_pytorch(
        self,
        transform: Any | None = None,
        storage_prefix: str = "",
        s3_client: Any | None = None,
    ) -> Any:
        """Returns a Zero-Copy Streaming PyTorch Dataset that pulls images on-the-fly from RustFS/S3."""
        from .torch import StreamingDataset

        return StreamingDataset(
            self, transform=transform, storage_prefix=storage_prefix, s3_client=s3_client
        )

    def export(self, format: str, path: str, label_field: str | None = None) -> None:
        """Client-side, not a server endpoint (docs/sdk.md#export-is-client-side-not-a-server-endpoint):
        format-specific serialization belongs in the layer that only needs the ordinary read
        API to do it."""
        from . import export as _export

        exporters: dict[str, Any] = {
            "coco": _export.export_coco,
            "yolo": _export.export_yolo,
            "label_studio": _export.export_label_studio,
            "huggingface": _export.export_huggingface,
            "hf": _export.export_huggingface,
            "jsonl": _export.export_jsonl,
            "parquet": _export.export_parquet,
        }
        if format not in exporters:
            raise ValueError(f"unknown export format {format!r}, want one of {sorted(exporters)}")
        exporters[format](self, path, label_field=label_field)
