"""Filter and Selection descriptors, mirroring docs/api.md exactly. No query builder DSL on
top of these — pass a plain dict or one of these models directly (docs/sdk.md#non-goals:
"they're already small, serializable, and shared with every other client")."""

from __future__ import annotations

import base64
import json
from typing import Any, Literal

from pydantic import BaseModel


class Predicate(BaseModel):
    field: str
    op: str
    value: Any


class NearField(BaseModel):
    field: str
    sample_id: int | None = None
    vector: list[float] | None = None


class SortSpec(BaseModel):
    field: str | None = None
    dir: Literal["asc", "desc"] | None = None
    near: NearField | None = None


class Stage(BaseModel):
    type: Literal["match", "sort", "sample", "rollup"]
    match: list[Predicate] | None = None
    sort: SortSpec | None = None
    size: int | None = None
    balance: dict | None = None
    seed: int | None = None
    by: str | None = None


class Filter(BaseModel):
    stages: list[Stage] = []


class Selection(BaseModel):
    mode: Literal["explicit", "filter"]
    ids: list[int] | None = None
    filter: dict | None = None
    excluded: list[int] | None = None


def _dump(x: BaseModel | dict | None) -> dict | None:
    if x is None:
        return None
    if isinstance(x, BaseModel):
        return x.model_dump(exclude_none=True)
    return x


def encode_filter(filter: BaseModel | dict | None) -> str:
    """base64url, no padding — api.md#filter: "Encoding in query strings"."""
    data = _dump(filter)
    if not data or not data.get("stages"):
        return ""
    raw = json.dumps(data).encode()
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()
