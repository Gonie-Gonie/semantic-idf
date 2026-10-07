# Performance and reproducible diagnosis

Start from the [architecture map](architecture.md) to locate a stage and use
[testing.md](testing.md) for ordinary verification. Measure engine execution,
output reading, graph construction, transport and rendering separately. A stage
percentage reports completed stages; it is not a remaining-time estimate.

## Current implementation boundaries

| Stage | Work already shared or avoided | Semantics to preserve |
| --- | --- | --- |
| Input analysis | Three cached parsed snapshots; concurrent parse deduplication; one lazy read-only index per snapshot; session dependency sharing | Editing owns fresh documents; export does not mutate model metadata |
| Thermal geometry | Source-ordered ownership indexes for surfaces, spaces and boundaries | Unicode `EqualFold` matching, significant whitespace, accumulation/output order |
| IDF serialization | Pre-sized builder rather than per-field formatting | Original field and comment formatting |
| SQL reading | Cached dictionary/Time metadata and compact observation streams; distinct observed IDs before metadata joins | Read-only SQL, row order, NULL/zero sources, source priority, filters and unusual-schema JOIN fallback |
| Alias lookup | Bounded indexes for fixed catalogs | First-match precedence, normalization, dynamic meter fallback and independently owned slices |
| Energy drivers | Unit/rate conversion, surface category and membership prepared per dictionary | Original SQL ordering, arithmetic, rounding and full observation arrays |
| Energy Path quality | Run-level evidence prepared once per Building/Zone scope and shared by annual/monthly periods | Period-local closure/ratios, source IDs, scope completeness and fresh evidence on refresh |
| Desktop result transport | Exact column/timeline deduplication, negotiated HTTP response, lazy frontend point arrays and compact workspace snapshots | Value bits including negative zero, timestamp gaps, labels, sample order and request identity |
| Browser tests | Discover Chrome once per test process | Existing browser scenarios; avoid repeated PATH/PATHEXT probes in Go test-cache logs |

The public SQL walker retains its original interface. TEXT/noninteger metadata,
one-to-many joins and unusual Time IDs use compatible fallback behavior; compact
reading must not invent an integer timestamp or modify the user's database.
Series limits and sampling remain part of the reader contract. The combined
`parseSimulationSQL` entrypoint retains its 20-second deadline checked between
phases; the runner's independent initial Series/Heat Flow read does not use that
deadline. A completed SQL parse is the proper baseline when a previously
timed-out parse had silently selected CSV/ESO instead.

Transfer negotiation uses `semantic-idf.simulation-transfer/v1`. Unnegotiated and
small responses retain their ordinary result shape. Compact references are
collision-checked and exactly restored; unopened loops keep numeric columns.
A newer run cannot replace another request's response, and a failed POST is
not retried through a second transport. Receiving/display failures are separate
from engine completion. See [simulation-runner.md](simulation-runner.md).

Keep worker pools bounded. Eliminating duplicate work should precede adding
parallel copies of large results. Zone summaries can share backing data; do not
parallelize mutation without proving ownership. Annual Profile series remain a
material allocation cost and need their own evidence before changing rounding.

## Recorded baselines

These are dated local measurements with the repository Go runtime on Windows,
not current machine guarantees or an application latency SLA. Allocation figures
are cumulative allocations, not peak resident memory.

| Date and measured work | Before | After | Fixture and validation boundary |
| --- | --- | --- | --- |
| 2026-10-07: four-stage input preparation | 63-68 ms; 31.54 MB | 15.8-16.3 ms; 7.89 MB | Bundled Large Office; four parses/indexes versus one shared snapshot |
| 2026-10-07: full analysis, warm geometry | 1.013 s; 1.011 GB | 0.844 s; 797 MB | 80 closed-shell Zones; median of three runs |
| 2026-10-07: IDF serialization | 7.94 ms; 91,051 allocations | 0.546 ms; one allocation | Same 80-Zone input; formatting regressions retained |
| 2026-10-07: quality across 13 periods | 32.4-33.6 ms; 14.13 MB | 2.57-2.60 ms; 1.096 MB | 1,001 sources; one prepared run-level context |
| 2026-10-07: capacity graph construction | 2.572 s; 2.524 GB | 2.098 s; 2.257 GB | 1,000 surfaces, 20 Zones, 12 months; time measures graph construction, allocations include parser/build/JSON; same 15,483,712-byte result size |
| 2026-09: combined-purpose saved bundle | 168.525 s | 60.826 s | Large Office, 6,277,230 SQL rows; compact reading plus fixed alias indexes |
| 2026-09: complete initial SQL parse | 30.023 s | 12.146 s | Same SQL, within the unchanged 20-second normal deadline; exact complete-result bytes |
| 2026-09-14: large HVAC result receiving | 1,733,651,105-byte JSON | 83,008,393-byte negotiated response | Eight loops, 33,315,600 bound observations; exact bits/order/metadata verified |
| 2026-09-14: saved Energy drivers stage | 117.448 s | 54.776 s | Large Office, 4,293 dictionaries; same 71,033,416-byte v1 result size |

The capacity result's equal size and numerical/provenance checks do not imply a
whole-file byte comparison. For the combined-purpose bundle, exact numeric
tokens and metadata matched while only preexisting nondeterministic source-ID
list ordering was normalized, retaining duplicate counts. Driver comparisons
also normalize existing reconciliation-record ordering. Neither comparison
introduces a numeric tolerance or drops missing/null/zero distinctions.

Initial SQL parser/source-selection replays use the complete original SQL result,
not the prior CSV/ESO fallback. Its retained 1,797,121-byte baseline has SHA-256
`314815dc401a0bff35bf2033e71cf7c8d9225a9e0885fcb49d5017cf8042d8bb`.
The combined-purpose 131,820,236-byte baseline has SHA-256
`72eddcab5afaf716d11497347ffb0ca1fde7b9f0a62f4059c01c215b8df83ccc`.
They establish parser/pipeline equivalence, independently of numerical oracle
approval described in [Energy Path](energy-path.md#native-fixtures).

These measurements were recorded before this documentation consolidation; they
have not been rerun as part of the documentation change. Reproduce against the
same fixture and implementation revision before comparing timings or hashes.

Native-clock browser playback of the compact HVAC response took 1.232 seconds
locally through HTTP receipt, JSON parsing, lazy binding and first display. It
materialized 127 of 717 point sets initially. This excludes engine/SQL work and
does not measure the native Wails WebView bridge.

## Reproduce focused measurements

From the repository root in PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
. .\scripts\toolchain.ps1
$paths = Use-RepoToolchain -RequireGo
& $paths.GoExe test ./cmd/semantic-idf -run '^$' -bench BenchmarkAnalysisInputPreparation -benchmem
& $paths.GoExe test ./cmd/semantic-idf/internal/idf -run '^$' -bench 'BenchmarkAnalyzeWarm|BenchmarkDocumentString|BenchmarkThermalTopology|BenchmarkAnalyzeHVACFromIndex' -benchmem
& $paths.GoExe test ./cmd/semantic-idf/internal/simulation -run '^$' -bench BenchmarkEnergyPathQualityPeriods -benchmem
& $paths.GoExe test ./cmd/semantic-idf/internal/simulation -run '^TestEPATH210ThousandSurfaceMonthlyCapacity$' -count=1 -v
```

Record fixture, engine/version, purposes, requested output frequencies, scope,
worker/GOMAXPROCS settings and background load. Report time and allocations by
stage. Retained heap after GC is a different measurement from peak RAM/RSS.

## Saved-run diagnostics

These existing tests read optional local captures and do not launch EnergyPlus.
Set the exact executed input and original run directory together. Write new
results/profiles outside the preserved capture; the SQL diagnosis requires its
output beneath repository `.runtime/`. Use `-count=1` when measuring.

| Test | Required environment | Optional diagnostic artifacts |
| --- | --- | --- |
| [TestPurposeSavedBundleReplay](../cmd/semantic-idf/internal/simulation/purpose_saved_bundle_replay_test.go) | `SEMANTIC_IDF_BUNDLE_REPLAY_DIR`, `_INPUT` | Same prefix plus `_PROFILE`, `_OUTPUT`, `_COMPARE` |
| [TestSQLSavedParseReplay](../cmd/semantic-idf/internal/simulation/sql_saved_parse_replay_test.go) | `SEMANTIC_IDF_SQL_REPLAY_DIR`, `_OUTPUT` | `_COMPARE` complete-result baseline |
| [TestSQLSavedOutputSourcesReplay](../cmd/semantic-idf/internal/simulation/sql_saved_output_sources_replay_test.go) | `SEMANTIC_IDF_SQL_REPLAY_DIR`, `_COMPARE` | Uses the baseline capture sidecar |
| [TestEnergyDriversSavedReplay](../cmd/semantic-idf/internal/simulation/energy_drivers_saved_replay_test.go) | `ENERGY_DRIVERS_REPLAY_DIR`, `_INPUT` | Same prefix plus `_PROFILE`, `_OUTPUT`, `_COMPARE` |
| [TestEnergyDriversSavedComparison](../cmd/semantic-idf/internal/simulation/energy_drivers_saved_replay_test.go) | `ENERGY_DRIVERS_REPLAY_COMPARE`, `_OUTPUT` | Compares existing artifacts without another SQL scan |
| [TestSimulationResultTransportSavedReplay](../cmd/semantic-idf/internal/simulation/result_transport_replay_test.go) | `SIMULATION_TRANSPORT_REPLAY_DIR`, `_INPUT` | Same prefix plus `_OUTPUT`; `_LEGACY=1` measures prior callback serialization |
| [TestSimulationActualLargePayloadResponseAndViewsBrowser](../cmd/semantic-idf/internal/frontendchecks/simulation_progress_actual_payload_browser_test.go) | `SIMULATION_UI_BUNDLE_PATH`, `_INPUT` | Captured canonical v2 bundle with Zone Heat Flow; native-clock display timing through the real frontend |

For example, after configuring the matching environment variables:

```powershell
& $paths.GoExe test ./cmd/semantic-idf/internal/simulation -run '^TestPurposeSavedBundleReplay$' -count=1 -v
& $paths.GoExe test ./cmd/semantic-idf/internal/frontendchecks -run '^TestSimulationActualLargePayloadResponseAndViewsBrowser$' -count=1 -v
```

Inspect the named test for its comparison format and output restrictions. Capture
snapshots verify hashes, sizes, timestamps and directory membership. A passing
performance comparison does not approve physical design or regenerate expected
native quantities. Ordinary tests retain their existing opt-in gates.

The actual-payload browser test accepts a canonical v2 purpose bundle, not a
compact transfer envelope or the historical v1 comparison baseline. The regular
[TestSimulationHTTPResultTransportBrowser](../cmd/semantic-idf/internal/frontendchecks/simulation_http_result_browser_test.go) covers production compact encoding,
lazy decoding and negotiated HTTP behavior with a bounded fixture.

Original captures and reusable SQL/bundle baselines survive cleanup. Local replay
JSON, CPU profiles, search dumps and disposable browser screenshots are not
guaranteed to exist in a clone. Generate fresh diagnostics when needed rather
than relying on a past session's temporary filenames. Detailed superseded
investigation logs remain in Git history.
