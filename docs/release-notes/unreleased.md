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

## Fixed

- _None._
