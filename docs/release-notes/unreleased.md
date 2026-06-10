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

## Fixed

- _None._
