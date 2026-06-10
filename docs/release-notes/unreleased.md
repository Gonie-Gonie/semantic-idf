# Unreleased Release Notes

<!--
Add release-note entries under the section that best describes the change.
The release script infers bump size from these sections:
- Breaking Changes: major
- Added or Features: minor
- Fixed, Changed, Performance, Security, Documentation, or internal-only notes: patch
-->

## Breaking Changes

- _None._

## Added

- Semantic YAML now exposes EnergyPlus compatibility adapter metadata, split ZoneList/SpaceList/ZoneGroup views, zone Spaces, richer output attachment resolution, and expanded HVAC loop metadata.
- Diagnose, Summary, HVAC, and semantic exports now expose source, confidence, and evidence metadata for inferred or analyzer-limited results.

## Changed

- HVAC analysis now treats Branch components as EP26-style 4-field groups by default and includes CondenserLoop, connector/branch rule diagnostics, and ZoneHVAC equipment sequence metadata.
- Summary and HVAC UI views now surface confidence/source badges, grouped diagnostic sources, HVAC component families, and a compact/full semantic YAML toggle.
- The analyzer workspace, HVAC relation view, and simulation graphs now provide denser layout presets, a zone-first relation table, and time-range controls for long SQL/CSV and heat-flow outputs.
- HVAC component family and role labels now distinguish pipes, cooling towers, heat pumps, water heaters, controls, and condenser/plant/air-side context in both semantic exports and the UI.
- Semantic YAML now defaults to a Basic outline, adds Detailed and Source/debug modes, and provides section jump buttons with narrower wrapping behavior for split-pane use.
- Diagnose now groups issues into severity/workflow accordions, supports severity/source filters, and lets users hide noisy diagnostic codes while reviewing source evidence.
- AirLoopHVAC demand paths now expose a structured SupplyPath/ReturnPath graph with splitter, mixer, plenum, and terminal evidence for zone relations and semantic exports.
- HVAC plant relation inference now uses a typed component reference graph with source field metadata instead of blindly scanning every component field.
- Heat-flow simulation now adds a mini timeline brush with reset/day/week/month/run-period shortcuts and Shift+wheel panning for visible frame ranges.
- Purpose simulation regression coverage now verifies run-copy output injection preserves the source IDF file and permanent purpose output edits flow through the Output apply pipeline.
- Simulation regression coverage now locks the SQL-first ReadVarsESO policy so `-r` is only used for explicit fallback or legacy CSV modes.
- SQL parser regression coverage now directly checks ReportDataDictionary/Time/ReportData joins for series names, labels, units, and statistics.
- Summary rows now switch long, inferred, advanced, and readiness metrics into compact two-line rows for narrow analysis panes.
- Diagnose regression tests now include golden snapshots for valid baseline, design-day RunPeriod notices, output wildcard/environment keys, and Schedule:Compact tokens.
- HVAC loop and relation graphs now include Fit, 100%, and Compact scale controls for narrow panes and detailed graph inspection.
- HVAC inspector panels can now be collapsed from the toolbar, and component labels expose full tooltips with direct inspector selection.
- Heat-flow floor plans now support independent spatial zoom/pan controls and a collapsible zone ledger inspector.
- HVAC analysis now builds separate SpaceHVAC equipment relations with parent-zone links, node-list provenance, splitter evidence, and UI labels.
- HVAC node diagnostics now use typed loop component edges, avoid global disconnected-loop false positives, and treat outdoor/relief boundary nodes as expected one-sided nodes.
- Summary HVAC node connection counts now use typed loop component connection pairs instead of sequential field fallback inference.
- Semantic HVAC duplicate markers now list actual loop, zone relation, demand graph, and catalog occurrence paths instead of only pointing back to the catalog entry.
- Semantic YAML search now includes facet buttons for source fields, editable values, derived metadata, and evidence/debug blocks while preserving parent path context.
- Summary conditioned-zone metrics now expose evidence breakdowns for equipment connections, ZoneHVAC objects, thermostats, and SpaceHVAC references, with inferred confidence metadata.
- Summary footprint area now remains floor-surface based, while XY bounding-box area is reported as a separate advanced inferred metric.
- Summary envelope area is now labeled as gross, with a separate net opaque envelope area metric that subtracts recognized fenestration.
- Semantic YAML lines now show Raw, Computed, Summary, and Inferred badges so derived values are visually distinct from editable IDF fields.
- Summary internal-load metrics now report resolved object coverage and unresolved calculation-method counts for People, Lights, and ElectricEquipment.
- HVAC terminal inlet mismatch warnings now expose relation edge ids, source field indexes, expected/actual nodes, and fix targets for UI inspection.
- Unresolved HVAC branch and zone equipment components now retain source owner and type/name field metadata across reports, semantic YAML, and the UI.
- HVAC regression coverage now includes zone-only four-pipe fan coil equipment and service/process plant loop notice behavior.
- Summary WWR and skylight metrics now distinguish computed azimuth evidence from unresolved fenestration base-surface cases.
- Semantic YAML Basic mode now enforces a 250-line budget for large models, with backend line-count metadata and regression coverage.
- Semantic schedule definitions now summarize used-by references with counts, role groups, top examples, and truncated counts instead of expanding every reference inline.
- Simulation runs now expose a purpose-driven request, run-plan preview model, SQL-first result mode controls, and purpose result bundle shell while preserving existing Series and Heat-Flow viewers.
- Purpose simulation planning now builds Basic Energy, Zone Heat Flow, Integrity, Comfort, HVAC Loop Check, and Custom Outputs presets with signature-based output merging and warning metadata.
- The Simulation tab is now framed as Run & Inspect with purpose cards, scope controls, run-plan preview, and a permanent purpose-output apply action.
- Basic Energy purpose results now parse EnergyPlus SQL meter and zone-energy rows directly, including J-to-kWh conversion and SQL source completeness metadata.
- Simulation results now include purpose-aware Energy, Heat-Flow, Integrity, Series, and Files tabs with a Basic Energy dashboard for SQL monthly energy output.
- Simulation runs now write an `idf-analyzer-run.json` manifest with input hash, selected purposes, output plan, engine/weather paths, and result file metadata.
- Purpose run planning now applies `preserve`, `highest_resolution`, and `purpose_default` frequency conflict policies instead of only flagging conflicts.
- Purpose run-plan estimates now use IDF `RunPeriod` and `Timestep` objects so short-run and timestep-heavy simulations show more realistic frame counts.
- Integrity purpose results now read SQL `Errors` and `TabularDataWithStrings` diagnostics and preview key tabular reports alongside ERR output.
- Integrity results now include a filter for SQL diagnostics and tabular report names, columns, rows, and values.
- Integrity ERR issues are now grouped by severity/message with repeat counts and expandable line details.
- HVAC Loop Check planning now uses selected air/plant/condenser loop names to request node-specific outputs instead of wildcard node variables when loop scope is available.
- HVAC Loop Check results now group SQL/CSV system-node state series into a dedicated HVAC Loops tab with per-variable completeness badges.
- Comfort Check now has purpose-scoped zone outputs, a comfort result bundle, and a dedicated Comfort tab for temperature, setpoint, PMV, and PPD series summaries.
- Comfort Check now shows a selected-zone timeline with temperature, setpoint band, humidity, heating/cooling rate bars, and setpoint-deviation highlights.
- Simulation output discovery now exposes a SQL/RDD/MDD-backed catalog API with purpose-plan fallback entries for future custom output picking.
- Custom Outputs purpose runs can now include user-entered Output:Variable and Output:Meter requests in the run-plan preview and simulation request.
- Custom Outputs setup now includes a discovery catalog picker for adding SQL/RDD/MDD-backed output variables and meters to the run plan.
- Purpose simulation results can now be exported as a JSON bundle containing run metadata, the purpose run plan, parsed purpose results, and output file references.
- Purpose simulation results can now be exported as a standalone HTML report with run metadata, output plan, completeness, file references, and embedded JSON.
- Purpose simulation runs now save `idf-analyzer-run-plan.json` and `temporary_outputs.diff` artifacts alongside EnergyPlus outputs.
- Simulation progress events now distinguish purpose planning, temporary output application, SQL parsing, fallback parsing, and purpose-result bundling phases.
- Basic Energy results now include monthly stacked profile charts and a zone energy heatmap matrix in addition to total bars and ranking tables.
- Basic Energy now surfaces purpose completeness badges for each requested energy meter and zone-energy variable alongside HVAC Loop and Comfort result views.
- Zone Heat Flow now reports purpose completeness for temperature and each heat-balance category instead of only a single ledger-level status.
- Comfort Check now includes relative humidity and sensible heating/cooling rate outputs, requests unmet-hours summary reports, exports metric-level completeness, and ranks zones by setpoint-band deviations.
- Basic Energy zone reported energy now has a matrix metric selector with tooltips showing the source variables and files behind each cell.
- Simulation output discovery now caches SQL/RDD/MDD catalog reads and invalidates them when source files change.
- Output analysis now tags existing and recommended output requests by simulation purpose and adds a purpose filter to the Output tab.
- HVAC Loop Check results now include node summaries, derived loop metrics, and alert rules for zero flow, missing setpoints, and large setpoint deltas.
- Simulation output discovery now marks purpose outputs as `alias` when a discovered alternate variable can satisfy the requested purpose variable.
- Custom Outputs purpose presets entered or picked in the Simulation setup are now saved locally and restored on the next session.
- Basic Energy SQL results now aggregate sub-monthly energy rows into monthly chart points while preserving converted kWh totals.
- Basic Energy SQL unit conversion policy now has explicit coverage for J/kJ/MJ/GJ/Wh/kWh/W inputs and converted monthly totals.
- Purpose result tables now show the matched source output state and run-plan signature for zone energy, comfort, and HVAC loop series rows.
- Simulation runner documentation now reflects purpose scopes, SQL monthly aggregation, discovery alias/cache states, result source signatures, and run export artifacts.
- HVAC Loop Check purpose runs now include selected-loop component operation outputs and group parsed fan, pump, coil, chiller, boiler, and cooling-tower series in the HVAC result view.
- HVAC Loop Check purpose scope now follows the selected HVAC graph component and limits component/node outputs to that component when available.
- SQL/CSV Series charts now include a variable-group filter for temperature, mass flow, setpoint, rate/load, power/energy, and other series.
- Custom Outputs now appear as chart-ready links in the Series tab, including wildcard variable matches and direct meter-name matches.
- Purpose output permanent apply now has explicit modes for adding missing outputs, replacing conflicts, keeping existing outputs while adding purpose duplicates, or removing selected purpose outputs.
- Purpose result HTML export now includes Energy, Zone Heat Flow, HVAC node/component, and Comfort summary tables before the embedded raw bundle.
- Simulation output discovery now treats purpose meters as first-class fallback/alias entries, including `NaturalGas:*` and `Gas:*` meter aliases.
- Simulation output discovery now structures MDD meter catalog entries with resource type, end-use category, and meter group metadata for Basic Energy availability checks and custom output search.
- Purpose result source rows now include a Chart action that opens the matched SQL/CSV series in the common time-range chart for zone energy, HVAC node/component, and comfort metrics.
- The SQL/CSV Series result tab now exposes visible All/Start/End time-range controls that stay synchronized with wheel zoom and row-driven chart inspection.
- Zone Heat Flow playback now uses the active purpose heat-flow dataset and keeps explicit zoom start/end plus visible frame index state in sync with the range controls.
- Simulation empty-state and advanced-option labels now frame legacy standard-output presets as secondary to the purpose-driven Run & Inspect flow.
- Discovery-enabled purpose run plans now request EnergyPlus output dictionaries so RDD/MDD catalog generation is part of the temporary run plan.
- HVAC Loop Check results now include a frame slider with node and component snapshots for inspecting selected-time temperatures, setpoints, flows, and operation values.
- Simulation output discovery now marks user-entered Custom Outputs as `missing` when they are not found in the current SQL/RDD/MDD catalog.
- Purpose simulation run manifests now include EnergyPlus version metadata when it can be resolved from configured or detected installations.
- Purpose zone scopes now support visible and filtered zone modes, letting Heat Flow and Comfort outputs narrow to geometry/profile-derived zone names.
- Purpose result HTML export now includes Integrity ERR, SQL diagnostics, and tabular report summary tables before the embedded raw bundle.
- Integrity purpose results now connect static Diagnose issues with EnergyPlus ERR and SQL diagnostics in the result view and exported bundle.
- Purpose run-plan regression tests now include golden snapshots for Basic Energy, Zone Heat Flow, HVAC Loop Check, and Integrity presets.
- Simulation run results and manifests now record SQL, CSV, and ESO source priority plus the actual sources used by result parsers.
- Comfort Check now supports custom period scope inputs that filter comfort trends, summaries, issue ranking, and HTML exports to the selected date range.
- Purpose run-plan tests now lock the `will_be_persisted` state shown when purpose outputs are planned as permanent edits.
- Integrity purpose results now cross-check SQL tabular zone, surface, construction, and nominal-load rows against static IDF names with exact, normalized, alias, static-only, and SQL-only statuses.
- SQL and CSV series now carry normalized display units and values so J/Wh energy, W rates, temperatures, mass flow, and humidity ratio render consistently across charts and purpose result tables.
- HVAC Loop Check results now classify loop operating status and add node-state-derived air/water heat-transfer estimates with reported-vs-derived labels in the viewer and HTML export.
- Purpose run plans now emit Heavy and Very Heavy output-weight warnings with series/frame counts, with regression coverage for large selected-zone models.
- HVAC Loop Check frame snapshots now include a compact node schematic with live temperature/flow labels before the detailed node and component cards.
- HVAC Loop Check results now provide snapshot/chart panel toggles and variable-group filters for the normalized multi-series chart and node/component result tables.
- HVAC Loop Check results can now open the selected HVAC tab loop topology using the shared `renderHVACLoopDiagram` renderer while keeping live frame overlays in the compact simulation schematic.

## Fixed

- SQL result parsing now has a combined entrypoint that gathers series, energy, heat-flow, integrity, and comfort unmet-hour summaries while preserving partial parse results.
- SQL result parsing now runs through a timeout-aware context entrypoint so long parse phases can be cancelled between parser steps.
- Purpose SQL parsers now treat incomplete SQLite result files as empty results so CSV/ESO fallback and partial purpose result handling can continue cleanly.
- HVAC Loop Check status classification now treats `CHW`/chilled-water node names as cooling before matching generic `HW` heating tokens.
