# Energy Path CLI and Python

Read an existing EnergyPlus run without starting another simulation or changing
the open desktop model. The CLI, desktop `LoadEnergyPath` method, local HTTP API
and Python client share the same Go loader, v2 builder and CSV formatter.

## CLI

```powershell
semantic-idf energy-path .\run --input .\run\model.idf --format json
semantic-idf energy-path .\run\eplusout.sql --input .\model.epjson --period M1 --service cooling
semantic-idf energy-path .\run --scope zone --zone Core_bottom --period annual --format csv
semantic-idf energy-path .\run --format csv --include-trace -o .\energy-path-trace.csv
```

Use the packaged executable's actual path if it is not on `PATH`, for example
`.\build\bin\semantic-idf-v0.4.4.exe`. The optional `cli` prefix also works.
Options may appear before or after the result path. Quote paths and Zone names
containing spaces; use `--` before a positional path beginning with `-`.

| Option | Meaning / default |
| --- | --- |
| Result path | Exactly one existing run directory or SQL file. |
| `--input` | IDF/epJSON used for that run; may be omitted only when verified run metadata resolves it. |
| `--scope` | `building` (default) or `zone`. |
| `--zone` | Exact Zone name, required for Zone scope. |
| `--period` | `annual` (default) or `M1` through `M12`. |
| `--service` | `all` (default), `cooling` or `heating`. |
| `--format` | `json` (default) or `csv`. |
| `--include-trace` | Add source/link rows to CSV. JSON already contains provenance. |
| `-o`, `--output` | Optional output file; omitted or `-` writes stdout. |

Input model text and SQLite bytes are not accepted on stdin: both paths are
read by the desktop's file-based builder. Python stdio refers to the CLI's
result stream, not a second model parser.

Output files cannot overwrite the selected SQL, input model or run metadata,
including hardlink/symlink aliases. An explicitly chosen unrelated report file
can still be replaced, as with other CLI exports.

## Result contract

JSON separates the canonical result from its selected view:

- `selection`: normalized Scope, Zone, Period and Service choices.
- `purposeResults`: the unchanged canonical GUI builder payload, including
  `energyExplanation` and `energyExplanationSummary`. Its v2 graph retains the
  original annual/monthly and Building/Zone data and source dictionary.
- `view`: the selected canonical nodes, links, sources, summary, quality,
  reconciliation and warnings. This is a data projection, not browser pixel
  layout or small-category presentation grouping.
- `provenance`: resolved SQL/model paths, the model hash and whether the input
  and historical output plan were verified. This describes the loader's file
  evidence, not another graph schema or a hash of the SQL contents.

A selected month is not relabeled as the canonical graph's annual top level.
An absent month/Zone never silently falls back to Building or Annual. An
explicitly present empty monthly graph remains empty. Selection preserves
source IDs, measured values, units and independent conversion-link quantities.
Source correspondence stays a non-flow relation, not another energy flow.

Cooling/Heating filters keep the original full carrier totals as context; they
do not pretend those meters measured only the chosen service. Quality and
reconciliation refer to the selected scope/period, not a newly invented meter
balance for a service subset. The source dictionary retains full-run metadata;
its annual source scalars are not silently labeled monthly observations.

CSV starts with summary rows for Drivers, Loads, End uses, Carriers, Ratios and
Quality in the selected context. Source/link detail is opt-in; trace preserves
source and normalized units, basis, source references and both link quantities.
JSON is the structured graph representation; CSV is a tabular projection.
Unknown coverage denominators and unavailable accounting percentages remain
blank with an explicit status; they are not exported as measured zeroes.
In particular, a Zone's observed/allocated carrier subtotal does not establish
a reported facility-meter denominator for carrier closure.

## Input identity and unavailable metadata

An explicit SQL path always means that file; a missing or unreadable file cannot
be replaced with a valid sibling. Resolve ambiguous directories by passing an
exact SQL path. Run metadata is used only when it belongs to the selected output.
SQLite is opened read-only. The loader does not apply outputs, write temporary
models, run discovery, execute EnergyPlus or update run files.

SQLite WAL mode and shared-memory/journal sidecars are rejected with a message
to finish the writer and provide a checkpointed, non-WAL database snapshot.
Closing a WAL-mode connection alone does not change its persistent journal mode.
Reading cannot discard pending observations or create shared-memory artifacts.

SemanticIDF records the hash of the actual run-copy model, including temporary
output requests. If a manifest hash does not match `--input`, use that run-copy
model, not an original with different output requests. A model that changes
during loading is rejected. An external SQL/model pair without verified run
metadata cannot claim verified provenance or known requested-output coverage:
the graph keeps observed data and reports the uncertainty. The loader does not
invent a historical output plan from the current model.

## Local HTTP API

`POST /api/energy-path` accepts one JSON object. Paths refer to files on the API
host, not an uploaded file or the caller's remote filesystem.

```json
{
  "resultPath": "C:/runs/office/eplusout.sql",
  "inputPath": "C:/runs/office/model.idf",
  "scope": "zone",
  "zone": "Core_bottom",
  "period": "M1",
  "service": "cooling",
  "format": "json",
  "includeTrace": false
}
```

The CLI defaults also apply here. `format: "csv"` returns UTF-8 CSV; JSON returns
the entire shared projection unchanged. Unknown fields, invalid choices/paths,
multiple JSON objects and bodies over 64 KiB return errors. This is the existing
local app API, not a public file-serving service.

## Python: API or CLI stdout

The checked-in `clients/python/semantic_idf_client.py` needs only Python's
standard library. Make that directory importable, for example with `PYTHONPATH`:

```python
from semantic_idf_client import SemanticIDFClient

client = SemanticIDFClient("http://127.0.0.1:34115")
result = client.energy_path(
    "C:/runs/office", input_path="C:/runs/office/model.idf",
    scope="zone", zone="Core_bottom", period="M1", service="cooling",
)

# No running HTTP server is needed for the CLI transport.
same_result = SemanticIDFClient.energy_path_stdio(
    "C:/tools/semantic-idf.exe", "C:/runs/office",
    input_path="C:/runs/office/model.idf",
    scope="zone", zone="Core_bottom", period="M1", service="cooling",
)

csv_text = client.energy_path("C:/runs/office", output_format="csv", include_trace=True)
```

Both methods return a dictionary for JSON and the exact decoded CSV stream for
CSV. Stdio passes an argument list without a shell; CLI failures raise
`subprocess.CalledProcessError` with captured stderr. The API uses normal
`urllib` HTTP errors. Both accept `timeout` in seconds, default 120. Neither
client calculates energy aggregates, migrates v1 data or infers allocation.

See [Energy Path schema](energy-path-schema.md) for domains, allocation,
quality, source correspondence and the separate JSON reconstruction example.
