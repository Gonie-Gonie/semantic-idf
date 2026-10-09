# Simulation runner contract

Use this document when changing purpose planning, EnergyPlus execution, saved
results, or Simulation and Batch Simulation views. Detailed accounting and
payload rules live in [Energy Path wire contract](energy-path.md#wire-contract);
CLI processing lives in [CLI and Python](energy-path.md#cli-and-python).

## Execution and API boundary

Purpose runs parse input, normalize the request, build a `PurposeRunPlan`,
apply missing outputs to a run copy, execute EnergyPlus, read results, and build
a `PurposeResultBundle`. The source document changes only through an explicit
permanent Output apply API call.

| API | Role |
| --- | --- |
| `BuildSimulationRunPlan` | Preview the backend plan without executing EnergyPlus. |
| `RunSimulationText` | Run text or an input path; use purpose flow when `purposeRequest` exists. |
| `RunPurposeSimulationText` | Purpose wrapper, defaulting to Basic Energy and Zone Heat Flow. |
| `ApplyPurposeOutputsText` | Permanently apply a plan through the Output preview/apply pipeline. |

`SimulationRunRequest` is defined in
[simulation.go](../cmd/semantic-idf/internal/simulation/simulation.go):

| Fields | Meaning |
| --- | --- |
| `runId` | Caller-provided or generated run identity. |
| `text`, `inputPath`, `filename` | Text takes precedence; otherwise read the IDF/IMF/JSON/epJSON path. Filename names the run copy. |
| `energyPlusExecutablePath`, `weatherPath`, `outputDirectory` | Explicit executable, optional EPW, and optional run directory. |
| `purposeRequest`, `purposeRunPlan` | Requested purposes and backend plan attached after preparation. |
| `resultMode`, `useReadVarsESO` | SQL-first or legacy CSV strategy; ReadVarsESO controls EnergyPlus `-r` in SQL-first mode. |
| `silent`, `auto` | Suppress UI status or identify app-started automatic runs. |
| `standardOutput`, `standardOutputMode` | Compatibility options for the earlier standard preset. |

## Current desktop defaults

The single-file view offers Basic Energy, Zone Heat Flow, HVAC Loop Check, and
Comfort. **Run & Inspect** sits beside Weather. Batch Simulation offers the
first three purposes, weather mapping, recursive file search, and worker count.
Run-plan preview, Custom Outputs, Integrity, and advanced output policy remain
backend capabilities.

Both views send these fixed purpose options:

| Option | Value |
| --- | --- |
| `allocationPolicy` | `by_service_path_load_share` |
| `basicEnergyDetail` | `energy_path` |
| `zoneHeatFlowDetail` | `surface` |
| `outputApplyMode` | `add_missing_only` |
| `frequencyPolicy` | `purpose_default` |
| `sqlMode` | `sql_first` |
| `persistOutputs` | `false` |
| `discoveryAllowed` | UI sends `false`; normalized Energy Path enables post-run dictionaries. |
| `scope.zoneMode`, `scope.loopMode`, `scope.periodMode` | `all`, `all`, `full` |

Period endpoints and zone filters are empty. HVAC Loop Check requests all loops
independently of HVAC tab selection. Result display scope is separate: choosing
a Zone or month does not rerun the model or change the output plan.

Single-file runs detect the IDF/epJSON version and choose a compatible registered
installation. Running is unavailable without a usable installation, a match for
the known model version, or installation version metadata to verify it. Direct
callers may provide an executable.

Batch leaves the executable empty. The runner merges configured/auto-detected
installations and matches each input's major/minor version independently. A
known version without a match yields `missing_energyplus` for that file while
others continue; an unreadable version uses the first available installation.
An explicit backend executable applies to the whole batch.

## Progress and estimates

Single-run progress reports engine activity, initial SQL Series and Heat Flow
reads, fallback reads and the selected purpose builders separately. Engine
stdout is drained while the process runs, including Warmup, sizing and simulated
dates. A simulated date is activity evidence, not a promised fraction of total
wall time. SQL observation counters reuse the existing read-only scans rather
than adding a full-table `COUNT` solely for the progress bar.

The optional `SimulationProgress` telemetry distinguishes observed work from
estimates: `progressKind`, `workCompleted`, `workTotal`, `workUnit`, `elapsedMs`,
`phaseElapsedMs`, `sequence`, `overallPercent`, `remainingMs`,
`remainingLowMs`, `remainingHighMs`, `estimateBasis` and `estimateSamples`.
A missing remaining-time field means unknown. A zero or missing work total has
no measured denominator. Legacy `completed`, `total` and `percent` fields remain
wire-compatible stage counts for single runs; the new UI never treats them as
elapsed-time percentages.

Successful runs teach a bounded, in-memory duration model for the same executed
input, purpose/detail/scope, engine, weather and execution configuration. No new
run-history files are written. The first completed measurement permits a wide
initial estimate; additional matching samples improve the range. The displayed
`≈` percentage and time range describe remaining backend processing, not an
exact deadline. New or changed inputs start with unknown time. An exceeded
prediction returns to unknown rather than counting down to zero during work.
Restarting the app discards these session measurements.
Overlapping unrelated runs or repeated phases that do not match the timing
profile invalidate that run's estimate; they do not teach the serial profile.

Wails receives ordered progress events. HTTP clients can poll
`GET /api/simulation/progress?runId=...` for the latest event or `null`; only a
bounded number of latest snapshots is retained, with no event log. Polling does
not launch or repeat a run. The frontend polls without overlapping requests,
rejects stale run/sequence events and stops when the request settles. Result
receiving reports actual response bytes when a valid Content-Length is available;
decoding and first display remain distinct. Backend completion does not make
the displayed bar 100% while a response or display is pending.

Batch Simulation retains completed-file counts as measured work, and also
reports active/queued workers and the active child's phase. Child row counts
describe that child's current scan, not an overall percentage or a sum of ETA
values. Parallel file completion times vary with CPU, memory and disk contention.

## Purpose planning

[purpose.go](../cmd/semantic-idf/internal/simulation/purpose.go) defines
normalization, planning, and bundle construction. Purpose IDs are
`basic_energy`, `zone_heat_flow`, `hvac_loop_check`, `integrity_check`,
`comfort_check`, and `custom_outputs`.

Backend scope supports all/selected/visible/filtered zones, selected
air/plant/condenser loops, components, output signatures, custom output objects,
and custom period. Zone Heat Flow and Comfort use scoped zones. HVAC Loop Check
resolves selected nodes/components from analyzed HVAC; broad or unresolved
scope falls back to wildcard keys.

Plans retain output signatures, purpose tags, state, estimated series/frame
counts and weight, SQL/discovery requirements, and wildcard/frequency warnings.

| Output state | Meaning |
| --- | --- |
| `existing` | Present in the source input. |
| `temporary` | Added to the run copy. |
| `will_be_persisted` | Planned for explicit permanent apply. |
| `conflict` | Same target with a different frequency or field set. |

### Basic Energy options

Omitted `basicEnergyDetail` normalizes to `energy_path`. Its model-aware plan
keeps Monthly accounting observations and adds native Hourly observations for
component charts. Existing output frequencies remain; shared requests are
deduplicated. Normalization enables lightweight post-run RDD/MDD dictionaries
even when the UI sends `discoveryAllowed: false`.

Explicit compatibility tiers remain: `light` requests SQL and monthly facility/
end-use meters; `explain` adds delivered-load and zone energy variables;
`heat_drivers` adds fan heat, internal gains, solar/window, air-exchange, and
heat-balance drivers. Detail is Monthly or Hourly with `highest_resolution`;
hourly Zone Heat Flow requests can be reused. Unrequested groups are
`not_applicable`, distinct from missing requested data.

Omitted allocation defaults to `by_service_path_load_share` for Basic Energy
Energy Path. Non-Energy-Path modes and explicit `direct_only` retain direct-only
behavior. `by_zone_load_share` and `by_service_path_load_share` allocate supported
remaining consumption, preserving exact direct observations and an unassigned
Building remainder. Service-path allocation uses matching paths, then matching
zone/service load share.

Auxiliary allocation uses AirLoop paths for fans and PlantLoop/CondenserLoop
paths for pumps and heat rejection. It uses a
supply-air volume series only when one is
present and does not add new EnergyPlus output requests. Shared plant energy
without a clear cooling/heating relationship remains unassigned. Zone quality
retains Building-wide direct, allocated, and unassigned auxiliary-energy shares;
unassigned Building energy is never inserted into the selected Zone graph or value.

### Permanent application and discovery

`AnalyzeInputOutputText`, `PreviewOutputApplyText`, `ApplyOutputText`, and
`ApplyPurposeOutputsText` remain backend/automation APIs. Permanent modes add
missing requests, replace conflicting frequencies, preserve existing requests
while adding purpose duplicates, or remove matching purpose requests. Output
analysis retains purpose tags. Desktop runs use temporary additions and
`add_missing_only`.

`DiscoverAvailableOutputs` reads SQL `ReportDataDictionary`, RDD, and MDD and
merges purpose-plan entries: `available` means exact/wildcard/dictionary-class
match; `alias` means an alternate output satisfies the request; `fallback`
means the preset can request it but this catalog did not discover it. Items
retain type, key, name, units, source, alias, and purpose tags. Reads are cached
by path and invalidated on size or modification time.

## Result reading and provenance

Source priority is SQL, CSV, then ESO. `readSimulationOutputs` collects files
and ERR, then reads Series and Heat Flow independently. Failure in one section
does not discard the other; CSV/ESO fill missing or failed sections. Energy,
Integrity, and Comfort builders read their own purpose data without repeating
those computations in the initial read.

`parseSimulationSQL` is a combined context-aware entrypoint for series, energy,
heat flow, integrity, and comfort unmet hours. It retains partial data, checks
cancellation between phases, and has an elapsed-time cutoff. The runner's
independent initial read does not use that combined cutoff.

`QueryReportData` joins `ReportDataDictionary`, `ReportData`, and `Time`, retaining
dictionary identity, meter flag, index group, frequency, and interval. Results
and `semantic-idf-run.json` expose intended `resultSourcePriority` and actual
`resultSources`. Source records retain IDs, raw/normalized units, aggregation
method, SQL table/row/column, and matching requests. Request matching validates
type, name/alias, key, frequency, and sign rather than trusting object index.

Generic preview is capped at 256 SQL or 16 CSV columns and samples points.
Requested HVAC/Comfort series and Energy Path source traces retain full
matching observations beyond preview limits. The plan is attached before
parsing for new runs and saved results.

Energy normalizes `J`/`kJ`/`MJ`/`GJ`/`Wh` to `kWh`; rate-only sources integrate
over reporting intervals. Reported energy takes precedence over a same-target
rate fallback. Tabular end-use fallback is annual `sql_tabular` with
`tabular_annual_value` aggregation. Monthly/RunPeriod rows cannot establish
daily/hourly observations. Finer SQL rows preserve observed periods; custom
backend scope can add `selected_range`.

Generic Series retain raw values plus display column/unit/statistics and
converted points. Missing output, denominator, unknown coverage, and measured
zero remain distinct through UI, reload, and export. Detailed carrier closure,
thermal/site links, allocation, and frozen v1 compatibility live in
[Energy Path wire contract](energy-path.md#wire-contract).

## Current result views

### Energy Path

Energy uses Scope and Period and shows cooling/heating together. Component
Monthly/Hourly charts use `kWh/m²`: Monthly shows computed contributions;
Hourly shows actual reported source energy, which can differ from allocated
Monthly values. Missing Hourly data is unavailable.

Intensity uses the executed model's area snapshot, including applicable
zone/list multipliers and building floor-area eligibility. Later editor changes
cannot alter it. Zone uses its own floor area; missing area means unavailable
intensity. Ratios, non-energy values, and stored/exported totals retain units.

The quality line separates Drivers, Loads, End uses, and Carriers. Stages open
source availability in a normally closed Data details drawer; Output shows this
run's requests. Tabs support arrow/Home/End; Escape restores opener focus.
Old Overview/Sankey/Zones/Systems/Reconciliation data may remain for compatibility;
these are not desktop navigation surfaces to extend.

### Heat Flow and HVAC

Zone Heat Flow retains SQL or CSV/ESO observations. Transfer signs are relative
to zone air: positive adds heat; negative removes heat. Physical Net sums the
six transfer terms (`internalConvective`, `surfaceConvection`, `interzoneAir`,
`outdoorAir`, `systemAir`, `systemConvective`). `airStorage` and `deviation`
are separate diagnostics, never additional gains/losses. Positive storage means
air warming and negative storage means air cooling. The computed balance residual
is `Net - airStorage`, available only when all six transfer terms and storage
are observed; reported deviation retains its original sign separately. See the
[EnergyPlus balance equation](https://github.com/NREL/EnergyPlus/blob/v25.1.0/src/EnergyPlus/ZoneTempPredictorCorrector.cc#L4981).

For verified EnergyPlus report versions, readers normalize `systemAir` by the
effective Zone × ZoneList multiplier. SQL producing-engine/`Zones` metadata is
authoritative; CSV/ESO and SQL fallbacks use the executed input copy, never a
later editor model. `systemConvective` is divided only when the executed input
excludes `ZoneHVAC:HighTemperatureRadiant` and `SwimmingPool:Indoor`; those
per-zone contributions otherwise mix with multiplied NonAirSystemResponse in
the same aggregate. Mixed aggregates retain the reported basis with a warning.
See [the engine report calculation](https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/ZoneTempPredictorCorrector.cc#L5326).
The optional zone `rateBasis` records `effectiveMultiplier`, `systemAir` and
`systemConvective` basis names plus original `reportedSystemAir` and
`reportedSystemConvective` arrays using the category's observation mask.
Generic Series, original outputs and canonical boundary energy remain raw.
Legacy payloads without basis metadata are not corrected on reload; a new run
is required. Reported deviation also stays raw and need not equal the computed
normalized Net-minus-storage residual. Unknown producing basis is never guessed.

The map's local bars and gain/loss summaries include internal convective gains,
system air transfer and system convective gains. Surface convection, interzone
air and outdoor air form a separate exchange group. Zone-level air/convection
arrows show aggregate direction, without assigning that total to a neighbouring
zone. Ledger numbers use signed kW to 0.001 kW, with the retained W value in
their title. Partial totals are marked; missing observations are not zeros.
New zone-series `observed` category-major masks and `temperatureObserved`
frame masks distinguish valid zero from invalid/absent values. Older payloads
without masks retain their historical value-array semantics.

Concrete surface arrows use canonical `thermalTopology` boundary flows with
explicit owner/target identities, the executed model's minimal `planGeometry`
snapshot, and verified Hourly source observations at the exact ledger timestamp.
Each reciprocal interzone pair is counted once. Incoming and outgoing amounts
remain separate, with kWh per reported interval; surface conduction/window
exchange is not the zone-air convection rate and is never added to its kW
balance. Contributing sources need explicit supported rate/energy units. Rate
aliases additionally require an actual one-hour integration interval; reported
Hourly energy in J/kWh can remain verified at aligned observed timestamps even
if other hours are absent. Unaligned sparse, mixed-frequency, ambiguous or
unmatched observations cannot produce a verified pair arrow. Older results
retain aggregate zone exchange direction but require a new Surface-detail run
for verified pair arrows.

The compact floor grid keeps one globally numbered badge/local stack per zone,
grouping its floor polygons without changing their geometry. Zone tables,
reading guidance and measured peer details start collapsed and retain their
disclosure state during frame changes. A shared SVG connects actual visible
badge centres, including cross-level pairs; resize and disclosure changes
refresh those positions. Plans fit the card automatically with no zoom/pan
camera, controls or transform state. Each visible pair has one signed-net arrow. All
verified outdoor orientations combine into one Outside arrow, excluding Ground
and Adiabatic peers; hidden zone peers never become external labels. Balanced
nonzero gross exchange has one neutral bidirectional path. Original gross
incoming/outgoing values and canonical provenance remain in the details.

Map colours, local stacks and inspector rate bars use fixed whole-dataset
extents across every zone and retained timestamp, independent of the Story
filter and selected frame. Signed logarithmic magnitudes use
`sign(q) * log1p(abs(q) / 1 W) / log1p(maximum / 1 W)` without a zero deadband.
Local stacks map each sign's total height then preserve category proportions
within that sign. Net fill uses the maximum absolute physical Net; local stacks
use the largest positive/negative local total. Inspector rows share the largest
absolute category/residual rate, including separately grouped diagnostics.
These extents are cached once per immutable dataset. Legends disclose fixed
logarithmic scaling; exact kW/W values and linear history plots remain available.
Temperature retains its reported range. Missing map observations are hatched
and missing local totals have no measured-zero baseline; known zero remains
neutral. Missing inspector rows are explicitly unavailable. Interval kWh
surface exchanges never enter the rate scales.

The history section spans the result width and separates local transfers,
boundary exchanges and reported storage/deviation into three panels with a
shared signed scale. Labels inherit the UI font and Settings graph font size
with a readable minimum; narrow panels scroll horizontally. It draws every
source frame from SQL, CSV or ESO, without a frame cap, stride or averaging.
Timestamps are registered even when all selected observations at that time are
invalid; observed masks retain those gaps. Original record order, missing
timestamps, design-day reversals and duplicate labels remain intact, without
interpolation or calendar sorting. Play advances one source frame per tick and
loops within the selected range. Slow/Normal/Fast change the wall-clock delay
(900/420/160 ms), never the frame increment. Playback updates only current
values, floor overlays, exchange arrows, legends and cursors; static history
paths and the overview brush remain mounted. Chart time layouts use a bounded
four-range cache per immutable dataset. Zone/range/language changes rebuild the
affected history. Previously saved sampled results retain their shown/original
counts and need regeneration to recover discarded source frames.
Day/Week/Month select 24-hour/7-day/30-day timestamp
windows around the selected time within one contiguous calendar sequence;
invalid dates or a backward/wrapped sequence cannot fall back to frame counts.
A displayed frame count is not elapsed hours; rate values require their actual
intervals for energy integration. Static Topology remains separate; see
[Topology data contracts](topology.md#data-contracts).

HVAC Loop Check uses executed-input topology and the shared supply/demand
schematic. Nodes show flow, temperature, humidity, and available setpoints;
equipment retains operation, power, load, and reported COP. Observations
are not added as disconnected diagram objects: annotations require a drawn
circuit port/equipment anchor or a unique typed parent/zone-port relationship.
Unplaced observations remain available to chart controls. Layout reserves
local annotation space around icons and between branches, and keeps bus
readings in the outer gutters. Missing readings
cannot borrow an earlier frame. Unset setpoint `-999`, including roundoff
within `0.000001 °C`, is excluded from snapshots, availability, and deviation
checks; valid zero/negative setpoints remain. Missing setpoints alone create
no control alert. Older results without executed topology show unavailable
topology alongside retained observations.

Flow, temperature, and humidity graphs have per-node controls, independent
Y ranges by unit, and Auto reset. Custom graphs choose object then property;
lines use at most two unit axes, and scatter compares two properties at matching
times. Humidity uses RH, otherwise humidity ratio in `g/kg`. HVAC and Simulation
selection stays local.

### Comfort and Integrity

Comfort graphs show available zone, operative/radiant temperature, heating/
cooling setpoints, PMV, PPD, humidity, and reported unmet time. People keys map
through the executed model to their actual zones with separate People-group
traces. Building values require facility observations, never invented averages/
sums of zone PMV or unmet hours. Custom periods omit full-run tabular unmet
hours. Comfort exports use the same graphs and indicators.

Integrity remains a backend result with ERR/SQL diagnostics, tabular previews,
and static/SQL cross-checks for zones, surfaces, constructions, and nominal
loads. Match statuses distinguish exact, normalized, compact alias, static-only,
and SQL-only names.

## Artifacts and exports

Purpose runs write `semantic-idf-run.json`, `semantic-idf-run-plan.json`, and
`temporary_outputs.diff`. Energy Path **Data details → Data → Export** provides
HTML, XLSX, and captured full-run JSON.

HTML/XLSX consume one immutable saved-result projection. Their first section/
sheet includes Scope/period, Drivers, Loads, End uses, Carriers, Ratios, and
Quality. Cooling/heating emphasis is labelled separately from the all-service
summary. Reports identify the saved run and retain direct/allocated/unassigned,
unknown, and unavailable meaning.

HTML hides raw source/link/JSON trace in closed details and includes available
Heat Flow, HVAC, and Comfort. XLSX defaults to summary; **Include trace sheets**
affects XLSX only. Trace retains original units, source IDs, independent
endpoint quantities, and exact original JSON including null/unknown fields.
Exporting uses captured results without analysis, SQL reread, or another run.

Batch CSV/XLSX carry purpose metrics, summaries, nodes, edges, sources/
availability, and reconciliation. IDs, output indexes, SQL references, units,
rule/formula, period, service, zone, and HVAC paths remain traceable. Default
CSV omits daily/hourly expansion; full batch JSON
(`semantic-idf.batch-simulation/v1`) retains embedded periods. XLSX includes Run
Context and selected baseline/target comparisons; v2 starts with Building/
Annual Summary, and link-delta detail is opt-in trace. Missing rows are
`Missing`; real zero remains zero; zero-baseline percent deltas are unavailable
in UI/CSV/XLSX. Baseline and target retain separate source evidence and case IDs.

## Generated storage ownership and cleanup

`storage_app.go` exposes `GetStorageUsage` and `CleanStorage`, also available as
`GET /api/storage` and `POST /api/storage/clean`. Settings shows logical file
lengths and completed-run counts in the default, configured and previously managed
run roots. `storage_roots.go` maintains a bounded discovery index independently
of the ownership checks.
Browser/WebView data is measured separately and never removed by this API.
RAM caches, models, settings, exports and engine/weather installations are not
generated-run cleanup targets.

`internal/simulation/storage.go` owns path validation, bounded inspection and
cleanup. New automatically created child directories carry a versioned ownership
marker with completion, generated-file inventory and size/mtime fingerprints.
Files modified after completion protect the whole bundle. Explicit output paths and
pre-existing directories remain unmanaged. Legacy completion metadata is trusted
only in the exact default root and after identity, path and timestamp validation.
Unknown files, nested directories, linked/reparse paths, live process ownership,
active runs and current single/batch/loaded result references prevent removal.
`storage_instances*.go` registers live desktop and filesystem-reading CLI consumers
under an OS gate. Another live consumer postpones cleanup; registration and
deletion use the same gate to prevent a startup race.
`SetStorageInputPath` (also `POST /api/storage/input`) protects the current model
and promotes an explicitly opened managed run copy to retained user data.
Simulation file/folder selection, path-based execution and run plans, and Batch
Metrics apply the same promotion before reading or preparing an input copy.
CLI filesystem inputs also promote managed run copies and retain process presence
through export completion; stdin/help do not create storage registration records.

The app holds a read lock for execution and result reads; cleanup uses `TryLock`
to return a busy error immediately. Domain execution/cleanup also share a registry
lock. Revalidate ownership immediately before removal, keep ownership metadata
until other files are gone, and report actual removed bytes and partial failures.
Never infer disposability from a filename or an ignored Git path.

The manual age filter removes all eligible runs or runs older than 7/30/90 days.
The opt-in `storage.autoClean` policy cleans unused completed runs after startup
workspace initialization, after result replacement and when the app closes.
Current result references
remain protected until shutdown; user model paths remain protected, and active
operations prevent shutdown cleanup. Settings must keep form edits and
in-flight storage status independent through rerenders and language changes.
Tests use isolated temporary fixtures; verification never cleans real user runs.

## Development checks

Use [testing.md](testing.md) to inspect plans and select affected areas.
`dev.bat test -Area simulation` runs every Simulation tier; `-Area energy-path`
covers its accounting/output pipeline. Focused contracts include
`TestBuildPurposeRunPlanBasicEnergyDefaultsToEnergyPath`,
`TestPurposeSQLSeriesBypassPreviewLimitForSelectedPanels`,
`TestInitialSQLReadKeepsSeriesWhenHeatFlowIsMalformed`,
`TestComfortSQLGraphsKeepEveryWeatherHour`, and
`TestPurposeBundleProgressCombinedParityAndImmutableInputs`.
