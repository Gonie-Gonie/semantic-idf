"""Minimal standard-library client for SemanticIDF's topology RPC."""

from __future__ import annotations

import json
from typing import Any
from urllib.request import Request, urlopen


class SemanticIDFClient:
    def __init__(self, base_url: str = "http://127.0.0.1:34115") -> None:
        self.base_url = base_url.rstrip("/")

    def topology(
        self,
        text: str,
        *,
        level: str = "zone",
        metric: str = "topology",
        scope: str = "building",
        area_basis: str = "effective",
        output_format: str = "json",
        story_index: int | None = None,
        selected_entity_id: str = "",
        neighbor_depth: int = 1,
    ) -> dict[str, Any] | str:
        options: dict[str, Any] = {
            "level": level,
            "metric": metric,
            "scope": scope,
            "areaBasis": area_basis,
            "selectedEntityId": selected_entity_id,
            "neighborDepth": neighbor_depth,
        }
        if story_index is not None:
            options["storyIndex"] = story_index
        request = Request(
            f"{self.base_url}/api/topology",
            data=json.dumps({"text": text, "format": output_format, "options": options}).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urlopen(request) as response:
            payload = response.read().decode("utf-8")
        return json.loads(payload) if output_format == "json" else payload
