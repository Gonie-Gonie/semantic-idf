# Refactoring and performance (2026-10-07)

## Structure

The desktop entrypoint retains its existing Wails methods and JSON contracts.
`cmd/semantic-idf/app.go` now contains lifecycle, input editing and file operations
(695 lines instead of 1,838). Related methods and types live together:

- `analysis_app.go`: full/quick/stage analysis and cached-result assembly.
- `analysis_cache.go`: bounded result caching and in-flight deduplication.
- `analysis_input_cache.go`: bounded parsed snapshots and lazy document indexes.
- `batch_metrics_app.go`: metrics workers, progress and comparison types.
- `settings_app.go`: settings persistence, defaults, normalization and migrations.

The existing simulation and Energy Path modules remain the entrypoints for those
features. The frontend's analysis scheduling is isolated in
`frontend/src/js/analysis-stage-queue.js`; `actions.js` applies results to the
workspace. The queue preserves two concurrent workers, active-tab priority and
input-stage result order. It stops launching pending stages after an input
changes or a worker fails, and waits for already-running requests to settle.

## Analysis work sharing

Quick, Profile, HVAC and Geometry analysis share one parsed input and one lazy,
read-only `DocumentIndex` for the same normalized content hash. Concurrent misses
share an in-flight parse. The input cache retains at most three documents and
evicts the least recently used snapshot; failures are retried. Editing APIs use
fresh documents. EPJSON export no longer inserts generated `idf_order` metadata
into its source model, allowing export and serialization to read a shared model.

Within full analysis, Metrics readiness now uses the session's HVAC,
Diagnostics and Output reports instead of rebuilding them. `sync.Once` shares
these dependencies among the existing bounded analysis workers. HVAC uses the
index's read-only type/name maps and keeps derived branch/node state local.

Thermal topology uses source-ordered surface/space/boundary ownership indexes
for centroids, enclosure checks and zone signatures. Matching preserves
`strings.EqualFold` Unicode behavior and significant whitespace. Accumulation
and output order remain source ordered. IDF serialization writes directly to a
pre-sized builder, preserving formatting while avoiding per-field `fmt` calls.

## Simulation post-processing

Energy Path quality separates run-level requested-output availability and source
identity from period-specific closure, ratios and allocation. Each Building or
Zone scope prepares its run-level evidence once per refresh and reuses it across
annual/monthly results. A later refresh prepares new evidence; this is not a
global cache. Missing periods, unknown values, reported zeroes, source IDs and
scope-local completeness retain their existing meanings.

Existing SQL compact readers, aliases and result transport already avoid
repeated metadata joins and large JSON copies; see the
[saved-run investigation](simulation-postprocessing-performance.md). This pass
improves result construction after reading, without changing SQL queries or
modifying saved simulation files. Existing stage and batch worker limits remain
bounded. Zone result mutation is not parallelized because summaries may share
backing data and the measured benefit comes from eliminating repeated work.

## Local measurements

Measurements use the repo-local Go runtime on Windows with an AMD Ryzen 9 3900X.
They describe these fixtures and stages, not an end-to-end latency guarantee.
Allocation figures are cumulative benchmark allocations, not peak resident RAM.

| Work | Before | After | Scope |
| --- | --- | --- | --- |
| Four-stage input preparation | 63–68 ms; 31.54 MB; 486k allocations | 15.8–16.3 ms; 7.89 MB; 122k allocations | Bundled Large Office input; four fresh parses/indexes versus one shared snapshot |
| Full analysis with warm geometry | 1.013 s; 1.011 GB; 18.969M allocations | 0.844 s; 797 MB; 10.523M allocations | 80 independent closed-shell Zones; median of three runs |
| IDF serialization | 7.94 ms; 7.69 MB; 91,051 allocations | 0.546 ms; 1.016 MB; 1 allocation | Same 80-Zone document; median of three runs |
| Energy Path quality for 13 periods | 32.4–33.6 ms; 14.13 MB | 2.57–2.60 ms; 1.096 MB | Same 1,001 sources; repeated classification versus one prepared context |
| Energy Path capacity graph | 2.572 s; 2.524 GB | 2.098 s; 2.257 GB | Existing 1,000-surface, 20-Zone, 12-month capacity fixture |

The capacity fixture's existing numerical, provenance, known-zero and read-only
checks passed before and after. Its serialized size (15,483,712 bytes) and
source/period/Zone cardinalities match. This was not a byte-for-byte comparison
of whole baseline and updated JSON files.

Topology construction changed from 119.7 to 116.4 ms on the 80-Zone fixture;
its timing improvement is small. Standalone HVAC timings varied under concurrent
local load, so no speedup is claimed there; cumulative allocations fell from
6.695 to 6.213 MB. Profiling still identifies annual Profile series construction
as a substantial allocation cost. This pass preserves those series and their
rounding behavior rather than expanding the change without separate evidence.

Reproduce the isolated benchmarks after loading `scripts/toolchain.ps1` and
calling `Use-RepoToolchain -RequireGo`:

```powershell
& $paths.GoExe test ./cmd/semantic-idf -run '^$' -bench BenchmarkAnalysisInputPreparation -benchmem
& $paths.GoExe test ./cmd/semantic-idf/internal/idf -run '^$' -bench 'BenchmarkAnalyzeWarm|BenchmarkDocumentString|BenchmarkThermalTopology|BenchmarkAnalyzeHVACFromIndex' -benchmem
& $paths.GoExe test ./cmd/semantic-idf/internal/simulation -run '^$' -bench BenchmarkEnergyPathQualityPeriods -benchmem
```

## Cleanup and verification

Removed 1,871 identified temporary files totaling 622,901,453 bytes (594.05 MiB):
redundant replay outputs, completed CPU profiles/search dumps and disposable
screenshot/browser review directories. Build outputs, generated binaries,
toolchains, caches, original simulation captures and reusable comparison
baselines were preserved. No regression fixtures or tests were removed.

The documentation index distinguishes maintained contracts, acceptance evidence
and historical progress ledgers. `scripts/test.ps1` now runs from the repository
root, restores the caller's location and propagates a failing Go test exit.

Browser executable discovery is shared across the test suite. Repeating the
same PATH/PATHEXT search for each browser harness produced 264,686 test-log
entries (18.94 MB), including 252,777 `stat` entries on this Windows environment.
Go subsequently re-evaluated that log when saving its test cache, adding several
minutes after the browser process had exited. Discovering the executable once
removes those repeated probes while retaining all browser harnesses and checks.

Focused regressions cover concurrent parse/index reuse, eviction, edits and parse
failures; shared-model EPJSON export; analysis dependency sharing; serialization
formatting; thermal ownership; quality refreshes; and browser queue concurrency,
priority, ordering, stale inputs and worker failure. Windows has no available C
compiler for Go's race detector; concurrent regressions run normally, including
20 repeats of the input-cache tests.

Final `scripts/verify.ps1` passed with every Go package and the Wails production
build. The full browser suite took 165.132 seconds, the simulation suite
469.564 seconds, and the production build 29.58 seconds on this local run.
