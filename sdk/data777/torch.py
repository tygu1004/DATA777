"""Zero-Copy PyTorch Streaming Dataset for DATA777.
Streams images directly from RustFS / MinIO / S3 Object Storage into PyTorch DataLoader
without downloading or duplicating datasets on local disk.
"""

from __future__ import annotations

import io
from typing import TYPE_CHECKING, Any, Callable

if TYPE_CHECKING:
    from PIL import Image
    from .models import Sample
    from .view import View


class StreamingDataset:
    """PyTorch Dataset that streams images directly from S3/RustFS Object Storage or DATA777 server."""

    def __init__(
        self,
        view: "View",
        transform: Callable[[Image.Image], Any] | None = None,
        storage_prefix: str = "",
        s3_client: Any | None = None,
    ):
        self.view = view
        self.transform = transform
        self.storage_prefix = storage_prefix.rstrip("/")
        self.s3_client = s3_client
        self._samples: list["Sample"] = list(view.samples())

    def __len__(self) -> int:
        return len(self._samples)

    def __getitem__(self, idx: int) -> tuple[Any, dict[str, Any]]:
        from PIL import Image

        sample = self._samples[idx]
        image_bytes: bytes

        if self.s3_client and self.storage_prefix.startswith("s3://"):
            # Stream directly from S3 / RustFS / MinIO
            bucket_and_key = self.storage_prefix[5:] + "/" + sample.path.lstrip("/")
            bucket, key = bucket_and_key.split("/", 1)
            response = self.s3_client.get_object(Bucket=bucket, Key=key)
            image_bytes = response["Body"].read()
        else:
            # Fallback: Stream directly from DATA777 preview endpoint
            import urllib.request

            preview_url = f"{self.view._client.endpoint}/api/previews/{sample.id}"  # noqa: SLF001
            req = urllib.request.Request(preview_url)
            if self.view._client.token:  # noqa: SLF001
                req.add_header("Authorization", f"Bearer {self.view._client.token}")  # noqa: SLF001
            with urllib.request.urlopen(req) as resp:
                image_bytes = resp.read()

        img = Image.open(io.BytesIO(image_bytes)).convert("RGB")
        if self.transform:
            img = self.transform(img)

        metadata = {
            "id": sample.id,
            "filename": sample.filename,
            "tags": sample.tags,
            "labels": sample.labels,
            "width": sample.width,
            "height": sample.height,
        }
        return img, metadata
