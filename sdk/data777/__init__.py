"""Thin Python client for data777. See docs/sdk.md — every method here is a direct translation
of one HTTP call defined in docs/api.md; there is no second implementation of filtering,
selection, or pagination here."""

from .client import APIError, Client, JobFailed, JobHandle, connect
from .filters import Filter, NearField, Predicate, Selection, SortSpec, Stage
from .models import (
    Classification,
    Commit,
    Detection,
    FieldDef,
    Job,
    Keypoints,
    Progress,
    Sample,
    TagCount,
    TokenMeta,
    TokenSecret,
)
from .view import View

__all__ = [
    "connect",
    "Client",
    "JobHandle",
    "JobFailed",
    "APIError",
    "View",
    "Filter",
    "Stage",
    "Predicate",
    "SortSpec",
    "NearField",
    "Selection",
    "Sample",
    "Commit",
    "Job",
    "Progress",
    "FieldDef",
    "TagCount",
    "TokenMeta",
    "TokenSecret",
    "Classification",
    "Detection",
    "Keypoints",
]
