# IDF Analyzer

Lightweight desktop tooling for EnergyPlus IDF files, built with Go and Wails using a static HTML/CSS/JS frontend.

## Current Scope

- Parse IDF objects and field comments.
- Detect EnergyPlus input format from extension or content.
- Support EnergyPlus 22+ inputs through version detection from `Version`.
- Parse and write both IDF and epJSON input text.
- View input as editable text, structured JSON, or a spreadsheet-style field table.
- Summarize object types, schedules, zones, unused named objects, and simple HVAC node connections.
- Jump from summary, schedule, unused, zone, and system analysis items to the matching object in the active input view.
- Edit field values, diagnose common modeling issues, and run cleanup workflows through the Go API.
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

The app toolbar includes top-level Tools, Guide, and Settings navigation buttons that open bundled full-page views inside the Wails WebView. Keep `frontend/dist/guide.html` focused on end-user workflows; developer commands and repository maintenance notes belong in this README or `docs/agent.md`.

## Input Views

- A shared input filter applies across Text, JSON, and Table views by matching object type, name/index, field label, and value text.
- Text: fully expanded editable object summary first, shared editable raw source below, and optional position sync from editable fields and analysis selections.
- JSON: structured epJSON-like editor first, with read-only syntax tokens and inline-editable value tokens that patch the backend model; raw source uses the shared Raw Text pane.
- Table: fully expanded IDF object type tables with fixed row headers, no synthetic Name column, global and per-table row/column orientation controls, and shared raw source sync.
- Workspace: resizable input and analysis panes with separate scroll areas, no window-level app scrolling, and vertical splitters for Raw Text and Geometry details.

## Analysis Navigation

- The right panel has Summary, Diagnose, and Geometry result tabs.
- Summary shows a metric catalog grouped by model, geometry, envelope, loads, schedules, and HVAC categories.
- Summary can be filtered and exported as categorized JSON or a two-column `name,value` CSV whose names are variable IDs with units in brackets, including `[-]` for unitless values.
- Diagnose reports error/warning issues such as missing references, duplicate names, orphan resources, required-object gaps, geometry problems, schedule-hour limits, and HVAC node graph hints.
- Geometry parses detailed zones, walls, roofs, floors, and fenestration into a 3D view that defaults to all levels, optional story filtering, a story-by-story plan view, selectable metrics, related object links, and Sync locate jumps to the matching input object.
- Summary metric guide entries are loaded from the same backend catalog as the calculated metrics.
- The startup sample is the official EnergyPlus `RefBldgLargeOfficeNew2004_Chicago.idf` example vendored under `frontend/dist/samples/`.
- The startup sample text is shown first; analysis then runs in visible-first stages so Summary/Text render before Diagnose and Geometry finish in the background.
- Open uses the desktop file dialog, Save writes the current text back to the opened file or asks for a path, and Revert restores the text from the last opened input snapshot.
- Analysis runs automatically after file open and after debounced editor changes; larger workflows belong under Tools.
- Tools includes Multi-IDF Summary, which opens several EnergyPlus inputs, analyzes them concurrently, displays progress, compares Summary metrics in a transposable table, and exports CSV in the selected table direction.
- Tools includes Cleanup Wizard, which scans a file, shows cleanup candidates, lets users choose rules, previews removals, and applies or exports the cleaned copy.
- Settings are stored under the local app data/config directory and currently expose only the page frame for future options.

## Project Layout

- `internal/idf`: IDF parsing, analysis, and editing core.
- `internal/epinput`: EnergyPlus input format detection, version detection, common model, and IDF/epJSON conversion.
- `frontend/dist`: tracked static frontend assets.
- `frontend/dist/app.js`: tiny ES module entrypoint.
- `frontend/dist/js`: frontend modules split by state, actions, input views, analysis views, navigation, layout, and sample data.
- `frontend/dist/guide.html`: user-facing tool guide maintained cumulatively.
- `frontend/dist/settings.html`: settings page frame backed by the local settings JSON API.
- `docs/agent.md`: consolidated working notes and implementation principles.
- `app.go`: Wails-bound application API.
- `scripts`: repo-local runtime setup, checks, and repeatable commands.
- `.runtime`: ignored local Go/Wails runtime and caches created by setup.

## EnergyPlus References

The parser currently supports EnergyPlus version 22 or newer when a `Version` object is present. Full IDD/schema validation is intentionally separated from parsing: version-specific files can be added later under `resources/energyplus/<major>.<minor>/Energy+.idd` and `resources/energyplus/<major>.<minor>/Energy+.schema.epJSON`.

The epJSON path is being aligned with the official schema shape. Detailed surface and shading vertex fields are converted to a `vertices` array of coordinate objects; broader schema-aware numeric typing and extensible-field support should continue to grow from the official EnergyPlus schema references.
