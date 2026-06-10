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
- HVAC Loop Check planning now uses selected air/plant/condenser loop names to request node-specific outputs instead of wildcard node variables when loop scope is available.
- HVAC Loop Check results now group SQL/CSV system-node state series into a dedicated HVAC Loops tab with per-variable completeness badges.
- Comfort Check now has purpose-scoped zone outputs, a comfort result bundle, and a dedicated Comfort tab for temperature, setpoint, PMV, and PPD series summaries.
- Simulation output discovery now exposes a SQL/RDD/MDD-backed catalog API with purpose-plan fallback entries for future custom output picking.
- Custom Outputs purpose runs can now include user-entered Output:Variable and Output:Meter requests in the run-plan preview and simulation request.
- Custom Outputs setup now includes a discovery catalog picker for adding SQL/RDD/MDD-backed output variables and meters to the run plan.
- Purpose simulation results can now be exported as a JSON bundle containing run metadata, the purpose run plan, parsed purpose results, and output file references.
- Purpose simulation results can now be exported as a standalone HTML report with run metadata, output plan, completeness, file references, and embedded JSON.
- Purpose simulation runs now save `idf-analyzer-run-plan.json` and `temporary_outputs.diff` artifacts alongside EnergyPlus outputs.
- Simulation progress events now distinguish purpose planning, temporary output application, SQL parsing, fallback parsing, and purpose-result bundling phases.
- Basic Energy results now include monthly stacked profile charts and a zone energy heatmap matrix in addition to total bars and ranking tables.
- Basic Energy now surfaces purpose completeness badges alongside HVAC Loop and Comfort result views.
- Simulation output discovery now caches SQL/RDD/MDD catalog reads and invalidates them when source files change.
- Output analysis now tags existing and recommended output requests by simulation purpose and adds a purpose filter to the Output tab.
- HVAC Loop Check results now include node summaries, derived loop metrics, and alert rules for zero flow, missing setpoints, and large setpoint deltas.
- Simulation output discovery now marks purpose outputs as `alias` when a discovered alternate variable can satisfy the requested purpose variable.
- Custom Outputs purpose presets entered or picked in the Simulation setup are now saved locally and restored on the next session.

## Fixed

- _None._
