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

- Text: formatted object summary first, editable raw source below.
- JSON: structured model tree first, editable epJSON text below.
- Table: IDF object type tables with global and per-table row/column orientation controls.
- Workspace: resizable input and analysis panes with separate scroll areas.

## Project Layout

- `internal/idf`: IDF parsing, analysis, and editing core.
- `internal/epinput`: EnergyPlus input format detection, version detection, common model, and IDF/epJSON conversion.
- `frontend/dist`: tracked static frontend assets.
- `frontend/dist/guide.html`: user-facing tool guide maintained cumulatively.
- `app.go`: Wails-bound application API.
- `scripts`: repo-local runtime setup, checks, and repeatable commands.
- `.runtime`: ignored local Go/Wails runtime and caches created by setup.

## EnergyPlus References

The parser currently supports EnergyPlus version 22 or newer when a `Version` object is present. Full IDD/schema validation is intentionally separated from parsing: version-specific files can be added later under `resources/energyplus/<major>.<minor>/Energy+.idd` and `resources/energyplus/<major>.<minor>/Energy+.schema.epJSON`.
