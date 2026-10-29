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
| Generated run storage and cleanup | `storage_app.go`, `internal/simulation/storage.go` | Verify ownership and preserve active/retained results before removing files |
| Frontend state, actions and views | `frontend/src/js/`, `frontend/src/styles/` | Feature contracts linked above |
| Language registry, catalogs and runtime translation | `frontend/src/js/i18n.js`, `frontend/src/js/locales/`, `frontend/src/js/localized-text.js` | [frontend language policy](../cmd/semantic-idf/frontend/README.md) |
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
supports identity and indexing; its former input renderer and view state are
removed. Text, JSON and Table are the only input views. Follow the
[navigation contract](semantic-navigation.md) when adding another participant.

Profile's `cooling_setpoint` and `heating_setpoint` dimensions carry configured
thermostat targets using the `temperature` metric in `degC`. Preserve actual
temperatures and native thermostat/schedule identity; never sum them as loads
or convert inactive/missing hours to zero. Their summary is the mean of known
active hourly values. They are comparison dimensions, not Profile Apply targets.
Graph series use nullable actual-temperature arrays `dayValueProfile`,
`weekValueProfile`, `monthValueProfile`, `annualValueProfile` and
`durationValueProfile`, separately from the existing load-multiplier arrays.
Null means an inactive or unresolved target; numeric zero remains a real 0 °C.

HVAC operating profiles are scoped by component object index and loop ID.
Typed containment supplies ancestors; exact node/circuit matches supply
controllers and setpoints. The interface has three panels: stacked Availability
permissions, compact On / Off expressions and Parameter control source cards.
Each `operatingProfiles` entry retains source-backed `HVACControlSequence` records:

| Field | Meaning |
| --- | --- |
| `availabilityDefinitions` | Declared owner, assignment list, AvailabilityManager and schedule references; expanded supported schedule definitions |
| `onOff` | Configured permission and demand conditions with explicit ON, OFF and unresolved branches |
| `parameterControl` | External inputs, controllers/setpoints, manipulated control variables, calculated state variables and external outputs |

`onOffExpression` is an additive `HVACOperatingExpression` AST for compact
decision cards. `referenceId` binds an opaque condition to its exact `onOff.nodes`
entry. The browser presents the full availability expression and the command
expression as named formulas; only native `all`/`any` become nested AND/OR groups.
Manager precedence, ordered first-non-NoAction selection and fan override retain
their named operators and status domain. NoAction=0 is not Boolean OFF.
The family activation condition stays opaque/source-backed; incoming demand,
thermostat and dispatch relationships are dependencies, not an inferred AND.
Continuous/default pumps retain mode-dependent activation; passive equipment
retains passive flow rather than an invented ON/OFF command. Missing/dangling
expression references remain unresolved and require reanalysis where necessary.

Parameter control projects the existing source graph into four compact groups:
inputs/targets, comparisons/controllers, commands and equipment response/outputs.
Cards group exact source identities and node memberships; signed relationship
chips retain the actual endpoint roles. Cyclic feedback is represented by source
references, never recursive expansion or a guessed execution order. The compact
presentation introduces no new control formula; supported native residuals and
raw Action fields keep their separate meanings. Original graph records remain
available to programmatic consumers. Numeric schedule badges retain the exact
representative-day trace basis in a date/day-type tooltip; they do not claim an
annual or constant target range.

The additive `configuredAvailabilityLayers` and `configuredAvailability` fields
use the existing layer/expression types for the minimum configured static
conditions. Availability uses these fields, with source-owning object labels at
the left of binary schedule rows and the combined configured result at the
bottom. A source schedule is ON when positive, OFF when nonpositive, and unknown
when unavailable. A source flag is not itself a manager status: ScheduledOff
tests exact zero, and supported scheduled-manager overrides and priority remain
part of the combined result.

The configured projection excludes verified runtime managers such as NightCycle,
temperature/load decisions and actual dispatch selection. Their unknown runtime
state does not contaminate the static result. Preserve operation-scheme activation
schedules and exact equipment membership, supported own-field defaults and known
scheduled manager rules. An absent separate own availability schedule is an
explicit unrestricted equipment row, not the omission of plant/loop conditions.
Detected missing, duplicate, unsupported or malformed configuration stays unknown; never
convert arbitrary unknown expressions to true. Runtime-manager exclusion requires
verified type, schema, assignment context and reference evidence. The legacy
`finalAvailability` and
control graph retain their full configured/runtime dependency semantics.
Source definition records remain
available programmatically; Availability exposes no raw numeric
schedule ranges, definition/parameter disclosures or algorithm nodes. Both
availability evaluation and On / Off preserve all/any semantics, air/zone
manager precedence and Plant availability-manager assignment priority rather than treating every
manager as another AND gate. On / Off retains unresolved runtime decisions that
the static Availability view excludes. Neither view asserts that equipment ran.

Graph nodes retain source identities and parameter object/field indexes.
Availability expressions may additionally retain their declaring `source`, so
an inherited fan override keeps the loop/parent manager's actual owner rather
than guessing it from the selected fan or manager-list position.
Manager `assignmentOwner` retains its actual loop/equipment attachment for
validating supported assignment context before excluding a runtime contribution.
`boundary` identifies `input`/`output`; edges may carry `condition` (`yes`, `no`,
`unknown`) and `polarity` (`positive`, `negative`, `unknown`). A feedback sign
requires a supported comparison or response rule, not an equipment name or a
generic controller label. Preserve the controller's Action field separately
from comparison signs. State-variable nodes describe modeled outcomes; capacity or
power is not automatically an independent actuator. Cyclic feedback links must
not be interpreted as a topological execution order.

The graph role `control_variable` means a manipulated command. The unchanged
native Controller:WaterCoil `Control Variable` field instead names the sensed
or regulated quantity; its source wording must not be relabeled as an actuator.
Temperature/humidity target channels match both node identity and target quantity;
one resolved channel does not establish another. Missing WaterCoil target channels
remain explicit external inputs with unresolved provider/value and comparison
signs, except the supported pure-temperature residual convention.

Legacy `availabilityLayers`, `finalAvailability` and `control` fields remain
compatible. On / Off requires its authored expression AST; missing metadata is
unresolved rather than Boolean logic inferred from old graph edges. Parameter
control shows a reanalysis hint when its source graph is absent.
Availability treats the new configured fields as authoritative. A legacy full
expression is reusable only when it has no opaque runtime dependency; otherwise
the final row reports missing configuration metadata until reanalysis, without
reintroducing runtime-result rows or assuming those unknowns are true.
`operatingSchedules` retains exact supported calendar rules. The Availability
toolbar exposes only Rep. day, Rep. week and Annual. Representative views use
January 1/current year and a Monday anchor; Annual uses that year's regular
Gregorian weekdays. Programmatic calendar options remain compatible, but the
UI has no month/day/day-type/year/reset controls. Holidays, DST and runtime
overrides are not invented. Minute transitions and unknown intervals survive
compression; caches and drawing work are bounded. All rows share horizontal
zoom/pan and a time cursor/status. Static HVAC does not reuse Profile's
approximate schedule fallback.

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

Desktop result loading separates immutable result resources from presentation.
The negotiated HTTP transport publishes a catalog and bounded JSON resources
identified by input hash, run ID and snapshot token. The original result
builder and canonical APIs remain the source of values. A common priority queue
deduplicates reads, prepares the active view first and prefetches nearby actions
and other views within its background budget. Other views share a round-robin
priority tier so one large tab cannot monopolize background preparation; newly
relevant work promotes existing requests. Worker parsing handles individual
resources; it never clones the expanded canonical result. View-specific
dependency declarations extend this policy without introducing another loader.

Unloaded data is a pending dependency, not an empty result or a measured zero.
Rendering resumes only for the current run and selection after its dependencies
are ready. Replacing a result cancels its pending reads and worker work. Keep
unvisited panels unmounted during prefetch. Previously visited Simulation panels
retain their DOM and chart state for the current result; returning to an unchanged
panel only changes visibility. Invalidate a panel for changed result owners,
presentation settings or relevant controls. Prepare reusable data indexes
separately, so prefetching never creates invisible DOM or changes controls.

Settings persist through the backend API in the local app data/config directory.
Keep defaults and migration in `settings_app.go`, rather than duplicating them
across frontend views.
Configuration writes replace a completed sibling temporary file under a shared
settings mutex; failed writes leave the previous configuration intact. Settings
presentation and form state belong to `js/settings.js` and `styles/settings.css`.
The default Input View is a startup preference; restored or explicitly selected
views take precedence.

The app retains six languages. English is the authoritative interface catalog;
Korean covers its keys, while Japanese/Hindi/Spanish/French supply translations
where available and fall back per key to English. Language-change events update
static and dynamic presentation without changing model IDs, units or selection.
Other windows synchronize Settings through storage events, and restored pages
reread cached settings. Late settings loads cannot overwrite a newer choice.
Guide uses Korean only for the Korean app locale and English for every other
locale; manual content and metric-description overlays belong to `frontend/src/manual/`.

## Where to make a change

Change backend model/analysis semantics in their domain package, desktop/Wails
coordination in the corresponding app module, and rendering in the existing
frontend feature module. CLI, App, HTTP and Python consumers should reuse the
same result builder. Keep wire compatibility and independent regression evidence
with the feature contract; avoid creating a second aggregator for one consumer.

Run `dev test -Plan` to inspect changed-source impact. Existing Go tests and
shared fixtures stay in their packages. Use feature/layer checks while editing,
and the installed commit hook for selected tests plus the Wails build.
