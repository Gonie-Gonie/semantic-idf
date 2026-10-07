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

- Show simulation-run disk usage in Settings and clean verified completed runs
  with all/7/30/90-day filters, while protecting active results, user files and
  linked paths. Show browser/runtime storage separately without clearing it.
- Offer opt-in Always clean for unused completed runs at startup, after result
  replacement and at app shutdown, retaining current results while in use.
- Add a default Input View preference and expose the existing Profile time-view
  default in Settings.
- Add `dev clean-repo` to remove regenerable development files while preserving
  `build/bin` and installed tools, and `--hard` to reset generated files and the
  repo-local toolchain before another `dev setup`. Include `-WhatIf` previews,
  with Git-tracked files and path boundaries protected.
- Add a bundled English/Korean technical reference manual with ten Markdown
  chapters, chapter/section navigation, whole-manual search and stable deep links.
- Include a registry-generated offline metric catalog alongside the live desktop
  definitions, with units, methods, assumptions and missing-data interpretation.

## Changed

- Organize Settings into focused sections with responsive cards, current-section
  navigation, a persistent save bar and accurate unsaved-change status.
- Separate the six app translation catalogs from the language engine, with
  per-key English fallback, complete Korean interface text and retained
  industry terminology. Guide follows Settings, with Korean metric descriptions
  and English content for the other app languages.
- Group desktop analysis, batch metrics and settings methods in feature modules,
  and isolate frontend analysis scheduling in a dedicated queue module.
- Add a documentation index and remove completed temporary investigation artifacts.
- Classify existing tests by feature, frontend/backend layer and test tier without
  moving test files. Use one runner for development, staged commit checks and
  mandatory full release verification.
- Consolidate Energy Path, Topology and performance references, retain current
  development contracts and evidence, and remove superseded progress logs.

## Fixed

- Preserve Profile time-view settings during form edits and replace settings
  files atomically so concurrent readers receive complete configuration JSON.
- Refresh dynamic labels, status messages, accessible text and open detail
  views after language changes, preserving selection and technical source data.
- Synchronize language settings across app pages and ignore stale settings loads.
- Preserve source-model metadata during EPJSON export so concurrent exports and
  serialization can safely read shared analysis inputs.
- Run the test script from the repository root and propagate Go test failures.
- Stop launching pending analysis stages for stale inputs or after a stage failure.

## Performance

- Skip Simulation environment discovery when only appearance settings change.
- Share parsed inputs and document indexes across concurrent analysis stages,
  and reuse HVAC, Diagnostics and Output reports for Metrics readiness.
- Index thermal geometry ownership and reduce IDF serialization allocations.
- Reuse browser executable discovery across test harnesses to reduce repeated
  Windows PATH/PATHEXT probes and Go test-cache processing.
- Prepare Energy Path quality availability once per scope and reuse it for
  annual/monthly periods. Local fixture measurements and their limits are in
  `docs/performance.md`.
- Run the fast test baseline plus affected feature regressions during development
  and commits, with full-suite fallback for shared or unclassified code changes.
  Avoid requiring duplicate full-suite runs for every implementation pass.
