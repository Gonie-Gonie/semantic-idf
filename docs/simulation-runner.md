# Simulation Runner

This document tracks the current runner contract and the purpose-driven
simulation flow. The runner is intentionally split into a run-copy workflow:
temporary output requests are applied to the text that EnergyPlus executes, not
to the source IDF, unless the user explicitly applies those outputs through the
Output workflow.

## Current Request

`SimulationRunRequest` contains:

- `runId`: caller-provided or generated run id.
- `text`: input text to run. When present, the runner writes this to the run
  output directory.
- `inputPath`: existing IDF/IMF/JSON/epJSON path used when `text` is empty.
- `filename`: display and run-copy filename.
- `energyPlusExecutablePath`: explicit EnergyPlus executable path.
- `weatherPath`: optional EPW file.
- `outputDirectory`: optional explicit output directory.
- `standardOutput`: legacy compatibility flag for the previous standard output
  preset.
- `standardOutputMode`: legacy merge/replace mode for the standard preset.
- `purposeRequest`: purpose-driven output and result request.
- `purposeRunPlan`: backend-built plan attached after preparation.
- `resultMode`: result strategy, currently `sql_first` or legacy CSV fallback.
- `useReadVarsESO`: controls EnergyPlus `-r`/ReadVarsESO CSV generation when
  `resultMode` is SQL-first.
- `silent`: suppresses UI-facing status messages.
- `auto`: marks auto-runs started by the app.

## Purpose Flow

The purpose flow is:

1. Parse current text or `inputPath`.
2. Normalize `SimulationPurposeRequest`.
3. Build a `PurposeRunPlan`.
4. Apply missing purpose outputs to the run copy only.
5. Run EnergyPlus.
6. Read SQL first, then CSV, then ESO fallbacks.
7. Build `PurposeResultBundle` while preserving legacy `Series` and
   `HeatFlow` fields.

`BuildSimulationRunPlan` exposes the planning step for UI preview. `RunSimulationText`
uses the purpose flow when `purposeRequest` is present. `RunPurposeSimulationText`
is a convenience wrapper that defaults to Basic Energy + Zone Heat Flow.

## Purpose Model

Supported purpose ids:

- `basic_energy`
- `zone_heat_flow`
- `hvac_loop_check`
- `integrity_check`
- `comfort_check`
- `custom_outputs`

`SimulationPurposeScope` can express all, selected, visible, or filtered zones;
selected air/plant/condenser loops; selected components; output signatures; and
custom output objects. Zone Heat Flow and Comfort use scoped zone names when
provided. HVAC Loop Check uses selected loop node names when they can be resolved
from the current HVAC analysis, requests component operation variables for
resolved loop components, and falls back to wildcard node/component keys when
scope is broad or unresolved. The Simulation UI passes the active HVAC tab loop
as selected HVAC scope when HVAC Loop Check is enabled.

`PurposeRunPlan` reports:

- output objects with purpose tags, signatures, state, and estimated weight
- overall estimated series count, frame count, and weight
- SQL and discovery requirements
- warnings for wildcard scope, frequency conflicts, and Heavy/Very Heavy output
  estimates based on series count times frame count

Basic Energy requests SQL, monthly top-level/end-use meters, monthly delivered
load variables, and monthly heat-balance driver variables. When Zone Heat Flow
is also selected, its hourly heat-balance outputs are reused instead of adding a
duplicate monthly heat-driver request.

Output states:

- `existing`: already present in the source IDF.
- `temporary`: added to the run copy only.
- `will_be_persisted`: planned for permanent apply.
- `conflict`: same output target exists with a different frequency or field set.

## Result Reading

`readSimulationOutputs` is split into:

- `collectSimulationFiles`
- `parseERR`
- `parseSQLResults`
- `parseCSVResults`
- `parseHeatFlowFallback`

The result source priority is SQL, then CSV, then ESO. The run result and
`semantic-idf-run.json` manifest expose both `resultSourcePriority` and the
actual `resultSources` used by the parsers. SQL parsing already feeds legacy
`Series` and `HeatFlow`, so older viewers continue to work while purpose result
viewers are added. Basic Energy SQL rows are converted to display units
(`J`/`kJ`/`MJ`/`GJ`/`Wh` to `kWh`, `W` to `kW`) and grouped into monthly chart
points when `Time.Month` is available, so hourly or timestep energy rows can
still feed monthly dashboards. Basic Energy also builds an
`energyExplanation` payload with `semantic-idf.energy-explanation/v1` schema,
source IDs derived from `ReportDataDictionary`, accounting-basis edges, and
residual reconciliation between facility carrier totals and mapped end-use
meters. The companion `energyExplanationSummary` payload keeps the annual
carrier, end-use, delivered-load, heat-driver, residual, and top-zone rollups in
a compact shape for batch comparisons and exports. When heat-balance rate
variables are present, the same payload
integrates them to `kWh` by timestep and links Delivered Load to Heat Drivers
with signed driver values and residual reconciliation.

Generic SQL and CSV series keep original values for compatibility and also
expose display metadata (`displayColumn`, `displayUnit`, `displayMin`,
`displayMax`, `displayAverage`, and converted `displayPoints` when values
change). Result charts and purpose summary tables use these display fields so
energy, power/rate, temperature, mass-flow, and humidity-ratio units stay
consistent across viewers.

Batch purpose simulations also summarize the annual explanation graph into
compact purpose metrics for Energy Use, Delivered Load, Heat Drivers, residual,
mapped percent, and the largest heat-driver groups. When two Basic Energy
purpose rows with explanation summaries are selected, the batch chart also shows
the largest explanation changes plus end-use, delivered-load, and heat-driver
delta tables beside the selected metric. Missing summary rows are labeled
separately from matched rows so an absent output is not silently treated as a
normal zero.

`parseSimulationSQL` is the combined SQLite entrypoint. It gathers generic
time-series rows, Basic Energy dashboard data, SQL heat-flow data, Integrity
diagnostics/tabular reports, and Comfort unmet-hours rows into one parse result,
while keeping partial results when one SQL feature is absent or malformed. The
entrypoint uses a timeout-aware context wrapper and checks cancellation between
parser phases so oversized SQL files do not monopolize the runner indefinitely.

Purpose result viewers now include:

- Basic Energy facility/end-use monthly charts, zone matrix, zone reported
  energy table, and `Overview` / `Sankey` / `Monthly` / `Zones` / `Sources` /
  `Reconciliation` subviews for tracing Energy Use to Delivered Load and Heat
  Drivers with source metadata. The `Systems` subview and node inspector match
  load/heat services to the current HVAC service model by zone and service kind,
  then link directly to the related HVAC service path. Sankey and Systems can
  focus the graph by all results, a selected zone, or a selected HVAC service
  path without changing the stored explanation payload.
- Zone Heat Flow SQL or CSV/ESO ledger with frame sampling metadata and
  time-range controls.
- HVAC Loop Check node summaries, component operation summaries for fans,
  pumps, coils, chillers, boilers, and cooling towers, derived loop metrics,
  node-state heat-transfer estimates, loop status classification, and alerts for
  zero flow, flow without temperature spread, missing setpoints, and large
  temperature-setpoint deltas. The frame snapshot includes a compact node
  schematic with live temperature/flow labels before detailed node and component
  cards. The Simulation result viewer deliberately keeps that live schematic
  compact, and reuses the existing HVAC tab `renderHVACLoopDiagram` only as an
  optional topology panel when the current HVAC selection matches the simulated
  loop. The result view also provides panel toggles for topology, snapshot, and
  normalized multi-series chart, plus variable group toggles for temperature,
  setpoints, mass flow, humidity/enthalpy, rate/load, power/energy, and other
  HVAC outputs.
- Comfort zone metric summaries for temperature, setpoint, PMV, and PPD series,
  with optional custom `MM-DD` period scoping for the rendered trends and issue
  ranking.
- Integrity ERR, SQL error table, tabular report previews, and SQL/static
  cross-checks for zone, surface, construction, and nominal-load tabular rows.
  Cross-check statuses distinguish exact names, normalized matches, compact
  aliases, static-only names, and SQL-only names.

Where a row can be matched back to the run plan, result tables show the source
output state and signature so the user can distinguish existing, temporary, and
will-be-persisted output requests.

## Permanent Outputs

`ApplyPurposeOutputsText` converts a purpose plan into the existing Output apply
pipeline. This keeps permanent output edits behind the same preview/apply
contract as manual Output tab changes. The Output analysis report also annotates
existing and recommended output requests with purpose tags, and the Output tab
can filter by purpose. Permanent purpose-output application supports four modes:
add missing outputs only, replace conflicting frequencies, keep existing outputs
and add purpose-specific duplicates, or remove existing outputs that match the
selected purpose plan. The EnergyPlus run-copy path still keeps existing outputs
and adds temporary purpose outputs so result parsing can use the requested
series without editing the source IDF.

## Output Discovery

`DiscoverAvailableOutputs` builds a searchable output catalog from available run
artifacts. It reads SQL `ReportDataDictionary`, `.rdd`, and `.mdd` files when
present, then merges selected purpose-plan outputs as `available`, `alias`, or
`fallback` entries:

- `available`: the exact requested output, wildcard equivalent, or dictionary
  class equivalent was discovered.
- `alias`: an alternate discovered variable can satisfy the purpose request
  (for example, `Zone Air Temperature` for `Zone Mean Air Temperature`, or
  `Gas:Facility` for `NaturalGas:Facility`).
- `fallback`: the purpose preset can still request the output, but it was not
  discovered in the current SQL/RDD/MDD catalog.

Catalog reads are cached per SQL/RDD/MDD path and invalidated when file size or
modification time changes. Each catalog item reports its object type, key,
variable or meter name, units, source, status, alias target when applicable, and
purpose tags. Custom Outputs entered manually or picked from discovery are saved
locally and restored in later sessions.

## Run Artifacts and Export

Purpose runs write `semantic-idf-run.json`, `semantic-idf-run-plan.json`, and
`temporary_outputs.diff` in the output directory. The UI can export a purpose
result JSON bundle or a standalone HTML report that embeds run metadata,
including EnergyPlus executable/version metadata, the run plan, parsed purpose
results, purpose-specific Energy/Heat Flow/HVAC/Comfort summary tables,
completeness, file references, and the source output signatures visible in
result tables.
