"""Typed models generated from the field/label type definitions in docs/api.md#fields, not
hand-maintained copies — see docs/sdk.md#typed-models for why Pydantic over a hand-rolled
schema."""

from __future__ import annotations

from datetime import datetime
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict


class Classification(BaseModel):
    model_config = ConfigDict(extra="allow")
    label: str
    confidence: float | None = None


class Detection(BaseModel):
    model_config = ConfigDict(extra="allow")
    label: str
    confidence: float | None = None
    bbox: tuple[float, float, float, float]


class Keypoints(BaseModel):
    model_config = ConfigDict(extra="allow")
    label: str
    points: list[tuple[float, float]]
    confidence: float | None = None


LABEL_TYPES: dict[str, type[BaseModel]] = {
    "classification": Classification,
    "detection": Detection,
    "keypoints": Keypoints,
}


class FieldDef(BaseModel):
    name: str
    kind: Literal["scalar", "tags", "labels", "embedding"]
    type: str | None = None
    dims: int | None = None
    metric: str | None = None


class Sample(BaseModel):
    id: int
    path: str
    filename: str
    width: int
    height: int
    filesize: int
    format: str
    media_type: str = "image"
    parent_id: int = 0
    group_id: int = 0
    t: float = 0
    slice: str = ""
    duration: float = 0
    fps: float = 0
    tags: list[str] = []
    # Values are typed (Classification/Detection/Keypoints) when the field's declared type is
    # known — see Client._parse_sample. Untyped/unknown fields keep raw dicts.
    labels: dict[str, list[Any]] = {}


class TagCount(BaseModel):
    tag: str
    count: int


class Commit(BaseModel):
    id: int
    parent_id: int | None
    message: str
    kind: Literal["set", "patch"]
    field: str
    created_at: datetime
    affected_count: int
    op_count: int
    is_head: bool


class Progress(BaseModel):
    processed: int
    total: int | None = None


class Job(BaseModel):
    id: str
    kind: str
    status: Literal["queued", "running", "succeeded", "failed", "canceled"]
    progress: Progress
    created_at: datetime
    started_at: datetime | None = None
    finished_at: datetime | None = None
    error: str | None = None
    result: Any = None


class TokenMeta(BaseModel):
    id: str
    name: str
    created_at: datetime
    expires_at: datetime | None = None


class TokenSecret(TokenMeta):
    secret: str
