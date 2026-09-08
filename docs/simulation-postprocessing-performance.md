# Saved-run post-processing stall investigation (2026-09-08)

The reported screen remained at `Building purpose result bundle` / 89% for a
Large Office run with Basic Energy and Surface Zone Heat Flow, all zones and
the full year. The user had already closed the desktop application. There was
no live process to inspect. The original run directory was preserved; the
engine was **not** rerun and no simulation inputs or result files were edited.

## Reproduction and boundary

The saved run is `20260908-085859-sim-1788825537636-RefBldgLargeOfficeNew2004_Chicago`
under the user's configured SemanticIDF simulations directory. Its executed
IDF, flat purpose run plan and SQL were used together, not a Basic-Energy-only
projection. EnergyPlus completed in 46.24 seconds with zero Severe/Fatal errors.
The SQL has 142,663,680 bytes and 6,277,230 ReportData rows. The completed
application manifest was absent.

89% was the rounded stage ratio 8/9, emitted before the entire purpose bundle
was built. It was not a work estimate or evidence of a running engine. The
first unmodified replay completed the bundle in 156.663 seconds; a second,
stage-instrumented baseline took 168.525 seconds. Thus this capture reproduced
an extended silent processing interval, not an infinite loop.

## First bounded correction

- Cache dictionary and Time metadata once, then stream the three ReportData
  fields in the original time/dictionary order for Energy Path and surface
  topology. Do not create indexes in a user's SQL file.
- Distinct observed dictionary IDs are selected before joining dictionary
  text. Observed NULL/zero sources still exist; unobserved identities do not.
- Skip unused interval parsing for reported energy; rate integration is
  unchanged.
- Preserve unusual-schema JOIN behavior. TEXT/noninteger metadata IDs and
  one-to-many joins use the original walker. BLOB time IDs cannot invent a
  matching integer timestamp. The public walker is unchanged.
- Report real geometry, energy totals, drivers, service-path, Energy Path and
  surface heat-flow stages. Display local elapsed time, explicitly not a
  remaining-time estimate. Progress updates preserve existing result DOM and
  focus. Receiving results is separate from completed display; a display error
  does not relabel a successful engine result as a simulation failure.

The final replay of this correction took 118.878 seconds to build the same
bundle, approximately 29% below the stage-instrumented baseline. This is a
local measurement, not a performance SLA. Reading the initial outputs took
43.481 seconds; serializing the bundle took 1.222 seconds. The diagnostic
including integrity checks and comparison passed in 170.09 seconds.

The complete 131,820,236-byte bundles were compared without application
migration, rounding or numeric tolerance. All numeric tokens, metadata and
provenance membership match. Raw byte hashes differ because 856 existing
`sourceIds` unions have nondeterministic list order. The comparison sorts only
these string lists, retaining duplicate counts, and preserves every other
array order and missing/null/zero distinction. A separate regression rejects
tiny numeric changes, removed source IDs, reordered values and absent/null
reported zeros. Every original capture file's hash, size, modification time
and directory membership remained unchanged.

Ignored local evidence:

| Artifact | SHA-256 / meaning |
| --- | --- |
| `.runtime/bundle-replay-baseline-01.json` | `72eddcab5afaf716d11497347ffb0ca1fde7b9f0a62f4059c01c215b8df83ccc` |
| `.runtime/bundle-replay-after-02.json` | `ae873cd358014857bdce27ab5b78de775977615c006ea5998b9969387ee78a20` |
| `.runtime/bundle-replay-before-01.pprof` | First 45 seconds of baseline bundle processing |
| `.runtime/bundle-replay-after-02-full.pprof` | Full corrected pipeline CPU profile |

## Display verification and regression tests

The real baseline JSON was served unchanged to an isolated headless browser
using the actual frontend and geometry from the exact executed IDF. It took
276.2 ms to receive text, 249.1 ms to decode, and 250.2 ms to display/layout
Energy (22 nodes); selecting Heat Flow took 79.9 ms (19 zones, 686 interactive
frames). There was one mocked response and no engine execution. These are
browser measurements, not a claim about native Wails bridge latency.

Focused tests cover compact/public SQL row equality, metadata filters,
NULL/zero/infinity, duplicate and orphan rows, callbacks, schema variants,
read-only access, combined/single-purpose result equality, four surface
periods and traces, stage sequence, unchanged result DOM/focus, pending
responses, elapsed-clock cleanup and stale/error events. The actual replay
and large-payload browser checks are explicit opt-in tests; ordinary tests
never depend on this user's capture or launch EnergyPlus.

Full `scripts/verify.ps1` passed: main 23.605 s, CLI 7.964 s, frontend checks
165.662 s, simulation 106.764 s, and the Wails production build 10.498 s.
This investigation does not approve checklist section 22's numeric expected
manifests or mark the overall Energy Path checklist complete. The full profile
also identifies repeated availability/quality name normalization as a further
in-scope performance opportunity; it is being examined separately from this
verified SQL/feedback correction.

## Follow-up: fixed alias lookup indexes

The full profile attributed 45.19 CPU seconds to completeness construction,
mostly repeated heat-alias catalog scans, and 22.54 seconds to Energy Path
quality calculations using the same name lookups. The follow-up replaces only
the four fixed meter/variable/load/heat alias lookups with bounded indexes.
Catalog order, first matching definition, name normalization and dynamic meter
fallback remain unchanged. Returned definitions still own independent slices,
including the distinction between nil and empty; arbitrary model names and
computed quality/scope values are not cached.

Every catalog alias and unknown/whitespace/case variant is compared with the
original linear algorithm. Mutation, collision precedence and concurrent-read
regressions pass. An independent review found no changed matching semantics.
Focused regression tests passed in 7.788 seconds. Lookup microbenchmarks show
roughly 109 microseconds to 0.83 microseconds for a late heat-alias match; this
microbenchmark does not substitute for a complete real-run measurement.

The same combined-purpose capture was replayed again, retaining all original
observations and the unchanged 131,820,236-byte bundle size. Bundle construction
took **60.826 seconds**, approximately 64% below the 168.525-second baseline.
All exact numeric tokens, metadata and source-ID membership again matched the
original result, and the entire capture remained unchanged. The diagnostic
passed in 127.86 seconds, including initial output reading, profiling,
serialization and the full comparison. Other local verification processes
were active, so these timings should be treated as local measurements.

The follow-up artifacts are `.runtime/bundle-replay-after-03.json`
(SHA-256 `856ece37a371b560f9399a20117584344537176bf6f09bdb3c83700b1c3f3f35`)
and `.runtime/bundle-replay-after-03-full.pprof`.

A pre-commit repeat exposed a flaky elapsed-clock browser assertion using
Chrome virtual time and a fixed 1,100 ms sleep. The test now observes an actual
native 1,000 ms interval callback and the corresponding elapsed DOM update
within a bounded five-second deadline, retaining every previous assertion.
It passed ten consecutive native-clock runs (25.667 seconds). Product timer
code was unchanged. The initially planned two checkpoints are combined for
fresh full verification after this test correction; the failed hook was not
bypassed and did not create a commit.

Fresh full `scripts/verify.ps1` passed after the native-clock correction and
alias indexing: main 21.648 s, CLI 5.900 s, frontend 156.716 s, simulation
84.031 s, and Wails production build 7.917 s. This is the final combined
performance/feedback checkpoint, not completion of the overall checklist.

## Follow-up: initial SQL result reading

The same capture also exposed an earlier, separate source-selection problem.
An unrestricted replay of the original SQL parser completed in 30.023 seconds;
the normal parser returned `context deadline exceeded` after 29.016 seconds.
Its 20-second limit is checked between parser stages, not inside each query.
In that run, 256 Series and the complete SQL Heat Flow (19 zones, 680 displayed
frames from 8,822 original frames) had already been read correctly. The caller
discarded the partial result on error and fell back to CSV/ESO. Other repeats
reached the deadline after Series, demonstrating timing-dependent selection.

The complete original SQL result is preserved as
`.runtime/sql-parse-unlimited-baseline-01.json` (1,797,121 bytes; SHA-256
`314815dc401a0bff35bf2033e71cf7c8d9225a9e0885fcb49d5017cf8042d8bb`).
Its sidecar binds the exact capture and records the separate deadline failure.
This is a parser equivalence baseline, not an independent numerical oracle.

The bounded correction extends the already verified compact walker to the
initial Series, Energy and Heat Flow readers, and selects distinct observed
dictionary IDs before joining metadata in the Series/Energy catalogs. It does
not change the deadline, error handling, source priority, filters, 256-column
Series cap, sampling or original public SQL query interface.

Restoring SQL-first selection intentionally differs from the previous fallback
result: the previous Series came from CSV and Heat Flow came from ESO (686
frames). Equivalence must therefore be checked against the original *complete
SQL parse*, not by accepting these differences in the old whole-bundle
comparison. `TestSQLSavedParseReplay` checks the complete SQL result and
`TestSQLSavedOutputSourcesReplay` checks exact Series/Heat Flow JSON through the
real output-reading and source-selection path. Both preserve the original
capture; the latter also verifies the baseline hash and capture sidecar.

The isolated actual replay passed: unrestricted parsing took 12.101 seconds,
and the normal 20-second path completed successfully in 12.146 seconds. Both
entire SQL results match the original 1,797,121 bytes and SHA-256 exactly; no
array sorting, numeric tolerance or compatibility repair is involved. The
separate real output-reading replay took 11.818 seconds and selected
`[sql, csv]`, with SQL Series/Heat Flow matching the baseline byte-for-byte and
no ESO fallback. CSV summaries remain intentionally available. All original
capture file hashes, sizes and modification times remained unchanged.

The retained result and sidecar are
`.runtime/sql-parse-compact-after-01.json` and its `.meta.json`. Focused tests
passed in 5.945 seconds, including a 1,503-frame fixture comparing all three
parsers against the preserved original walker. The fixture also explicitly
checks mixed reporting frequencies, the 256-Series cap, 1,200-point Series
sampling, Heat Flow stride/last-frame handling, duplicate observations,
NULL versus known zero, signed values and unknown optional metadata.

General timeout/error reporting remains a separate boundary: this optimization
does not make the limit a hard query deadline or retain completed sections after
a later parse error. No error is converted to success by this change.
