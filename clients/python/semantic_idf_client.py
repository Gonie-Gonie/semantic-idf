"""Standard-library clients for SemanticIDF topology and Energy Path results."""

from __future__ import annotations

import json
import os
import subprocess
from collections.abc import Sequence
from typing import Any
from urllib.request import Request, urlopen


class SemanticIDFClient:
    def __init__(self, base_url: str = "http://127.0.0.1:34115") -> None:
        self.base_url = base_url.rstrip("/")

    def energy_path(
        self,
        result_path: str | os.PathLike[str],
        *,
        input_path: str | os.PathLike[str] | None = None,
        scope: str = "building",
        zone: str = "",
        period: str = "annual",
        service: str = "all",
        output_format: str = "json",
        include_trace: bool = False,
        timeout: float = 120,
    ) -> dict[str, Any] | str:
        """Read existing SQL via the local API; paths belong to the API host.

        The returned ``purposeResults`` is the desktop's canonical v2 payload.
        ``view`` contains the requested scope/period/service projection. CSV
        defaults to summary rows; include_trace adds source and link rows.
        This operation never starts a simulation or changes the open model.
        """
        output_format = output_format.strip().lower()
        payload: dict[str, Any] = {
            "resultPath": os.fspath(result_path),
            "scope": scope,
            "zone": zone,
            "period": period,
            "service": service,
            "format": output_format,
            "includeTrace": include_trace,
        }
        if input_path is not None:
            payload["inputPath"] = os.fspath(input_path)
        request = Request(
            f"{self.base_url}/api/energy-path",
            data=json.dumps(payload, allow_nan=False).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urlopen(request, timeout=timeout) as response:
            result = response.read().decode("utf-8")
        return json.loads(result) if output_format in ("", "json") else result

    @staticmethod
    def energy_path_stdio(
        executable: str | os.PathLike[str] | Sequence[str],
        result_path: str | os.PathLike[str],
        *,
        input_path: str | os.PathLike[str] | None = None,
        scope: str = "building",
        zone: str = "",
        period: str = "annual",
        service: str = "all",
        output_format: str = "json",
        include_trace: bool = False,
        timeout: float = 120,
    ) -> dict[str, Any] | str:
        """Read the identical result through CLI stdout, without a web server.

        ``executable`` is the SemanticIDF executable path or a command-prefix
        sequence. Arguments never pass through a shell. Input is a model path,
        not model text on stdin, because the shared builder reads that file.
        CLI errors raise CalledProcessError with captured stderr.
        """
        output_format = output_format.strip().lower() or "json"
        command = (
            [os.fspath(executable)]
            if isinstance(executable, (str, os.PathLike))
            else list(executable)
        )
        if not command:
            raise ValueError("executable must not be empty")
        command.extend([
            "energy-path", "--scope", scope, "--period", period,
            "--service", service, "--format", output_format,
        ])
        if input_path is not None:
            command.extend(["--input", os.fspath(input_path)])
        if zone:
            command.extend(["--zone", zone])
        if include_trace:
            command.append("--include-trace")
        command.extend(["--", os.fspath(result_path)])
        completed = subprocess.run(
            command, stdin=subprocess.DEVNULL, capture_output=True,
            check=True, timeout=timeout, shell=False,
        )
        # Decode bytes directly: text=True would normalize CSV CRLF differently
        # from the HTTP transport and no longer return the identical CSV stream.
        result = completed.stdout.decode("utf-8")
        return json.loads(result) if output_format == "json" else result

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
