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

- Loop equipment selection now shows a 24-hour availability stack and a compact
  control-sequence diagram. Date and day-type selection applies to all permission
  plots; unresolved runtime conditions remain conditional. The control diagram
  follows inputs through the on/off decision to modulation, including an OFF branch.
- Energy Path separates thermal gains/losses and storage/release before native
  interval aggregation. Directional cards show one positive gross magnitude with
  gain/loss in the label, including nonzero exchange with zero load. Ribbons
  retain their allocated load contribution.
  Older Monthly-only evidence remains explicitly net-only; new Basic Energy
  runs request Timestep outputs for reversible thermal quantities.

## Fixed

- Repository cleanup removes identified temporary browser profiles, captures
  and the `.runtime/tmp/` scratch workspace while preserving all build outputs
  during normal cleanup.
- Energy Path restores exterior convection and longwave observations from native
  Zone Timestep SQL timestamps, retaining complete Monthly net evidence when a
  gross split is unavailable. Verified ground-contact surfaces expose outside-face
  conduction separately from room-side load drivers.

## Performance

- _None._
