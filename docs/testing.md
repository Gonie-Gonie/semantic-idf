# Testing workflow

Use one test runner to choose the required checks by development scope. Existing
Go tests and fixtures remain in their current directories. The catalogs under
`scripts/test-groups-*.json` organize execution by feature, frontend/backend layer
and tier; they do not change the test implementations or retire older regressions.

## Choosing a command

Run these commands from the repository root after `dev setup`:

| Command | Scope |
| --- | --- |
| `dev test -Plan` | Show package test counts and changed-file impact reasons without executing tests |
| `dev test` | All fast tests, plus browser and regression tests for changed features |
| `dev test -Area profile` | All Profile tests across every tier |
| `dev test -Quick` | The fast baseline across all features |
| `dev test -Full` | All Go tests, including browser and historical regressions |
| `dev test -List` | Current test counts by feature and tier |
| `dev verify` | Selected tests, whitespace checks, frontend validation and Wails build |
| `dev verify -Full` | All Go tests, whitespace/frontend checks and Wails build |

In PowerShell, use `.\dev.bat` in place of `dev`. Direct invocation is also
supported, for example:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test.ps1 -Area profile
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1 -Full
```

`-Quick` is useful for immediate feedback while editing. It is a baseline check;
the default changed-file selection or a complete `-Area` check adds the affected
feature's browser and regression coverage. Use `-Full` for broad changes or when
you explicitly need the complete regression suite.

## Features, layers and tiers

Features describe product behavior rather than historical implementation phases.
Common areas include `input`, `analysis`, `topology`, `profile`, `hvac`,
`simulation`, `energy-path`, `batch`, `export`, `settings`, `storage`, `navigation` and
`tooling`, `guide` and `i18n`. The `i18n` area covers catalog completeness and
placeholders, dynamic language changes, settings synchronization and the
English/Korean manual. Shared locale sources select all frontend features and
desktop asset contracts. Energy Path and Simulation also have finer areas for specific backend
pipelines; use `dev test -List` to discover the current names and counts.

| Tier | Purpose |
| --- | --- |
| `fast` | Unit tests and static contracts used as the common development baseline |
| `browser` | Browser interactions, rendering behavior and frontend integration |
| `regression` | Saved results, actual-model matrices, large-model and acceptance checks |

A test can belong to multiple areas. For example, an HVAC inspection test can
cover both HVAC navigation and Simulation result presentation. The runner
selects each test once. Long selections use several package commands to stay
within the Windows command-line limit. Area names in old filenames
or acceptance identifiers are retained; the catalog defines their current
functional ownership.

`dev test -Area storage` focuses on generated-run ownership, protected inputs,
partial deletion, age filters, cross-process locking and Settings cleanup flows.
It uses isolated temporary files and browser API fixtures rather than real user
run directories. `-Area settings` adds preference persistence and presentation.

Use a layer when investigating one side of a feature:

```powershell
.\dev.bat test -Area profile -Layer frontend
.\dev.bat test -Area hvac -Layer backend
```

`-List` shows area and tier counts. `-Layer` filters execution to `frontend` or
`backend` and can be combined with `-Area` or `-Quick`; automatic changed-file
selection includes both dependent layers, and `-Full` always includes both.

Layer-specific runs help development feedback. Commit and release checks use
both layers so backend changes and their frontend contracts are checked together.

## Automatic selection

The default command reads changed paths, selects the fast baseline and adds all
tiers for the affected areas. This includes staged, unstaged and untracked work.
Use `-Staged` to select from staged paths, or `-BaseRef <reference>` when checking
changes relative to a Git reference. Combine these options with `-Plan` to see
the actual affected areas before running tests:

```powershell
.\dev.bat test -Staged -Plan
.\dev.bat test -BaseRef main -Plan
```

Source impact is conservative. Shared parser/model/index/schema code, helpers
used across features, dependency or toolchain changes and unclassified source
paths select the full suite. A feature module selects its own checks and the
affected downstream areas. Changes
to a `_test.go` file select its entire package because fixtures and test helpers
can be shared across that package. Unclassified fixture paths select the full
suite. Adding a source file does not silently bypass tests: until its impact is
classified, full selection is the safe fallback.

New tests are discovered from Go test files. A test without a catalog rule is
included automatically and reported for classification, so the catalog is not
an allowlist that can accidentally omit new coverage. Full verification calls
the complete Go suite independently of scoped selection.

The existing browser and SQLite-heavy packages continue to run with `-p=1` to
avoid competing browser/SQL workloads on Windows. Tests retain finite timeouts,
and the standard Go test cache remains enabled. Benchmarks are separate, opt-in
measurements and do not run as ordinary tests.

Selection changes test execution, while Go still compiles each complete package
and its shared test helpers. The Go build/test cache handles repeated compilation.

## Commit and release checks

`dev setup` installs a pre-commit hook; run `dev hook` to refresh an existing
clone's hook after changing the workflow. The hook calls `verify -Staged`, which
checks the selected tests and builds the Wails executable through the repo-local
runtime. Its successful result is the commit verification; an additional full
run for every implementation pass is not required.

Selection uses staged paths, while Go and Wails execute against the working tree.
Stage the complete related change before relying on the commit result. Review
`test -Staged -Plan` when splitting a change across several commits.

Release packaging always calls `verify -Full`. The GitHub Actions tag and manual
release routes both use that packaging path, preserving the full regression
check before a release executable is published. Feature/layer development
filters do not weaken the release gate.

Some tests have existing opt-in environment variables or require local captures
and an EnergyPlus installation. Full selection preserves those test-level gates;
it does not manufacture missing external data or turn a skipped test into an
executed one. Their fixture and replay instructions remain in the acceptance
and performance documents linked from the [documentation index](README.md).

## Maintaining the catalog

1. Keep tests and their shared helpers in the package they exercise. White-box
   tests can access package internals without exporting implementation details.
2. Add or update a package-relative filename rule in the appropriate
   `scripts/test-groups-*.json` file. Use a test-name glob when one file covers
   several features or contains one expensive matrix among fast checks.
3. Assign functional areas and a tier. Rules can overlap; matching areas are
   combined and the more demanding matching tier takes precedence.
4. Update the changed-source impact mapping when a new source module or shared
   dependency affects another feature.
5. Inspect `test -List` and the relevant `test -Plan`, then run the affected area.
   Use full verification when changing the runner, catalogs or shared selection
   rules themselves.

Do not delete a regression merely because its development phase is old. A test
can be consolidated when its behavior is already covered by an equivalent
assertion and fixture, with that equivalence reviewed explicitly. The current
workflow reduces repeated execution through classification.

## Local validation

On the Windows development workspace, the catalog and Go's own test discovery
both found 2,146 tests: 1,252 fast, 91 browser and 803 regression. No existing
test or assertion was removed. The scoped commands passed as follows:

| Command | Selected tests | Wall time |
| --- | ---: | ---: |
| `dev test -Quick` | 1,252 | 59.0 seconds |
| `dev test -Area profile -Layer frontend` | 26 | 18.0 seconds |

These are local feedback measurements with a warm toolchain and some cached
package results, rather than a forced uncached benchmark. The simulation fast
subset itself ran in 10.8 seconds across four batches after expensive fixture
tests were classified as regression. Full verification remains a separate gate.
