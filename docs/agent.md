# Agent working notes

Start with the [documentation index](README.md) and [architecture map](architecture.md).
Feature contracts, fixture evidence and performance baselines have separate owners;
read the relevant contract before changing a shared model or consumer.

## Development workflow

- Use the repo-local Go/Wails runtime installed by `dev setup` under ignored
  `.runtime/`. Prefer `dev.bat` on Windows for the PowerShell bypass flags.
- Use `dev test -Plan` to inspect impact, `-Area <feature>` for all tiers of a
  feature, and `-Quick` for the common baseline. See [testing.md](testing.md).
- Keep Go tests and shared fixtures in their existing packages. Register new
  tests in `scripts/test-groups-*.json` and maintain changed-source impact rules.
- The installed pre-commit hook runs `verify -Staged`: selected tests, frontend
  validation and a Wails build. Its successful result is the commit check; do
  not repeat an unchanged full suite just for reporting.
- Release packaging uses `verify -Full`. Shared core, toolchain and unclassified
  changes also select the full suite. Stage the complete related change because
  execution uses the working tree. Commit and push completed work.
- Preserve unrelated user changes. Do not weaken fixture expectations, source
  identity, precision or tolerances to make a regression pass.

## Implementation boundaries

- Keep lifecycle and file operations in `app.go`; group Wails feature methods in
  analysis, batch, settings, simulation and Energy Path app modules.
- Format/version/conversion belongs in `internal/epinput`; low-level IDF analysis
  belongs in `internal/idf`. Prefer domain functions that do not launch Wails.
- Shared parsed inputs and indexes are read-only. Keep editing and derived
  mutable state owned; bound caches and worker pools. Follow [architecture.md](architecture.md).
- Use the existing result builder for App, HTTP, CLI and Python. Preserve wire
  compatibility and missing/null/known-zero distinctions across every consumer.
- Keep frontend entrypoints small and feature logic in the existing JS modules.
  End-user help belongs in the bundled Guide; developer instructions belong here
  and in the project README.
- Keep app translations in `frontend/src/js/locales/`, with English authority and
  per-key fallback for incomplete locales. Preserve all six app languages and
  familiar industry terms. Use locale descriptors for dynamic status text; never
  localize source IDs, units or backend evidence. Guide follows the app language
  and supports Korean/English only. See the frontend README for owning modules.

## Documentation and artifacts

- Maintain current behavior, invariants, source ownership, reproducible checks
  and known limits. Update the owning contract when implementation changes.
- Update user-facing workflows, algorithms and quantity meanings in the single
  `frontend/src/manual/` source directory. Keep English/Korean section IDs aligned;
  regenerate the offline metric catalog from its Go registry after metadata edits.
- Keep release notes as published history. Avoid new phase-by-phase progress
  ledgers, repeated test timings, session transcripts or stale pending counts.
- Preserve original simulation inputs/results, approved expected manifests,
  reusable comparison baselines, build outputs and toolchain caches. An ignored
  path alone is not proof that its contents are disposable.
- Remove identified temporary replay/profile/search/screenshot artifacts after
  recording useful findings. Reproduction commands must distinguish local
  optional captures from files guaranteed to exist in a fresh clone.
