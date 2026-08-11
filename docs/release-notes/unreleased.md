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

- _None._

## Changed

- Streamlined the main workspace and Summary presentation by removing redundant
  version and metric-count labels, simplifying panel headers, and aligning
  numeric values and units across desktop and responsive layouts.
- Focused the Topology Network on a single zone-level Gross representation.
  Boundary drill-down, area-basis and area-component choices, simulated-flow
  overlays, and inspector Output Request, Diagnostics, and Actions sections were
  removed; Multiplier and Area/UA values are now presented separately in an
  aligned table.
- Updated the bundled Large Office example to the official EnergyPlus v24.2
  source and refined the analysis and Profile controls, including aligned
  Inspect by, Dimensions, and Metric rows.
- Simplified the Topology legend, toolbar, and inspector so the primary zone
  network and source-boundary navigation remain the focus.

## Fixed

- Restored malformed Back, Forward, and Profile pin button markup that could
  appear as literal `??/button>` text in the interface.
- Corrected physical zone exposure and UA completeness handling for exterior,
  ground, interzone, adiabatic, foundation, and invalid boundary relations.
