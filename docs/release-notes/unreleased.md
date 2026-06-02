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

- Added an Analyze Output tab for reviewing existing `Output:*` and `OutputControl:*` requests, adding recommended output presets, and previewing edits/removals before applying them to the input.
- Added EnergyPlus simulation execution support with external executable detection/registration, weather-file discovery, AppData-backed run directories, single-run progress, ERR parsing, CSV summaries, and graphable result series in the Analyze Simulation tab.
- Added Simulation settings for EnergyPlus installations, extra weather folders, run-directory defaults, auto-run-on-open, and parallel worker defaults.
- Added a Tools Multiple Simulation workflow for selecting files or recursive folders, running EnergyPlus in parallel, using shared or nearest-folder weather files, sorting result tables, and overlaying selected CSV output series.

## Changed

- Moved canonical frontend assets from `frontend/dist` to `frontend/src` and reserved `frontend/dist` for future generated build output.
- Refined the Output analysis UI into request-map and preset-library tables so it manages output relationships instead of mirroring the raw object list.

## Fixed

- Fixed EnergyPlus simulation output prefixing so runs produce standard `eplusout.*` files and ReadVarsESO post-processing can find the expected ESO/MTR outputs.
