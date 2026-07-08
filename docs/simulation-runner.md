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

`SimulationPurposeRequest.allocationPolicy` defaults to `direct_only`. Basic
Energy also accepts `by_zone_load_share`, which replaces direct Energy Use ->
Delivered Load links with `basis=allocated` zone-load-share edges when
zone-scoped delivered-load variables are available. It also accepts
`by_service_path_load_share`, which applies the same load-share allocation only
after load nodes are matched to HVAC service paths; if a load group cannot be
fully matched to service paths, the measured direct edges are preserved. The
Simulation and Batch Simulation purpose controls expose these policies. Batch
Simulation also exposes the same frequency policy selector as single-file
Simulation, so batch users can keep the monthly purpose default, preserve
existing output frequencies, or ask for the highest available resolution when
they need drilldown.

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
load variables across zone air system, ideal loads, radiant HVAC, coil, and
plant demand aliases, monthly object-level fan heat-to-air variables, monthly
detailed internal-gain and air-exchange heat-driver variables, and monthly zone
heat-balance driver variables. When Zone Heat Flow is also
selected, its hourly heat-balance outputs are reused instead of adding a
duplicate monthly zone heat-driver request.
End-use meter aliases cover cooling, heating, lighting, equipment, fans, pumps,
heat rejection, heat recovery, water systems, exterior lighting, refrigeration,
onsite generation, district cooling/heating end uses, natural-gas
heating/equipment/water-system use, and facility fuel oil/propane/steam/other
fuel totals where the model exposes those meters.
Onsite electricity production remains visible as a Level 1 energy end-use item
and summary/export row, but it is not counted as mapped facility consumption
when residuals and mapped percent are calculated. In Sankey payloads it is
linked with `relation=onsite_production` so the measured production source is
traceable without being classified as consumption.
When both energy and rate outputs are present for the same delivered-load or
heat-driver target, the explanation parser uses the reported energy series and
keeps the rate series only as traceable fallback source metadata. Completeness
uses the canonical target count for these fallback groups while source
availability still lists each requested output name.
Delivered-load nodes keep zone loads in `zoneName`, plant demand in `loopName`,
and aggregate coil/system loads at the system layer; heat-driver reconciliation
uses zone loads when they are available so plant and system layers are not
double-counted against zone heat-balance drivers.
When the source IDF or epJSON can be read for the run, load/heat nodes and
related edges also include `relatedPathIds` from the HVAC service model. The
Sankey inspector and Systems view use those IDs before falling back to
zone/service-kind matching. SQL source metadata also records the matching output
request `objectIndex` when the run plan references an existing output object, so
source tables, Sankey inspectors, and batch source CSV rows can jump back to the
original request before falling back to output-name matching. SQL source
metadata also records an `aggregationMethod` such as `sum_report_data` or
`integrate_rate_by_time_interval`, and source tables, inspectors, and batch CSV
exports show that method beside frequency and unit.

Output states:

- `existing`: already present in the source IDF.
- `temporary`: added to the run copy only.
- `will_be_persisted`: planned for permanent apply.
- `conflict`: same output target exists with a different frequency or field set.

Basic Energy output requests use tiered reasons in plan previews and Output
apply previews: top-level SQL/meters form the light energy basis, monthly
delivered-load and zone energy variables are labeled as `Basic Energy Explain`,
and monthly heat-balance, fan heat, internal-gain, and air-exchange variables
are labeled as `Basic Energy Heat Drivers`.

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
meters. If detailed `ReportData` rows are unavailable, Basic Energy can fall
back to annual `TabularDataWithStrings` end-use rows and marks those sources as
`sql_tabular` with `tabular_annual_value` aggregation. Energy sources preserve
both the raw source unit and the normalized graph unit so `J` meters and `W`
rate variables remain traceable after conversion to `kWh`. Daily, hourly,
timestep, or detailed SQL sources emit daily `D<n>` periods from `Time.Month`
and `Time.Day`; Hourly, Timestep, or Detailed sources also emit hourly `H<n>`
periods from `Time.Hour`. Monthly/RunPeriod sources stay annual/monthly only to
avoid treating monthly rows as high-resolution data. When a custom period scope
is selected, SQL row values whose `Time` month/day falls inside the scope are
also emitted as a `selected_range` period alongside annual and monthly periods.
The payload includes the Basic Energy relationship rule catalog, and
explanation edges carry a relationship `ruleId` from that catalog so the UI and
exports can distinguish measured end-use, measured load, heat-balance, and
residual links. The companion
`energyExplanationSummary` payload keeps the annual
carrier, end-use, delivered-load, heat-driver, residual, and top-zone rollups in
a compact shape for batch comparisons and exports. Energy nodes expose
`meterHierarchyLevel` values such as `facility_total` and `broad_end_use` so the
Sankey inspector can show which meter hierarchy tier is being reconciled. Both
payloads expose
`allocationPolicy` so exported results make clear whether allocated edges were
allowed. Carrier-qualified meters that are not in the explicit end-use alias
catalog are retained as `other` end uses while preserving the original meter
name in source metadata. With `by_zone_load_share`, cooling/heating end-use
energy is allocated to zone load nodes by measured delivered-load share and the
edge uses `relation=allocation`, `basis=allocated`, and the
`allocation.by_zone_load_share` rule. With `by_service_path_load_share`, the
allocated edge also carries the matched service path IDs and the
`allocation.by_service_path_load_share` rule. When heat-balance rate
variables are present, the same payload
integrates them to `kWh` by timestep and links Delivered Load to Heat Drivers
with signed driver values and residual reconciliation. Explicit sensible heat
gain/loss outputs are kept as separate positive/negative heat-driver nodes even
when EnergyPlus reports both source series as positive energy values.
Delivered-load nodes carry both `serviceKind` and `pathType` metadata, using the
load alias scope (`zone`, `system`, or `plant`) so HVAC service links and batch
exports can distinguish zone loads from broader system or plant demand.
When electric end-use energy and delivered thermal load are both present, Basic
Energy also reports derived COP KPIs separately from the Sankey graph rather
than creating synthetic COP conversion edges. Batch purpose metrics expose those
derived KPIs so COP can be selected directly in the Batch Simulation chart and
table.

Generic SQL and CSV series keep original values for compatibility and also
expose display metadata (`displayColumn`, `displayUnit`, `displayMin`,
`displayMax`, `displayAverage`, and converted `displayPoints` when values
change). Result charts and purpose summary tables use these display fields so
energy, power/rate, temperature, mass-flow, and humidity-ratio units stay
consistent across viewers.

Energy explanation periods include their own reconciliation rows and warnings,
so the Reconciliation subview can switch the accounting-gap table between
annual and monthly periods instead of showing only the annual graph.

Batch purpose simulations also summarize the annual explanation graph into
compact purpose metrics for Energy Use, Delivered Load, Heat Drivers, residual,
mapped percent, derived COP KPIs, and the largest heat-driver groups. When two
Basic Energy purpose rows with explanation summaries are selected, the batch
chart also lets the user pin an explicit baseline and target case, then shows
the largest explanation changes plus end-use, delivered-load, and heat-driver
delta tables beside the selected metric. It flags completeness
differences between the two selected cases, including mapped percent and missing
category changes. Explicit gain/loss heat-driver summary rows stay separate so
opposite air-exchange directions can be compared. It also ranks annual Sankey
edge deltas by relation, basis, edge label, rule ID, delta, percent, and
missing-row status. A compact bar view renders the largest selected-case edge
deltas before the table so the two-case Sankey change is visible without
reading every row.
Missing summary and edge rows are labeled separately from matched rows, and the
comparison value cell renders `Missing` instead of numeric `0` so an absent
output is not silently treated as a normal zero. Batch Simulation can export
purpose metrics, compact
`energyExplanationSummary` rows, `energyExplanation` source metadata rows,
reconciliation rows, and Sankey edge metadata rows with period, relation, basis,
`ruleId`, formula, endpoint, service, zone, source IDs, related source output
object indexes, source/normalized units, load path type, and related HVAC
service path IDs as CSV for spreadsheet comparison. The batch CSV keeps annual,
monthly, and selected-range explanation periods by default; daily and hourly
periods remain available in the embedded purpose result payload without
expanding the default spreadsheet export. Compact summary rows also carry their
source IDs and matching source output object indexes when available. Batch
Simulation can also export the same core purpose metrics, compact energy
summary, source metadata, edge, and reconciliation sheets as XLSX. When a
baseline and target case are selected, the XLSX workbook also includes
comparison context, summary delta, and annual Sankey edge delta sheets. The
workbook also includes a Run Context sheet with the selected paths, purpose
request, frequency/allocation policy, weather mode/path, worker count, and
batch view settings used for the export. It can
export the full batch result as
`semantic-idf.batch-simulation/v1` JSON, preserving embedded purpose result
payloads such as high-resolution daily/hourly explanation periods that are
intentionally omitted from the default CSV. The export context also preserves
the selected baseline and target row IDs used by the two-case explanation delta
view, along with the purpose request, frequency/allocation policy, weather
mode/path, worker count, and selected batch view mode.

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
  Drivers with source metadata. The Sankey inspector shows edge relation, basis,
  rule, formula, sources, and related service paths, and the selected edge plus
  its endpoint nodes are highlighted in the graph while the inspector is open.
  The Overview subview also includes an annual Energy Use breakdown by carrier
  and end use so fuel-qualified uses remain distinct in comparison.
  The completeness panel
  shows mapped percent, allocation policy, missing categories, and missing
  source availability rows.
  Source availability uses `found` and `missing` status values so missing rows
  are not confused with present SQL dictionary sources. Source output cells link
  back to the matching existing output request object when the run plan can
  identify one. The Monthly subview includes an Energy Use / Delivered Load /
  Heat Drivers / Residual level chart, and monthly charts and ledger rows link
  directly to the same period in the Sankey subview so month-level explanation
  changes can be inspected without manually changing views. The Reconciliation
  subview expands row `sourceIds`
  into compact source/output links so residual checks remain traceable to their
  meter or variable requests. Energy residual rows include both the expected facility
  total source and the mapped consumption end-use sources referenced by the
  residual formula. Heat-driver reconciliation includes service-level rows and,
  where zone load and heat-driver data exist, zone/service rows for the selected
  annual or monthly period. The subview ranks the largest zone/service heat
  residuals for the active period below the full reconciliation table.
  The `Zones` subview summarizes each zone/service period as Delivered Load,
  Cooling Pressure, Heating Pressure, signed heat, and residual values, with
  direct jumps to the matching Sankey zone focus, Heat-Flow Ledger zone, and
  related HVAC service paths.
  The `Systems` subview and node inspector match load/heat services to the
  current HVAC service model by zone and service kind, then link directly to the
  related HVAC service path. The Systems table now shows source energy,
  delivered load, heat-driver totals, connected plant/air/source systems, and
  supporting assets on the same service-path row, and each row can open the
  matching Sankey service-path focus. Sankey and Systems can focus the graph by all
  results, a selected zone, or a selected HVAC service path without changing the
  stored explanation payload. The Sankey view can switch heat-driver rendering
  between display magnitude, signed balance, cooling-pressure, and
  heating-pressure modes. Its sign note states that positive heat drivers add
  cooling pressure, negative heat drivers add heating pressure, and signed mode
  preserves the original heat-balance sign. The view uses carrier/service-specific
  node colors for common energy and load classes, and can cap visible heat-driver nodes with omitted
  drivers grouped as `Other heat drivers`. When grouping occurs, the view shows
  how many heat drivers were folded and offers an `All` action to expand them.
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

Basic Energy completeness panels distinguish source output shortage from
accounting/model coverage gaps. When missing source requests are reported, the
result view can jump back to the Simulation output plan, refresh the purpose
request set, and invoke the same permanent output application flow before the
user reruns Basic Energy with explanation outputs.

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
Basic Energy explanation completeness, source availability, relationship rules,
summary/monthly ledger/source/annual edge/reconciliation/warning evidence, file
references, source IDs and related service path IDs for exported explanation
rows, and the source output signatures visible in result tables.
