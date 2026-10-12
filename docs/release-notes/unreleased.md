# Unreleased Release Notes

## Highlights Compared With v0.5.0

- Preserve the Main workspace and analysis results while opening Settings, Guide
  or Tools as in-app panels.
- Expand Energy Path with measured boundary inputs, auxiliary-service paths,
  clearer monthly comparisons and readable ratio charts.
- Improve Heat Flow maps, exchange arrows, signed ledgers and continuous playback.
- Replace the lightweight Guide with a searchable English/Korean technical manual,
  and add visible storage usage and cleanup controls.

## Added

- Add a ten-chapter Markdown technical reference manual with chapter and section
  navigation, whole-manual search, stable links and an offline metric catalog.
- Show simulation-run disk usage in Settings, with age-filtered cleanup and an
  opt-in Always clean mode that protects active results and user files. Show
  browser/runtime storage separately without clearing it.
- Add measured simulation-processing progress and session-based remaining-time
  ranges for matching completed runs.
- Add Energy Path boundary observations for incident solar, exterior convection,
  exterior longwave exchange, people, lighting and equipment, plus validated
  cooling/heating associations with fans, pumps and HVAC auxiliaries.
- Show native simulation failure causes on hover and in accessible Failure details,
  including EnergyPlus messages, process output and run context.

## Changed

- Keep Main mounted behind auxiliary panels, retaining current results, selection
  and view state.
- Organize Settings into responsive sections with a persistent save bar, reliable
  unsaved-change status and default Input/Profile view preferences.
- Retain all six app languages with per-key English fallback; improve Korean
  coverage and synchronize dynamic views after language changes. Guide uses
  Korean or English according to the app language.
- Simplify Energy Path scope selection to All or an individual Zone. Use consistent
  Sankey ribbons, grouped internal gains, more specific heat-balance categories,
  and responsive endpoint/ratio inspection panels.
- Restore compact fixed Heat Flow plans with numbered zones, one arrow per visible
  zone pair and one combined Outside arrow. Improve colour contrast, signed rate
  bars, exact values and chart typography.
- Restrict HVAC circuit annotations to objects in the selected circuit and align
  inspection-chart styling across result panels.

## Fixed

- Automatically run ExpandObjects for IDF models containing HVACTemplate or
  GroundHeatTransfer objects, and read results against the verified expanded model
  while preserving the source input.
- Restore valid monthly Energy Path input/driver comparisons and distinguish
  missing observations, reported zeroes and unusable ratio denominators.
- Keep ratio axes linear and exclude extreme outliers from automatic limits,
  retaining their original values in off-scale marks and tooltips.
- Preserve every original Heat Flow frame instead of sampling the result; playback
  advances one source frame at a time.
- Preserve Profile time-view settings during form edits, write configuration files
  atomically and synchronize settings across app panels.
- Preserve source-model metadata during concurrent EPJSON exports and stop stale
  or failed analysis queues.
- Preserve diagnostic status text during timing and language refreshes.
- Run the test script from the repository root and propagate Go test failures.

## Performance and Development

- Share parsed inputs and document indexes across analysis stages, reuse reports
  for Metrics readiness and index thermal-geometry ownership.
- Skip Simulation environment discovery when only appearance settings change.
- Reuse Energy Path availability calculations and cached Heat Flow layouts; update
  current playback values without rebuilding static history charts.
- Reduce IDF serialization allocations and reuse browser executable discovery
  across test harnesses.
- Classify existing tests by feature, layer and runtime tier without relocating
  them. Development checks select a fast baseline plus affected regressions;
  releases retain mandatory full verification.
- Add `dev clean-repo` with preview support, protected tracked files and checked
  path boundaries. Normal cleanup preserves `build/bin` and installed tools;
  `dev clean-repo --hard` also resets generated builds and the repo-local toolchain
  before another `dev setup`.
- Consolidate developer documentation and remove superseded progress logs.

## Compatibility and Limits

- Older simulation results require regeneration to recover previously sampled
  Heat Flow frames or obtain newly requested boundary observations and verified
  surface-exchange arrows.
- Boundary-input ribbons are reference associations, not conserved heat-transfer
  paths or conversion efficiencies. Load-driver values remain allocated
  contributions to actual cooling/heating loads.
- Remaining-time estimates use matching measurements from the current app session;
  new or changed runs start with unknown time.
- Template-based epJSON requires conversion to IDF for ExpandObjects.
