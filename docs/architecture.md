# Development architecture

Use this map to locate the implementation, then read the feature's contract in
the [documentation index](README.md). Setup, CLI examples and release commands
belong in the [project README](../README.md); verification is described in
[testing.md](testing.md).

## Code ownership

Paths below are relative to `cmd/semantic-idf/` unless specified otherwise.

| Responsibility | Implementation | Reference |
| --- | --- | --- |
| Desktop lifecycle, input editing and file dialogs | `main.go`, `app.go` | Input/workspace invariants below |
| Analysis API and stage/result assembly | `analysis_app.go`, `analysis_cache.go`, `analysis_input_cache.go` | Shared analysis below; [performance](performance.md) |
| IDF/epJSON format, version, conversion and patching | `internal/epinput/` | Input/workspace invariants below |
| IDF parsing, Metrics, Profile, HVAC, diagnostics and topology | `internal/idf/` | [navigation](semantic-navigation.md), [topology views](topology.md#views-and-interaction), [topology data](topology.md#data-contracts) |
| Batch Metrics and exports | `batch_app.go`, `batch_metrics_app.go`, `batch_cache.go`, `internal/tabular/` | Export invariants below |
| Engine execution, purposes, SQL/CSV/ESO reading and results | `simulation_app.go`, `internal/simulation/` | [simulation runner](simulation-runner.md) |
| Energy Path desktop adapters, reports and batch exports | `energy_path_app.go`, `energy_path_report_export.go`, `batch_energy_path_export.go` | [Energy Path wire contract](energy-path.md#wire-contract) |
| CLI and Python consumers | `internal/cli/`, repository `clients/python/` | [CLI/Python contract](energy-path.md#cli-and-python) |
| Settings persistence and migration | `settings_app.go` | Normalize through the backend settings API |
| Frontend state, actions and views | `frontend/src/js/`, `frontend/src/styles/` | Feature contracts linked above |
| Frontend acceptance harnesses | `internal/frontendchecks/` | [testing workflow](testing.md) |
| Shipped technical manual | `frontend/src/manual/`, `frontend/src/js/guide-manual.js` | [manual authoring](../cmd/semantic-idf/frontend/src/manual/README.md); metadata comes from `idf.MetricGuides()` |

The frontend is static HTML/CSS/JS embedded by `frontend/assets.go`. `app.js`
initializes startup settings/status; `js/main.js` wires the application and its
feature modules. `frontend/dist/` is reserved for generated assets, Wails
bindings are generated in `frontend/wailsjs/`, and binaries use repository
`build/bin/`. Repository `build/` also contains maintained app icons; it is not
entirely generated output. Prefer the static asset workflow while it meets
the application's needs.

## Input and workspace invariants

- EnergyPlus 22+ compatibility uses the input's `Version`; format/IDD/schema
  handling belongs in `epinput`, independently of the desktop lifecycle.
- Text, JSON and Table are views of one editable source document. JSON syntax
  tokens are read-only, while editable values patch the common model. epJSON
  surface coordinate extensibles use schema-shaped `vertices` arrays.
- The shared filter matches object type, real name/index, field label and value.
  Objects without a real name use type/index identity. Text/Table start expanded;
  Table has stable row headers and orientation controls.
- Editing reparses an owned document and refreshes analysis. Cached analysis
  snapshots and indexes are read-only; EPJSON export must not mutate metadata.
- The bundled startup input is the vendored official Large Office example under
  `frontend/src/samples/`. Preserve its original bytes and license provenance.
- Input and result panes scroll independently. Splitter updates use animation
  frames and persist localStorage layout values at the end of dragging.

## Shared analysis

The desktop stage API shares parsed input by a text hash after normalizing line
endings. Its input cache retains at most three snapshots, deduplicates concurrent
parses and builds one lazy `DocumentIndex` per snapshot. Result caching is
separate from parsed input ownership. Failed parses can be retried; edits use
fresh documents.

Full/overview IDF analysis uses a session that shares Geometry, Metrics, HVAC,
Diagnostics, Profile and Output dependencies with `sync.Once`. Independently
requested desktop stages share parsed input/indexes and the separate result
cache, rather than one session across every request. Keep shared indexes
read-only and mutable derived state local.
Feature functions in older IDF files sometimes serve other domains; check their
callers before treating a change as isolated.

The frontend uses `analysis-stage-queue.js` for two bounded workers, active-tab
priority and stable stage ordering. Metrics/Text/JSON/Table become usable
first; Profile, HVAC and geometry then run in the selected priority order.
A stale input or failed worker stops pending launches, and active requests
settle before the queue ends. Defer heavy
topology rendering while its panel is hidden.

## Result and export invariants

Metrics, Topology, Profile, HVAC and Simulation are the five result tabs. Tools
hosts Batch Metrics, Batch Simulation and Diagnose. Output request APIs remain
available to automation even though the former Output tab is hidden.

Metrics, Profile and Topology participate in the shared source-selection model.
HVAC and Simulation keep standalone selections. The internal Semantic projection
supports identity and indexing; it is not an exposed input tab. Follow the
[navigation contract](semantic-navigation.md) when adding another participant.

Metrics definitions, calculated values, guide entries and exports share one
catalog. Metrics JSON stays categorized; CSV is `name,value`, with variable IDs
and units in the name, including `[-]` for unitless values. Batch Metrics uses
bounded workers, progress events, transposable comparisons and CSV/XLSX exports.

Simulation purposes control output requests and result sources. Source identity,
units, multipliers and known-zero versus absent values must survive reading,
aggregation, graph construction, transport, reload and export. Shared annual or
Zone projections must preserve shared consumption budgets without double counting.
Detailed accounting and ownership rules belong in the
[Energy Path wire contract](energy-path.md#wire-contract) and
[equipment boundaries](energy-path.md#equipment-boundaries).

Settings persist through the backend API in the local app data/config directory.
Keep defaults and migration in `settings_app.go`, rather than duplicating them
across frontend views.

## Where to make a change

Change backend model/analysis semantics in their domain package, desktop/Wails
coordination in the corresponding app module, and rendering in the existing
frontend feature module. CLI, App, HTTP and Python consumers should reuse the
same result builder. Keep wire compatibility and independent regression evidence
with the feature contract; avoid creating a second aggregator for one consumer.

Run `dev test -Plan` to inspect changed-source impact. Existing Go tests and
shared fixtures stay in their packages. Use feature/layer checks while editing,
and the installed commit hook for selected tests plus the Wails build.
