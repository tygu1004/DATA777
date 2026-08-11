"""The Client is a direct translation of the HTTP calls in docs/api.md — no client-side query
planner, no local cache of dataset state (docs/sdk.md#non-goals)."""

from __future__ import annotations

import time
from datetime import datetime
from typing import Any, Iterator

import requests
from pydantic import BaseModel

from .filters import Selection, encode_filter
from .models import LABEL_TYPES, Commit, FieldDef, Job, Sample, TagCount, TokenMeta, TokenSecret


class APIError(Exception):
    def __init__(self, status: int, message: str):
        super().__init__(f"{status}: {message}")
        self.status = status
        self.message = message


class JobFailed(Exception):
    def __init__(self, job: Job):
        super().__init__(job.error or f"job {job.id} failed")
        self.job = job


def _dump(x: BaseModel | dict | None) -> dict | None:
    if x is None:
        return None
    return x.model_dump(exclude_none=True) if isinstance(x, BaseModel) else x


class JobHandle:
    """A job_id plus the polling loop api.md#jobs describes: `?wait=Ns` long-polling means a
    small mutation resolves in one round trip, a large one keeps polling."""

    def __init__(self, client: "Client", job_id: str):
        self._client = client
        self.id = job_id
        self._last: Job | None = None

    def wait(self, poll_seconds: float = 3.0, timeout: float | None = None) -> Job:
        start = time.monotonic()
        while True:
            job = self._client.job(self.id, wait=poll_seconds)
            self._last = job
            if job.status in ("succeeded", "failed", "canceled"):
                if job.status == "failed":
                    raise JobFailed(job)
                return job
            if timeout is not None and time.monotonic() - start > timeout:
                raise TimeoutError(f"job {self.id} did not reach a terminal state within {timeout}s")

    @property
    def result(self) -> Any:
        if self._last is None or self._last.status not in ("succeeded", "failed", "canceled"):
            self.wait()
        return self._last.result

    def cancel(self) -> None:
        self._client.cancel_job(self.id)


class Client:
    def __init__(self, base_url: str, token: str | None = None, timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.session = requests.Session()
        if token:
            self.session.headers["Authorization"] = f"Bearer {token}"
        self._schema_cache: dict[str, FieldDef] | None = None

    def _request(self, method: str, path: str, **kwargs) -> Any:
        resp = self.session.request(method, f"{self.base_url}{path}", timeout=self.timeout, **kwargs)
        if not resp.ok:
            try:
                message = resp.json().get("error", resp.text)
            except ValueError:
                message = resp.text
            raise APIError(resp.status_code, message)
        if not resp.content:
            return None
        return resp.json()

    # --- schema (api.md#fields) ---

    def schema(self) -> list[FieldDef]:
        return [FieldDef(**f) for f in self._request("GET", "/api/schema")["fields"]]

    def define_field(self, field: FieldDef | dict) -> FieldDef:
        result = self._request("POST", "/api/schema/fields", json=_dump(field))
        self._schema_cache = None  # invalidate: a new field changes how future samples parse
        return FieldDef(**result)

    def _schema_map(self) -> dict[str, FieldDef]:
        if self._schema_cache is None:
            self._schema_cache = {f.name: f for f in self.schema()}
        return self._schema_cache

    def _parse_sample(self, raw: dict) -> Sample:
        schema = self._schema_map()
        labels: dict[str, list[Any]] = {}
        for field, items in (raw.get("labels") or {}).items():
            field_def = schema.get(field)
            model = LABEL_TYPES.get(field_def.type) if field_def and field_def.type else None
            labels[field] = [model(**item) for item in items] if model else items
        return Sample(**{**raw, "labels": labels})

    # --- reads (api.md#get-apisamples etc.) ---

    def sample_page(
        self, filter: BaseModel | dict | None = None, offset: int = 0, limit: int = 200, at_commit: int | None = None
    ) -> tuple[list[Sample], str | None]:
        params: dict[str, Any] = {"offset": offset, "limit": limit}
        encoded = encode_filter(filter)
        if encoded:
            params["filter"] = encoded
        if at_commit is not None:
            params["at_commit"] = at_commit
        data = self._request("GET", "/api/samples", params=params)
        return [self._parse_sample(s) for s in data["items"]], data.get("next_cursor")

    def samples(self, filter: BaseModel | dict | None = None, at_commit: int | None = None) -> Iterator[Sample]:
        """Cursor-based full walk (docs/api.md: stable independent of offset/mutations) — what
        a script iterating the whole matching set wants, unlike the grid's `offset`."""
        cursor: str | None = None
        encoded = encode_filter(filter)
        while True:
            params: dict[str, Any] = {"limit": 200}
            if encoded:
                params["filter"] = encoded
            if at_commit is not None:
                params["at_commit"] = at_commit
            if cursor:
                params["cursor"] = cursor
            data = self._request("GET", "/api/samples", params=params)
            for raw in data["items"]:
                yield self._parse_sample(raw)
            cursor = data.get("next_cursor")
            if not cursor:
                return

    def count(self, filter: BaseModel | dict | None = None, at_commit: int | None = None) -> int:
        params: dict[str, Any] = {}
        encoded = encode_filter(filter)
        if encoded:
            params["filter"] = encoded
        if at_commit is not None:
            params["at_commit"] = at_commit
        return self._request("GET", "/api/samples/count", params=params)["count"]

    def tag_counts(self, filter: BaseModel | dict | None = None, at_commit: int | None = None) -> list[TagCount]:
        params: dict[str, Any] = {}
        encoded = encode_filter(filter)
        if encoded:
            params["filter"] = encoded
        if at_commit is not None:
            params["at_commit"] = at_commit
        return [TagCount(**t) for t in self._request("GET", "/api/tags", params=params)["items"]]

    # --- mutations (api.md#post-apicommits) ---

    def tag(
        self, selection: Selection | dict, tag: str, op: str = "add", message: str | None = None
    ) -> JobHandle:
        body = {
            "message": message or f'{op} "{tag}"',
            "kind": "set",
            "field": "tags",
            "selection": _dump(selection),
            "op": op,
            "value": tag,
        }
        data = self._request("POST", "/api/commits", json=body)
        return JobHandle(self, data["job_id"])

    def patch(self, field: str, patches: list[dict], message: str | None = None) -> JobHandle:
        body = {"message": message or f"patch {field}", "kind": "patch", "field": field, "patches": patches}
        data = self._request("POST", "/api/commits", json=body)
        return JobHandle(self, data["job_id"])

    def commits(self, offset: int = 0, limit: int = 50) -> list[Commit]:
        items = self._request("GET", "/api/commits", params={"offset": offset, "limit": limit})["items"]
        return [Commit(**c) for c in items]

    def head(self) -> int | None:
        for c in self.commits(0, 1):
            if c.is_head:
                return c.id
        return None

    def undo(self, expected_head: int | None = None) -> JobHandle:
        body = {"expected_head": expected_head} if expected_head is not None else {}
        data = self._request("POST", "/api/undo", json=body)
        return JobHandle(self, data["job_id"])

    def index(self, path: str) -> JobHandle:
        data = self._request("POST", "/api/index", json={"path": path})
        return JobHandle(self, data["job_id"])

    # --- jobs (api.md#jobs) ---

    def job(self, job_id: str, wait: float = 0) -> Job:
        params = {"wait": wait} if wait else {}
        return Job(**self._request("GET", f"/api/jobs/{job_id}", params=params))

    def cancel_job(self, job_id: str) -> None:
        self._request("POST", f"/api/jobs/{job_id}/cancel")

    def jobs(self, status: str | None = None) -> list[Job]:
        params = {"status": status} if status else {}
        return [Job(**j) for j in self._request("GET", "/api/jobs", params=params)["items"]]

    # --- embeddings (api.md#post-apiembeddingsfield) ---

    def upsert_embeddings(self, field: str, items: list[tuple[int, list[float]]]) -> None:
        body = {"items": [{"sample_id": sid, "vector": vec} for sid, vec in items]}
        self._request("POST", f"/api/embeddings/{field}", json=body)

    def embedding(self, field: str, sample_id: int) -> list[float]:
        return self._request("GET", f"/api/embeddings/{field}/{sample_id}")["vector"]

    # --- tokens (api.md#authentication) ---

    def create_token(self, name: str, expires_at: datetime | None = None) -> TokenSecret:
        body: dict[str, Any] = {"name": name}
        if expires_at is not None:
            body["expires_at"] = expires_at.isoformat()
        return TokenSecret(**self._request("POST", "/api/tokens", json=body))

    def tokens(self) -> list[TokenMeta]:
        return [TokenMeta(**t) for t in self._request("GET", "/api/tokens")["items"]]

    def revoke_token(self, token_id: str) -> None:
        self._request("DELETE", f"/api/tokens/{token_id}")

    # --- plugins (docs/plugins.md) ---

    def plugins(self) -> dict:
        return self._request("GET", "/api/plugins")

    def run_operator(
        self, plugin: str, operator: str, selection: Selection | dict | None = None, inputs: dict | None = None
    ) -> JobHandle:
        body: dict[str, Any] = {}
        if selection is not None:
            body["selection"] = _dump(selection)
        if inputs is not None:
            body["inputs"] = inputs
        data = self._request("POST", f"/api/plugins/{plugin}/operators/{operator}", json=body)
        return JobHandle(self, data["job_id"])

    # --- views ---

    def view(self, filter: BaseModel | dict | None = None, at_commit: int | None = None) -> "View":
        from .view import View

        return View(self, filter=filter, at_commit=at_commit if at_commit is not None else self.head())


def connect(base_url: str, token: str | None = None) -> Client:
    """The only entry point (docs/sdk.md) — no separate read/write classes, matching a token
    being coarse-grained per docs/api.md#authentication."""
    return Client(base_url, token=token)
