# Documentation

Start with the [project README](../README.md) for setup, developer commands and
CLI examples. The application's bundled Guide covers end-user workflows.

## Architecture and contracts

| Document | Purpose |
| --- | --- |
| [Agent working notes](agent.md) | Repository conventions and implementation boundaries |
| [Refactoring and performance](refactoring-performance.md) | Current cleanup, code structure and performance measurements |
| [Semantic navigation](semantic-navigation.md) | Shared identity, selection and cross-view navigation |
| [Topology view](topology-view.md) | Geometry views, interaction and exports |
| [Thermal topology schema](thermal-topology-schema.md) | Surface heat-flow result and presentation contracts |
| [Simulation runner](simulation-runner.md) | Run plans, output requests, result reading and batch execution |
| [Energy Path schema](energy-path-schema.md) | Graph domains, allocations, provenance and serialization |
| [Energy Path CLI and Python](energy-path-cli.md) | Read existing results through CLI and the local API |

## Acceptance and regression evidence

- [Thermal topology acceptance](thermal-topology-acceptance.md)
- [Energy Path acceptance](energy-path-acceptance.md)
- [Actual-model acceptance and capture provenance](energy-path-real-models.md)
- Equipment and model families:
  [district](energy-path-district-acceptance.md),
  [fan coil](energy-path-fan-coil-acceptance.md),
  [mixed heating](energy-path-mixed-heating-acceptance.md),
  [pool](energy-path-pool-acceptance.md),
  [PTHP](energy-path-pthp-acceptance.md),
  [PV/storage](energy-path-pv-acceptance.md),
  [radiant](energy-path-radiant-acceptance.md),
  [VRF](energy-path-vrf-acceptance.md), and
  [zone groups](energy-path-zone-group-acceptance.md).

The [Energy Path progress ledger](energy-path-progress.md) records historical
implementation checkpoints. Read its latest completion entries and the current
schema before using earlier checkpoint descriptions as current requirements.
The [saved-run performance investigation](simulation-postprocessing-performance.md)
retains historical measurements, equivalence checks and opt-in replay commands.

## Releases

[Unreleased notes](release-notes/unreleased.md) describe pending changes.
[CHANGELOG](../CHANGELOG.md) and the versioned files in `release-notes/` retain
published release history.

## Local artifacts

Tracked fixtures and their expected manifests remain beside their tests.
`.runtime/` holds the repo-local toolchain, dependency/build caches and optional
local captures. Keep original simulation inputs/results and reusable comparison
baselines. Ad hoc search dumps, completed replay outputs, CPU profiles, screenshot
reviews and their disposable browser profiles can be removed after recording
their useful findings. They are not required by the ordinary test suite.

Preserve `build/`, frontend `dist/`, generated binaries and the Go/Wails runtime
when cleaning temporary investigation files. Never treat an ignored path alone
as proof that its contents are disposable.
