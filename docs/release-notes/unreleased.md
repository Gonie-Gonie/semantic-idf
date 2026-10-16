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

- Loop equipment inspection uses compact nested AND/OR and named-rule cards for
  On / Off conditions, with source references and explicit runtime unknowns.
  Parameter control groups inputs/targets, controllers/comparisons, commands
  and equipment response/outputs into source cards, retaining supported signed
  relationships and native Action meanings.
- Availability stacks compact ON/OFF patches for the minimum configured static
  conditions, with owning objects at the left and the combined result at the
  bottom. Supported scheduled-manager rules and plant activation remain;
  runtime managers/load selection are excluded, while configuration errors stay
  unknown. Equipment without a separate schedule has an explicit unrestricted
  row. Rep. day/week and Annual share time-axis zoom/pan and a status cursor.
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
