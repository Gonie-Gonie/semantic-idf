# CLI, API and exports

The executable supports both desktop use and non-interactive commands. All examples below use the generic name semantic-idf; substitute the path/name of your packaged executable, including its version suffix if present.

## Invocation and streams {#invocation}

Without arguments, the executable starts the desktop app. Use either an explicit cli prefix or a recognized top-level command for batch processing.

```text
semantic-idf cli --help
semantic-idf cli metrics --help
semantic-idf version
semantic-idf cli metrics -format json -o metrics.json model.idf
```

Put options before input operands for model commands. Quote paths and object names containing spaces according to your shell. A successful command returns exit code 0; invalid arguments or failed processing return 1 and an Error message on stderr. Help is successful.

For supported model commands, input `-` reads stdin and output `-o -` writes stdout. XLSX output on stdout is binary; redirect it with a binary-safe tool rather than a shell pipeline that decodes/re-encodes text. Energy Path requires filesystem model/result paths.

## Static analysis commands {#analysis-commands}

| Command | Output formats | Purpose |
| --- | --- | --- |
| metrics | text, json, csv, xlsx | Shared static metric definitions and values |
| batch-metrics | csv, json, text, xlsx | Several independent models in one comparison |
| diagnostics | text, json, csv | Diagnose issues |
| analyze | json, text | Combined analysis report |
| topology | json, graphml, dot | Canonical static thermal-network projection |
| hvac-graph | json, text | HVAC rule, service or coupling navigation |
| profile-graph | json, text | Resolved Profile graph series |
| profile-qa | text, json, csv | Profile QA issues and candidates |
| profile-schedules | json, text, csv | Resolved schedules and similarity clusters |

```text
semantic-idf cli metrics -format csv -o metrics.csv model.idf
semantic-idf cli batch-metrics -format xlsx -orientation files -o compare.xlsx a.idf b.epjson
semantic-idf cli diagnostics -format json -o issues.json model.idf
semantic-idf cli analyze -format json -o analysis.json model.idf
semantic-idf cli profile-schedules -format csv -o schedules.csv model.idf
semantic-idf cli hvac-graph -graph service -format json -o service.json model.idf
```

Batch orientation is metrics or files. HVAC graph is rule, service or coupling. A graph export represents supported model connectivity and evidence; it does not execute the systems.

Legacy command aliases include summary, multi-summary/multi-metrics and diagnose. New automation should use the current explicit command names to make intent clear.

## Topology projection options {#topology-export}

| Option | Choices / default |
| --- | --- |
| -level | zone or boundary; default zone |
| -metric | topology, area, ua, exposure, qa, air; default topology |
| -scope | building, story, selection, neighbors; default building |
| -area-basis | effective or physical; default effective |
| -story | Zero-based story index for story scope |
| -selection | Stable entity ID for selection/neighbors |
| -neighbor-depth | 1–3; default 1 |
| -format | json, graphml or dot; default json |

```text
semantic-idf cli topology -metric ua -area-basis physical -format graphml -o thermal.graphml model.idf
semantic-idf cli topology -scope story -story 0 -format json -o first-story.json model.idf
```

Use exported stable entity IDs for selection; do not substitute a display label without checking identity. See [Topology](./topology.en.md) for area/UA, canonical reciprocal pairs and unsupported values.

## Clean and convert {#clean-convert}

Clean can preview without applying changes. Rules accept default, all, none, or comma-separated rule IDs. Exclusions are candidate keys, not arbitrary object names.

```text
semantic-idf cli clean --dry-run -format json -o preview.json model.idf
semantic-idf cli clean -rules remove_unused_schedules,remove_duplicate_output_variables -o cleaned.idf model.idf
semantic-idf cli clean -rules none --compact -o compact.idf model.idf
semantic-idf cli clean -rules none --semantic-duplicates -o renamed.idf model.idf
semantic-idf cli convert -to idf -o model.idf model.epjson
semantic-idf cli convert -to json -o model.epjson model.idf
semantic-idf cli convert -to yaml -o model.semantic.yaml model.idf
semantic-idf cli convert -to table -o model.tables.xlsx model.idf
```

Semantic YAML is a view export, not a promised lossless replacement for an EnergyPlus execution input. The table/XLSX conversion groups objects in a styled worksheet. Choosing the JSON input tab in the app does not itself perform either file conversion.

Cleanup defaults, optional resource rules and unavailable future rules are explained in [Tools](./tools.en.md#cleanup-rules). Keep the input and proposed destination distinct when reviewing changes.

## Read existing Energy Path results {#energy-path-cli}

Energy Path reads an existing run directory or SQL file. It neither launches EnergyPlus nor edits the open model.

```text
semantic-idf energy-path -format json -o result.json C:/runs/office
semantic-idf energy-path -input C:/models/office.idf -scope zone -zone "Core_bottom" -period M1 -service cooling -format csv -include-trace -o january.csv C:/runs/office/eplusout.sql
```

| Option | Meaning |
| --- | --- |
| -input | Matching IDF/epJSON path; if omitted, use a verifiable retained run input |
| -scope | building (default) or zone |
| -zone | Exact Zone name when scope is zone |
| -period | annual (default), M1 through M12 |
| -service | all (default), cooling, heating; API/CLI option even though current UI displays both |
| -format | json (default) or csv |
| -include-trace | Append source/link trace rows to CSV; JSON remains complete |
| -o / -output | Destination or stdout with - |

Known registered Energy Path options may appear around its positional result path; this does not change the established option ordering of other commands. Use -- when needed to terminate options.

The source model must match the retained SQL/run provenance. A nearby, current or similarly named editor input is not automatically equivalent. Protected SQL, input and run metadata cannot be overwritten by the export destination.

## JSON, CSV, XLSX and HTML {#export-meaning}

| Representation | Appropriate use | Interpretation |
| --- | --- | --- |
| Metrics JSON | Programmatic categorized analysis | Preserves grouped metric metadata and statuses |
| Metrics CSV | Spreadsheet or simple scripts | name,value rows; variable-style names include [unit], or [-] |
| XLSX | Human review and comparison tables | Formatted workbook; inspect sheet meaning and status alongside numbers |
| Energy Path JSON | Structured shared result plus selected view | Retains full purposeResults and provenance rather than only visible ribbons |
| Energy Path summary CSV | Scope/period/service tabulation | Selected summary; optional trace rows add original sources/links |
| Energy Path HTML/XLSX report | Shareable human report | Selected all-service presentation; trace details follow export controls |
| Topology GraphML/DOT | Graph tools | Static projection, not a simulated flow ledger |

A complete JSON result can contain periods and Zones beyond the currently displayed selection. CSV is not a universal lossless replacement for that graph. The meaning of a formatted value depends on its unit, status, source and denominator.

## Local HTTP API {#http-api}

When the app's local API is reachable, endpoints operate on the API host's files. They do not upload your caller's SQL file automatically. Configure your client with the actual reachable base URL; an example port is not proof that a server is listening.

POST /api/energy-path accepts one JSON object:

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

JSON returns the shared projection. format=csv returns UTF-8 CSV. Invalid choices, unknown fields, multiple objects and bodies over 64 KiB fail. GET /api/metric-guides returns the same metric catalog used by the app. POST /api/topology projects supplied model text according to supported options.

Other app endpoints include settings, run planning, output discovery, simulation execution and batch operations. They are different operations with their own request shapes; do not treat a result-read endpoint as a run command.

## Python and repeatable comparisons {#python}

The standard-library SemanticIDFClient supports HTTP and Energy Path CLI stdout. Make the distributed clients/python module importable. CLI stdout is useful when no local HTTP API is available.

```python
from semantic_idf_client import SemanticIDFClient

result = SemanticIDFClient.energy_path_stdio(
    "C:/tools/semantic-idf.exe", "C:/runs/office",
    input_path="C:/runs/office/model.idf",
    scope="zone", zone="Core_bottom", period="M1",
)
print(result["view"]["summary"])
```

Neither transport recomputes allocations in Python; both consume the shared builder's result. Keep executable version, original/executed inputs, SQL, engine/weather and selectors with a comparison. Compare corresponding units and observed values, preserving missing/null/zero distinctions. A parseable export alone does not certify the physical model.

## Repository cleanup {#repository-cleanup}

These commands run from a source checkout on Windows. From PowerShell, use `.\dev.bat`; from Command Prompt, `dev` also works. Repository cleanup manages development artifacts; the executable's `cli clean` command above edits a model.

```text
.\dev.bat clean-repo -WhatIf
.\dev.bat clean-repo
.\dev.bat clean-repo --hard -WhatIf
.\dev.bat clean-repo --hard
.\dev.bat setup
```

Use `-WhatIf` to inspect the cleanup plan without deleting files. Normal `clean-repo` removes the Go build cache, generated frontend bindings/build files and known temporary development artifacts. It keeps `build/bin`, the installed Go/Wails tools, dependency caches, local simulation evidence and reusable baselines, so setup is not required again. Subsequent builds and tests recreate removed artifacts as needed.

`clean-repo --hard` also removes `build/bin`, `.runtime/` and `.tools/`, returning installed tooling and generated files to a fresh-clone state. Everything kept under `.runtime/`, including local simulation captures and baselines, is removed; copy anything you want to retain outside that directory first. Run `dev setup` before the next build or test.

Both modes preserve tracked source files, maintained build icons, Git metadata/hooks, and model/settings files outside the explicit generated paths. A directory junction or symbolic link does not permit cleanup to reach files outside the checkout.
