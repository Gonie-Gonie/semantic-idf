# Documentation

Start with the [project README](../README.md) for setup and commands. The bundled
Guide contains the detailed [technical reference manual](../cmd/semantic-idf/frontend/src/manual/README.md), including workflows and calculation/interpretation rules. These developer references describe current behavior,
implementation boundaries and verification needed for further development.

## Development

| Reference | Use it for |
| --- | --- |
| [Architecture](architecture.md) | Finding code ownership and shared input, analysis, result and export invariants |
| [Working conventions](agent.md) | Repository workflow, fixture preservation and documentation maintenance |
| [Testing](testing.md) | Choosing feature/layer checks, commit verification and the full release suite |
| [Performance](performance.md) | Current optimizations, measured limits, benchmarks and optional saved-run replay |

## Feature contracts

| Reference | Use it for |
| --- | --- |
| [Energy Path](energy-path.md) | [Wire data](energy-path.md#wire-contract), [CLI/Python](energy-path.md#cli-and-python), [regressions](energy-path.md#regression-map), [native fixtures](energy-path.md#native-fixtures) and [equipment boundaries](energy-path.md#equipment-boundaries) |
| [Topology](topology.md) | [Views](topology.md#views-and-interaction), [data contracts](topology.md#data-contracts) and [regressions](topology.md#regression-coverage) |
| [Simulation runner](simulation-runner.md) | Run plans, output requests, result reading, purpose-specific views and batch execution |
| [Semantic navigation](semantic-navigation.md) | Source identity, selection and navigation between participating views |

Keep contracts and their regression references together. Historical implementation
checkpoints are available in Git history; dated measurements and native acceptance
baselines in current references describe the evidence actually retained.

## Releases and local artifacts

[Unreleased notes](release-notes/unreleased.md) describe pending changes.
[CHANGELOG](../CHANGELOG.md) and versioned files in `release-notes/` retain
published release history.

Tracked fixtures and expected manifests stay beside their tests. `.runtime/`
holds the repo-local toolchain, dependency/build caches and optional local captures.
Preserve original simulation inputs/results and reusable comparison baselines.
Completed search dumps, replay outputs, profiles and screenshot review artifacts
may be removed after their useful findings are recorded.

Preserve `build/`, frontend `dist/`, generated binaries and the Go/Wails runtime
when cleaning investigation files. An ignored path alone does not establish that
its contents are disposable.
