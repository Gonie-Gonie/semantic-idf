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

- Loop equipment inspection separates three graphs: Availability,
  configured On / Off decisions, and parameter control. Parameter
  control distinguishes external inputs/outputs, manipulated commands and
  calculated states, with signed comparisons only for supported rules.
  Unresolved runtime decisions remain explicit.
- Availability stacks compact ON/OFF patches for related conditions and final
  configured permission. Rep. day/week and Annual share time-axis zoom/pan and
  a cursor for comparing statuses. Manager overrides and unknown runtime inputs
  are preserved; the calendar remains a static inspection basis.
- Energy Path separates thermal gains/losses and storage/release before native
  interval aggregation. Directional cards show one positive gross magnitude with
  gain/loss in the label, including nonzero exchange with zero load. Ribbons
  retain their allocated load contribution.
  Older Monthly-only evidence remains explicitly net-only; new Basic Energy
  runs request Timestep outputs for reversible thermal quantities.

## Fixed

- Default four-purpose simulation results no longer exceed the native WebView
  text limit on the bundled Large Office case: repeated Heat Flow and Thermal
  Topology arrays are transferred once, preserving every value and observation.
  Result-receiving and JSON-decoding failures now have specific diagnostics and
  are identified separately from EnergyPlus failures.
- Repository cleanup removes identified temporary browser profiles, captures
  and the `.runtime/tmp/` scratch workspace while preserving all build outputs
  during normal cleanup.
- Energy Path restores exterior convection and longwave observations from native
  Zone Timestep SQL timestamps, retaining complete Monthly net evidence when a
  gross split is unavailable. Verified ground-contact surfaces expose outside-face
  conduction separately from room-side load drivers.

## Performance

- _None._
