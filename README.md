# SemanticIDF

Lightweight desktop tooling for EnergyPlus IDF files, built with Go and Wails using a static HTML/CSS/JS frontend.

## Current Scope

- Parse IDF objects and field comments.
- Detect EnergyPlus input format from extension or content.
- Support EnergyPlus 22+ inputs through version detection from `Version`.
- Parse and write both IDF and epJSON input text.
- View input as editable text, structured JSON, or a spreadsheet-style field table.
- Summarize object types, schedules, zones, unused named objects, and simple HVAC node connections.
- Jump from metrics, schedule, unused, zone, and system analysis items to the matching object in the active input view.
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

Use the top-level `dev.bat` wrapper on Windows. From PowerShell, prefix it with `.\`; from `cmd.exe`, `dev setup` also works.

```bat
.\dev.bat setup
.\dev.bat check
.\dev.bat test
.\dev.bat run
.\dev.bat build
.\dev.bat verify
.\dev.bat release
.\dev.bat guide
.\dev.bat clean-repo
.\dev.bat clean-repo --hard
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
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\release.ps1
```

`scripts/setup.ps1` installs the repo-local runtime and a pre-commit hook. The hook runs
`scripts/verify.ps1 -Staged`, which checks whitespace, runs the fast tests plus browser
and regression tests for the affected features, validates frontend assets, and builds
the Wails executable using `.runtime/`.

Choose the test scope through the same runner throughout development:

```powershell
.\dev.bat test -Plan           # Inspect the plan for changed files without running tests.
.\dev.bat test                 # Fast tests plus all tiers for affected features.
.\dev.bat test -Area profile   # All Profile tests, including browser/regression checks.
.\dev.bat test -Quick          # Only the fast baseline across all features.
.\dev.bat test -Full           # Every Go regression test.
.\dev.bat test -List           # Current feature and tier counts.
.\dev.bat verify               # Selected tests, whitespace/frontend checks and Wails build.
.\dev.bat verify -Full         # Full regression suite and Wails build.
```

Tests stay beside their existing packages. The runner groups them by feature and layer;
shared core, toolchain, and unclassified source changes select the full suite. Release
packaging always uses `verify -Full`. See [Testing workflow](docs/testing.md) for scope
selection, catalog maintenance and commit checks.

Build artifacts and downloaded runtimes stay ignored by git.

### Repository cleanup

`dev clean-repo` removes generated frontend bindings/build files, build/package
intermediates, Go test binaries, coverage files, Go build cache and temporary
Guide review captures. It preserves `build/bin`, the installed Go/Wails tools,
downloaded dependencies, local simulation evidence and reusable comparison
baselines. Development can continue without another setup.

`dev clean-repo --hard` also removes `build/bin`, `.runtime/` and `.tools/`,
resetting installed tooling and generated files to a fresh-clone state. Local
captures and baselines inside `.runtime/` are included; move any you want to
retain outside that directory first. Run `dev setup` before the next build or
test. Optional root/frontend `node_modules` are removed in both modes.

Both modes preserve Git-tracked or staged files, maintained build icons, Git
metadata/hooks and models/settings outside the named generated locations. They
refuse cleanup through symbolic links or junctions. Use `dev clean-repo -WhatIf`
or `dev clean-repo --hard -WhatIf` to preview the complete plan without deleting
anything. Cleanup works without the repo-local Go/Wails runtime.

## CLI

The packaged executable opens the desktop app when run without arguments. It also supports scriptable commands through
`semantic-idf cli ...`; recognized commands can also be used directly as `semantic-idf metrics ...`.

```powershell
# Metrics / diagnostics / full analysis
.\build\bin\semantic-idf-v0.5.0.exe cli metrics -format text .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli metrics -format json -o .\metrics.json .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli metrics -format xlsx -o .\metrics.xlsx .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli diagnostics -format csv -o .\diagnostics.csv .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli analyze -format json -o .\report.json .\model.idf

# Batch metrics
.\build\bin\semantic-idf-v0.5.0.exe cli batch-metrics -format csv -o .\compare.csv .\a.idf .\b.epjson
.\build\bin\semantic-idf-v0.5.0.exe cli batch-metrics -format xlsx -orientation files -o .\compare.xlsx .\a.idf .\b.idf

# Existing simulation Energy Path (read-only; no new simulation)
.\build\bin\semantic-idf-v0.5.0.exe energy-path .\run --input .\run\model.idf --format json
.\build\bin\semantic-idf-v0.5.0.exe energy-path .\run --scope zone --zone Core_bottom --format csv --include-trace

# Cleanup
.\build\bin\semantic-idf-v0.5.0.exe cli clean --dry-run .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli clean -rules all --compact -o .\cleaned.idf .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli clean -rules none --semantic-duplicates -o .\semantic-fixed.idf .\model.idf

# Conversion
.\build\bin\semantic-idf-v0.5.0.exe cli convert -to idf -o .\model.idf .\model.epjson
.\build\bin\semantic-idf-v0.5.0.exe cli convert -to json -o .\model.epjson .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli convert -to semantic-yaml -o .\model.semantic.yaml .\model.idf
.\build\bin\semantic-idf-v0.5.0.exe cli convert -to table -o .\model.tables.xlsx .\model.idf
```

`convert -to semantic-yaml` writes a one-way semantic YAML projection for inspection and token editing. `-to yaml`
remains an alias, but YAML-to-IDF reverse conversion is planned separately and this export should not be treated as a
standalone round-trip source.
The table conversion writes one XLSX worksheet with `[ObjectType]` section markers. Column headers are bold with a fill
color and table cells carry borders so the export is easier to scan and filter in Excel. Use `-` as an input path to
read stdin and `-o -` to write command output to stdout.

`energy-path` requires existing SQL and model paths, not stdin model text. See
[Energy Path CLI and Python](docs/energy-path.md#cli-and-python) for scope/month/service
selection, input provenance, summary/trace CSV and the identical local API result.

## Release Process

Release timing is manual. Update `docs/release-notes/unreleased.md`, then create and push a `vX.Y.Z` tag when you want to publish. The tag push runs the GitHub Actions `Release` workflow, which builds a versioned executable from that tag and publishes the GitHub Release.

Use `scripts/release.ps1` to keep the release metadata and tag together. It updates `cmd/semantic-idf/wails.json`, writes `CHANGELOG.md`, snapshots the release notes under `docs/release-notes/vX.Y.Z.md`, builds a versioned executable, creates the release commit, creates the `vX.Y.Z` tag, and pushes both. The pushed tag then publishes the GitHub Release.

The GitHub Actions `Release` workflow can still be run manually from GitHub when you want the workflow to do the prepare, tag, push, and publish steps in one run.

The script chooses the semver bump from release-note sections when no explicit version is provided:

- `Breaking Changes` or `BREAKING CHANGE`: major.
- `Added` or `Features`: minor.
- `Fixed`, `Changed`, `Performance`, `Security`, documentation-only, or internal-only notes: patch.

For the first release, if no `v*` tag exists and `cmd/semantic-idf/wails.json` already has a product version, `auto` releases that current version. Later automatic bumps use the latest release tag as their base, so rerunning a prepared release does not increment the version twice.

Useful local release commands:

```powershell
# Prepare metadata only; leaves unreleased notes in place for review.
.\dev.bat release -KeepUnreleased

# Prepare, verify, build, commit, tag, and push.
# The pushed tag publishes the GitHub Release.
.\dev.bat release -Package -Commit -Tag -Push

# Fallback: publish directly through GitHub CLI.
.\dev.bat release -Package -Commit -Tag -Push -Publish
```

Release packaging requires the full Go test suite and a successful Wails build through
`scripts/verify.ps1 -Full`, both locally and in the tag/manual release workflow.

The app version is shown in the window title, page headers, Settings storage details, release asset names, and the built executable filename.

## Technical Reference Manual

The app toolbar includes top-level Tools, Guide, and Settings pages in the Wails WebView. The app supports English, Korean, Japanese, Hindi, Spanish and French, with English fallback for untranslated text. Guide follows the app language: Korean uses the Korean reference, and every other language uses English. It reads from the single [manual source directory](cmd/semantic-idf/frontend/src/manual/README.md), with chapter/section navigation, search and shareable section links. The Markdown chapters cover workflows, algorithms, units, assumptions, examples and troubleshooting; `guide.html` contains the page shell. Metric definitions use the backend registry with a generated offline fallback and a Korean description overlay. Keep repository maintenance instructions in this README or `docs/agent.md`.

## Input Views

- Text, JSON, and Table are peer tabs styled consistently with the analysis tabs; Text is the default and the former Input View heading and line counter are not shown.
- The Semantic structure projection and renderer remain available internally for cross-view identity and a lightweight rollback, but the Semantic tab, shortcuts, settings entry, and saved-view restoration are not exposed in v0.5.0.
- A shared input filter applies across Text, JSON, and Table views by matching object type, name/index, field label, and value text.
- Text: fully expanded editable object and field summaries that update the saved source document directly.
- JSON: a structured epJSON-like editor with read-only syntax tokens and inline-editable value tokens that patch the shared document model.
- Table: fully expanded IDF object type tables with fixed row headers, no synthetic Name column, and global and per-table row/column orientation controls.
- Workspace: resizable input and analysis panes with separate scroll areas, no window-level app scrolling, and a vertical splitter for Topology details.

## Analysis Navigation

- The right panel has Metrics, Topology, Profile, HVAC, and Simulation result tabs.
- Metrics shows a catalog grouped by model, topology, envelope, loads, schedules, and HVAC categories.
- Metrics can be exported as categorized JSON or a two-column `name,value` CSV whose names are variable IDs with units in brackets, including `[-]` for unitless values.
- Tools / Diagnose reports error and warning issues and reviews safe cleanup fixes for the current input snapshot or a separately selected input.
- The former main Output tab and Batch Output QA tool are no longer exposed. Output-request analysis and edits remain available to backend and automation callers through `AnalyzeInputOutputText`, `PreviewOutputApplyText`, `ApplyOutputText`, and `ApplyPurposeOutputsText`.
- Topology parses detailed zones, walls, roofs, floors, and fenestration into 3D, Plan, and Network views. Its compact toolbar keeps the view and Level selectors on one row, then shows only Zones/Surfaces/Openings for 3D and Plan or Metric/Layout for Network. All three views show selected-object properties in the same lower details panel. Selecting an object automatically links to its input object, emphasizes its one-hop connections, and strongly fades unrelated objects.
- Metric guide entries are loaded from the same backend catalog as the calculated metrics.
- The startup sample is the official EnergyPlus `RefBldgLargeOfficeNew2004_Chicago.idf` example vendored under `cmd/semantic-idf/frontend/src/samples/`.
- The startup sample text is shown first; analysis then runs in visible-first stages so Metrics/Text render before Topology is prepared in the background.
- Open uses the desktop file dialog, Save writes the current text back to the opened file or asks for a path, and Revert restores the text from the last opened input snapshot.
- Analysis runs automatically after file open, and structured field edits refresh the shared analysis directly; larger workflows belong under Tools.
- Tools includes Batch Metrics, which opens several EnergyPlus inputs, analyzes them concurrently, displays progress, compares model metrics in a transposable table, and exports CSV or XLSX results.
- Batch Simulation uses fixed purpose defaults for output application, frequency, detail, allocation, period, and scope. It automatically resolves a compatible registered or detected EnergyPlus installation for each input file, while retaining purpose, weather, recursion, and worker controls.
- Tools contains Batch Metrics, Batch Simulation, and Diagnose. Diagnose lets users choose cleanup rules, filter and include/exclude individual candidates, apply fixes back to the app snapshot, or save a cleaned copy.
- Settings are stored under the local app data/config directory and cover appearance and topology colors, EnergyPlus/weather defaults, keyboard shortcuts, Profile analysis preferences, and storage actions.

## Project Layout

See the [documentation index](docs/README.md) for architecture, contracts,
acceptance evidence and performance notes.

- `cmd/semantic-idf`: complete desktop/CLI application subtree, including its frontend and internal packages.
- `cmd/semantic-idf/app.go`: desktop lifecycle and input/file operations; feature-specific Wails methods live in `analysis_app.go`, `batch_metrics_app.go`, `settings_app.go` and the existing simulation/Energy Path modules.
- `cmd/semantic-idf/wails.json`: Wails project and release metadata.
- `cmd/semantic-idf/internal/idf`: IDF parsing, analysis, and editing core.
- `cmd/semantic-idf/internal/epinput`: EnergyPlus input format detection, version detection, common model, and IDF/epJSON conversion.
- `cmd/semantic-idf/frontend/assets.go`: embedded frontend filesystem exposed to the desktop entrypoint.
- `cmd/semantic-idf/frontend/src`: tracked static frontend source served by Wails.
- `cmd/semantic-idf/frontend/src/js`: frontend modules split by state, actions, input views, analysis views, navigation, layout, and sample data.
- `cmd/semantic-idf/frontend/src/vendor`: vendored browser-only libraries.
- `cmd/semantic-idf/frontend/src/samples`: bundled sample inputs used by the app and tests.
- `cmd/semantic-idf/frontend/dist`: ignored future build output location.
- `docs/agent.md`: consolidated working notes and implementation principles.
- [Energy Path handbook](docs/energy-path.md): graph and consumer contracts, equipment boundaries, independent native fixtures and regression coverage.
- `scripts`: repo-local runtime setup, checks, and repeatable commands.
- `.runtime`: ignored local Go/Wails runtime and caches created by setup.

## EnergyPlus References

The parser currently supports EnergyPlus version 22 or newer when a `Version` object is present. Full IDD/schema validation is intentionally separated from parsing: version-specific files can be added later under `resources/energyplus/<major>.<minor>/Energy+.idd` and `resources/energyplus/<major>.<minor>/Energy+.schema.epJSON`.

The epJSON path is being aligned with the official schema shape. Detailed surface and shading vertex fields are converted to a `vertices` array of coordinate objects; broader schema-aware numeric typing and extensible-field support should continue to grow from the official EnergyPlus schema references.
