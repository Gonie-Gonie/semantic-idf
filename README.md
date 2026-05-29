# IDF Analyzer

Lightweight desktop tooling for EnergyPlus IDF files, built with Go and Wails using a static HTML/CSS/JS frontend.

## Current Scope

- Parse IDF objects and field comments.
- Detect EnergyPlus input format from extension or content.
- Support EnergyPlus 22+ inputs through version detection from `Version`.
- Parse and write both IDF and epJSON input text.
- Convert IDF to epJSON and epJSON/JSON to IDF.
- View input as editable text, structured JSON, or a spreadsheet-style field table.
- Summarize object types, schedules, zones, unused named objects, and simple HVAC node connections.
- Jump from summary, schedule, unused, zone, and system analysis items to the matching object in the active input view.
- Edit field values and remove unused named objects through the Go API.
- Run the frontend without a Node/npm build chain.

## Requirements

- PowerShell.
- Internet access for the first setup.
- Platform webview runtime required by Wails.

The Go runtime and Wails CLI are installed into `.runtime/` by setup. That directory is local to each clone and is ignored by git.

Default setup versions:

- Go 1.24.5
- Wails CLI v2.12.0

## Commands

Use the top-level batch wrapper on Windows. From PowerShell, prefix it with `.\`; from `cmd.exe`, `dev setup` also works.

```bat
.\dev.bat setup
.\dev.bat check
.\dev.bat test
.\dev.bat run
.\dev.bat build
.\dev.bat verify
.\dev.bat guide
```

The wrapper calls PowerShell with `-NoProfile -ExecutionPolicy Bypass` and forwards to scripts under `scripts/`.

Direct PowerShell commands are also available:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\setup.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\check-env.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\run.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\package.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

`scripts/setup.ps1` installs the repo-local runtime and a pre-commit hook. The hook runs `scripts/verify.ps1`, which performs whitespace checks, `go test ./...`, and `wails build` using `.runtime/`.

Build artifacts and downloaded runtimes stay ignored by git.

## User Guide

The app toolbar includes a Guide button that navigates to the bundled `frontend/dist/guide.html` document inside the Wails WebView. Keep that guide cumulative: whenever a workflow, button, limitation, or developer command changes, update the relevant section and append an entry to its update log.

## Input Views

- Text: fully expanded editable object summary first, shared editable raw source below, and optional action-based position sync between them.
- JSON: structured epJSON-like editor first, with read-only syntax tokens and inline-editable value tokens that patch the backend model; raw source uses the shared Raw Text pane.
- Table: fully expanded IDF object type tables with fixed row headers, no synthetic Name column, global and per-table row/column orientation controls, and shared raw source sync.
- Workspace: resizable input and analysis panes with separate scroll areas and no window-level app scrolling.

## Analysis Navigation

- Result tabs are Summary, Schedules, and Systems.
- Summary zone cards expose surface and related-object lists.
- Clickable analysis rows and system paths locate and highlight the matching object inside the left input pane without moving the analysis pane.
- Conversion and cleanup commands are grouped under the top toolbar Tools menu.

## Project Layout

- `internal/idf`: IDF parsing, analysis, and editing core.
- `internal/epinput`: EnergyPlus input format detection, version detection, common model, and IDF/epJSON conversion.
- `frontend/dist`: tracked static frontend assets.
- `frontend/dist/app.js`: tiny ES module entrypoint.
- `frontend/dist/js`: frontend modules split by state, actions, input views, analysis views, navigation, layout, and sample data.
- `frontend/dist/guide.html`: user-facing tool guide maintained cumulatively.
- `docs/agent.md`: consolidated working notes and implementation principles.
- `app.go`: Wails-bound application API.
- `scripts`: repo-local runtime setup, checks, and repeatable commands.
- `.runtime`: ignored local Go/Wails runtime and caches created by setup.

## EnergyPlus References

The parser currently supports EnergyPlus version 22 or newer when a `Version` object is present. Full IDD/schema validation is intentionally separated from parsing: version-specific files can be added later under `resources/energyplus/<major>.<minor>/Energy+.idd` and `resources/energyplus/<major>.<minor>/Energy+.schema.epJSON`.

The epJSON path is being aligned with the official schema shape. Detailed surface and shading vertex fields are converted to a `vertices` array of coordinate objects; broader schema-aware numeric typing and extensible-field support should continue to grow from the official EnergyPlus schema references.
