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

- Energy Path separates thermal gains/losses and storage/release before native
  interval aggregation. Directional cards distinguish gross heat exchange from
  allocated load contribution, including nonzero exchange with zero load.
  Older Monthly-only evidence remains explicitly net-only; new Basic Energy
  runs request Timestep outputs for reversible thermal quantities.

## Fixed

- _None._

## Performance

- _None._
