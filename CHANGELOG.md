# Changelog

All notable changes to SemanticIDF are recorded here from release notes.

## [0.4.2] - 2026-06-17

## Highlights Compared With v0.4.1

- v0.4.2 turns the HVAC work from resolver coverage into a more navigable service-analysis surface with object-based zone service graphs, search, cross-tab context actions, graph export, and clearer wire routing.
- The release also focuses on interactive scale: staged analysis, result caching, deferred heavy renders, and cleaner shortcut semantics make large models feel less jumpy and less blocking than v0.4.1.

## Added

- Added a Profile graph deck workflow with schedule similarity matrices, outlier ranking, series selection, exportable graph payloads, and dedicated CLI export commands.
- Added an object-based HVAC Zone Services graph with service navigation indexes, focused entity controls, navigator search/filtering, cross-tab context actions, debug graph export, and bundled service/coupling evidence.

## Changed

- Reworked the HVAC service view around smoother, Grasshopper-style curved wires, lane offsets, selection reset behavior, graph scrolling, compact routing, and clearer hover/selection emphasis.
- Improved analysis responsiveness by caching completed analysis results, deferring inactive/heavy renders, prioritizing visible staged renders, throttling batch progress updates, and exposing timing/status details for staged tabs.
- Split large frontend modules and styles into focused view/style files so HVAC, Profile, Geometry, Output, workspace, and responsive behavior are easier to maintain.
- Refined navigation shortcuts so editor undo/redo restores input positions, HVAC analysis undo/redo restores HVAC view history, Ctrl+PageUp/PageDown cycles analysis tabs, and mouse/browser back-forward controls HVAC history.

## Fixed

- Fixed duplicate HVAC component navigation IDs and strengthened local delivery, coupling, supporting asset, and external-fixture coverage for the service model.
- Reduced startup and interaction stalls by deferring oversized HVAC debug payloads and heavy summary readiness work until the relevant view needs them.

## [0.4.1] - 2026-06-12

## Added

- Added broad HVAC resolver regression coverage for air-loop plenums, SpaceHVAC splitters, air terminals, direct zone equipment, coil systems, unitary systems, plant sources, transport components, and control objects.
- Added EnergyPlus field-catalog entries for additional HVAC families so comment-free IDF objects can still resolve node roles, internal component references, and source-field traces.
- Added a coverage gate that fails when a completed HVAC resolver matrix item has no regression fixture.

## Changed

- Expanded HVAC service-chain and rule-graph resolution around verified RuleGraph paths, component references, connected plant/air systems, and direct zone equipment paths.
- Improved HVAC UI and summary presentation with updated toolbar icons, layout presets, cleaner metric rows, warning/error status styling, and aligned numeric display.

## Fixed

- Fixed synthetic IDF name insertion paths that could corrupt nameless EnergyPlus objects during analysis or run-copy conversion.
- Fixed HVAC relation graph regressions including missing component labels, cartesian service-chain combinations, weak/inferred wording leakage, and incomplete terminal/plant cross-loop traces.

## [0.4.0] - 2026-06-11

## Highlights Compared With v0.3.0

- v0.4.0 expands SemanticIDF from the v0.3.0 output/simulation foundation into a purpose-driven inspection workflow with richer simulation results, batch analysis, and traceable semantic diagnostics.
- The former Tools surface is now a full Batch workspace, while Simulation is now organized around purpose presets, scoped run plans, SQL-first result bundles, and exportable evidence.

## Added

- Added a Batch workspace with Summary delta comparison, Diagnose matrix, Output QA, Convert / Export, Batch Simulation purpose controls, Cleanup Report surfaces, CSV export, and raw/delta XLSX workbook export.
- Added purpose-driven Run & Inspect simulation presets for Basic Energy, Zone Heat Flow, HVAC Loop Check, Integrity Check, Comfort Check, and Custom Outputs, including run-plan previews, output weight estimates, temporary run-copy artifacts, and run manifests.
- Added purpose result bundles with JSON and standalone HTML export, including Energy, Zone Heat Flow, HVAC Loop, Comfort, Integrity, output-plan, completeness, file-reference, and embedded raw-result sections.
- Added Basic Energy result dashboards with SQL meter and zone-energy parsing, monthly aggregation, converted kWh totals, monthly stacked charts, zone energy heatmap matrices, metric selectors, and completeness badges.
- Added Zone Heat Flow result completeness by heat-balance category, range/zoom playback synchronization, timeline brush controls, spatial zoom/pan, and collapsible zone ledger inspection.
- Added HVAC Loop Check purpose scoping for selected loops/components, node and component operation outputs, frame snapshots, compact loop schematics, topology links, multi-series charts, variable-group filters, and reported-vs-derived loop metrics.
- Added Comfort Check scoped result views with temperature/setpoint/humidity/heating/cooling timelines, comfort period filters, unmet-hour summaries, setpoint-deviation ranking, and metric-level completeness.
- Added Integrity purpose results that combine static Diagnose issues, EnergyPlus ERR messages, SQL diagnostics, tabular report previews, and SQL/static cross-checks for zones, surfaces, constructions, and nominal loads.
- Added simulation output discovery from SQL/RDD/MDD sources, including cached catalog reads, fallback/alias state, meter metadata, custom output search, missing-output marking, and locally persisted custom output presets.
- Added richer semantic YAML, Summary, Diagnose, and HVAC evidence surfaces, including source/confidence metadata, Basic/Detailed/Source modes, search facets, section jumps, schema adapter metadata, Space/Zone list views, HVAC occurrence paths, and source/debug badges.

## Changed

- Reorganized the old Tools page into Batch and moved current-file cleanup/fix workflows into Diagnose / Fixes, keeping batch work and single-file repair work separate.
- Reframed Simulation empty states and controls around purpose-driven Run & Inspect workflows instead of the v0.3.0 standard-output-first flow.
- Expanded SQL-first simulation parsing so one combined, timeout-aware parser gathers series, energy, heat-flow, integrity, comfort, source-priority, normalized units, display values, and statistics while preserving partial results.
- Improved purpose run planning with frequency conflict policies, selected/visible/filtered zone scopes, selected HVAC loop/component scopes, discovery dictionary outputs, heavy/very-heavy output warnings, and persisted-output state.
- Upgraded HVAC analysis from broad field scanning to typed component/reference graphs with plant, condenser, air-loop demand path, splitter, mixer, plenum, terminal, SpaceHVAC, node-list, and source-field provenance.
- Improved analyzer UI density and navigation with compact summary rows, grouped diagnostic accordions, suppression filters, graph fit/scale controls, collapsible HVAC inspectors, denser relation tables, and long-series time-range controls.
- Aligned EnergyPlus schema handling more closely with IDD/schema metadata by preserving `legacy_idd` field order and field names when loading schema adapters.
- Updated simulation documentation, release notes, and regression coverage around purpose scopes, SQL aggregation, discovery aliases, run artifacts, ReadVarsESO policy, SQL fixtures, run-copy output injection, and purpose result exports.

## Fixed

- Fixed app startup paths so sample loading and main interface loading fall back cleanly instead of leaving the UI stuck at the loading message.
- Fixed SQL/CSV series rendering when no current series is selected or a series list contains empty values, preventing `displayPoints` null-access errors.
- Fixed purpose simulation run-copy conversion so nameless `OutputControl:*` objects do not receive synthetic Name fields that cause EnergyPlus IDD parsing failures.
- Fixed purpose output application to avoid duplicate unique tabular output objects and to merge extensible unique outputs such as `Output:Table:SummaryReports` and `Output:Diagnostics`.
- Fixed incomplete SQLite handling so purpose SQL parsers can fall back to CSV/ESO or partial results instead of failing the whole result bundle.
- Fixed SQL result parsing cancellation and timeout behavior so long parse phases can stop between parser steps.
- Fixed SQL-first ReadVarsESO behavior so `-r` is used only for explicit fallback or legacy CSV modes.
- Fixed HVAC Loop Check cooling/heating classification so `CHW` and chilled-water node names are recognized as cooling before generic `HW` heating token matching.

## [0.3.0] - 2026-06-09

## Added

- Added an Analyze Output tab for reviewing existing `Output:*` and `OutputControl:*` requests, adding recommended output presets, and previewing edits/removals before applying them to the input.
- Added EnergyPlus simulation execution support with external executable detection/registration, weather-file discovery, AppData-backed run directories, single-run progress, ERR parsing, CSV summaries, and graphable result series in the Analyze Simulation tab.
- Added Simulation settings for EnergyPlus installations, extra weather folders, run-directory defaults, auto-run-on-open, and parallel worker defaults.
- Added a Tools Multiple Simulation workflow for selecting files or recursive folders, running EnergyPlus in parallel, using shared or nearest-folder weather files, sorting result tables, and overlaying selected CSV output series.
- Added standard output request presets for simulation-dependent analysis, including heat-flow ledger variables and warnings when a pending run will not be able to show those panels.
- Added a Heat-Flow Ledger simulation output view with floor-plan zone overlays, a sliding annual timeline, hover details, and stacked per-zone heat-flow plots.
- Added semantic YAML as the first input panel, including object-range helpers, source-object metadata, duplicate/conflict handling, sticky hierarchy context, compact schedule abstractions, and zone/HVAC/output-centered rendering.
- Added CLI commands for summary, multi-summary, diagnostics, full analysis, cleanup, IDF/epJSON/semantic YAML conversion, and styled single-sheet XLSX table exports.

## Changed

- Moved canonical frontend assets from `frontend/dist` to `frontend/src` and reserved `frontend/dist` for future generated build output.
- Refined the Output analysis UI into request-map and preset-library tables so it manages output relationships instead of mirroring the raw object list.
- Refactored simulation code into an `internal/simulation` package and aligned execution with the detected IDF EnergyPlus version.
- Switched simulation output parsing to SQL-first reads for better speed and stability while retaining fallbacks where needed.
- Made standard simulation outputs enabled by default and collapsed simulation analysis panels before a run, with capability labels that explain which views the current options can produce.
- Reworked semantic YAML output as a zone/HVAC/output-centered view export with source name conflicts separated from semantic duplicate appearances.
- Tightened semantic YAML typography, line density, section spacing, colon/value alignment, and scroll context so the panel reads more like a purpose-built editor than raw YAML text.
- Clarified heat-flow map markers and hover payloads so zone-level net heat flow, temperature, and component contributions are easier to interpret.

## Fixed

- Fixed EnergyPlus simulation output prefixing so runs produce standard `eplusout.*` files and ReadVarsESO post-processing can find the expected ESO/MTR outputs.
- Fixed simulation analysis tab switching after run-state updates.
- Fixed cache-like confusion around standard-output runs by treating missing output prerequisites as explicit capability state instead of only showing a generic missing-data message.
- Fixed semantic YAML edge cases for zone/space lists, compact schedules, output-file requests, wildcard/inherited outputs, surface boundary/exposure metadata, HVAC loop nodes, and conflicting source object names.

## Commit-by-Commit Update Map

- `67c87a3` moved canonical frontend assets from `frontend/dist` to `frontend/src` and updated build/release paths.
- `9ed8ce8` added output request analysis, standard output recommendations, preview/apply plumbing, and tests.
- `03e4258` refined the output request management UI into clearer request-map and preset-library views.
- `a33c538` added the EnergyPlus simulation backend and Wails API entry points.
- `0c5b518` added the Analyze Simulation results tab with graphable output-series support.
- `3995c02` added simulation settings for executable, weather, run directory, auto-run, and worker defaults.
- `08aece9` added the Tools Multiple Simulation workflow for batch EnergyPlus runs.
- `743b37e` documented the new simulation workflow in release notes.
- `7e1d2f7` verified the EnergyPlus smoke path and added external simulation tests.
- `43afccd` refactored simulation code into `internal/simulation` and split the multi-simulation frontend module.
- `f586366` clarified simulation run-state handling and platform command execution.
- `e7cd4dc` fixed simulation analyze-tab switching.
- `874becd` matched the selected simulation engine to the IDF version.
- `340c539` added standard-output requirement plumbing, preset application, and simulation prerequisite reporting.
- `33400bb` added heat-flow ledger outputs, parsing, floor overlays, and stacked component plots.
- `da2a467` added semantic YAML input/export support, CLI commands, table XLSX export, and guide coverage.
- `2e390dd` refined semantic YAML and heat-flow UI readability.
- `3f2e28a` switched simulation output parsing toward SQL for speed and stability.
- `db7b95d` reworked semantic YAML structure around zones, HVAC, outputs, and source-name conflicts.
- `1638e03` tightened semantic YAML spacing, sticky hierarchy context, and schedule-rule display.
- `9b07d0b` completed the semantic YAML checklist with builder/renderer separation and golden coverage for zones, schedules, surfaces, HVAC loops, outputs, and conflicts.

## [0.2.0] - 2026-06-01

## Added

- Profile Analysis for normalized internal-load, ventilation, outdoor-air, schedule pattern, matrix, graph, and apply-preview workflows.
- HVAC analysis views for loop diagrams, system-zone relation graphs, connection diagnostics, cross-loop navigation, node inspection, and output-variable guidance.
- User-configurable Analyze tab order, GUI language selection, keyboard shortcuts, view-location undo/redo, and definition/reference jumps in text, JSON, and table input views.
- Fullscreen-style in-app expansion for HVAC and Geometry views plus geometry selection aids and construction layer graphics.

## Changed

- Refined the main UI sizing, scroll containers, visual hierarchy, profile graph focus, HVAC selectors, and geometry detail panels for denser analysis workflows.
- Updated release automation to validate app version consistency across Wails metadata, bundled app info, and static HTML placeholders.

## Fixed

- Restored native scroll-axis behavior so vertical and horizontal scrolling stay visually predictable.
- Reduced release/version drift risk by updating static page version labels during release preparation.
- Reused translated status messages for input navigation and field updates.

## [0.1.0] - 2026-06-01

## Added

- Package SemanticIDF as a Wails desktop app with a static HTML/CSS/JS frontend.
- Parse, edit, summarize, diagnose, and clean up EnergyPlus IDF and epJSON inputs.
- Add geometry visualization, summary exports, multi-IDF comparison, bundled sample input, and in-app guide/settings pages.
- Add manual GitHub release automation driven by release notes, including semver selection, changelog updates, packaging, tagging, and GitHub Release publishing.
- Publish GitHub Releases automatically when version tags are pushed.
- Show the app version in the window title, page headers, Settings, release package name, and built executable filename.
