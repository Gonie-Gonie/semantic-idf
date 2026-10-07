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

- Group desktop analysis, batch metrics and settings methods in feature modules,
  and isolate frontend analysis scheduling in a dedicated queue module.
- Add a documentation index and remove completed temporary investigation artifacts.

## Fixed

- Preserve source-model metadata during EPJSON export so concurrent exports and
  serialization can safely read shared analysis inputs.
- Run the test script from the repository root and propagate Go test failures.
- Stop launching pending analysis stages for stale inputs or after a stage failure.

## Performance

- Share parsed inputs and document indexes across concurrent analysis stages,
  and reuse HVAC, Diagnostics and Output reports for Metrics readiness.
- Index thermal geometry ownership and reduce IDF serialization allocations.
- Reuse browser executable discovery across test harnesses to reduce repeated
  Windows PATH/PATHEXT probes and Go test-cache processing.
- Prepare Energy Path quality availability once per scope and reuse it for
  annual/monthly periods. Local fixture measurements and their limits are in
  `docs/refactoring-performance.md`.
