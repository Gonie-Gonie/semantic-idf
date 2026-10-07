# Actual Energy Path model fixtures

This directory preserves official native inputs and independent regression
artifacts. Development contracts and replay instructions are maintained in
[Energy Path: native fixtures](../../../../../../docs/energy-path.md#native-fixtures)
and [equipment boundaries](../../../../../../docs/energy-path.md#equipment-boundaries).

## Catalog and artifact contract

`catalog.json` uses
`semantic-idf.energy-path-real-model-catalog/v1` with a `fixtures` array.
There are 20 original entries: 19 have separately reviewed expected headers and
lossless companions. `no-heating-25-1` (DataCenter) remains rejected for native
Severe errors. The approved `no-heating-ventilation-25-1` (SimpleVentilation)
is a distinct fixture, not DataCenter approval.

Each row declares:

- `id`, `version`: stable filter identity and required EnergyPlus version.
- `modelPath`, `modelSHA256`: unchanged original model and exact-byte digest.
- `engineSourceFilename`, `sourceURL`, `licensePath`: official distribution
  origin and license, not a claim that LF-normalized GitHub files share its bytes.
- `weather.filename`, `weather.sha256`: explicitly controlled weather.
- `types`: required coverage obligations, not evidence of passed acceptance.
- `oraclePath`, `expectedPath`: independent source recipe and approval header.

Paths are relative to this directory except the installation-relative
`engineSourceFilename` and weather basename. Exact models, versions and approved
metric counts are in the catalog/headers; do not derive status from filenames
or coverage tags. The last recorded complete native replay is the dated
2026-09-25 baseline, not automatic verification of later source changes.

`oracles/*.json` declares original ownership, exact SQL identities and independent
comparison rules. It cannot use production allocation/classification helpers or
candidate scalars as arithmetic authority. `expected/*.json` is an explicitly
authored review/provenance header; `metricPayload` fixes the companion path,
compressed/uncompressed SHA-256, complete count and required-key digest.

`expected/*.metrics.json.gz` retains every metric, known zero, typed unknown,
count and status. The loader rejects conflicting inline/compressed payloads,
missing explicit values, duplicate members, checksum/count/key changes,
physical path escape, oversized/truncated/extra-member gzip and trailing data.
Plain inline-v1 compatibility remains.

## Original bytes and licenses

Models retain exact original bytes from official EnergyPlus 23.2/24.2/25.1
installations and the verified official 22.1 Windows portable distribution.

| Version | Original license SHA-256 |
| --- | --- |
| 22.1 | `b34ed244dc8c0693f07524337091285566d1b0ab9299395b50a96522af301757` |
| 23.2 | `081b43b6cc5066f56442164cc6d74bd98ed2a8d44b03a5a87e061c48c3adfd91` |
| 24.2 | `b43f1553459a4bcc49d180b42123a64a54fcbb6213cd99ac6ac6aa32cb1c1a05` |
| 25.1 | `ce67ca926130d81acd0f5158d4efd61aeab3ded3a8826024b050abcf85278f97` |

Fixture-local `.gitattributes` marks `models/**` and `licenses/**`
`-text -whitespace` so Git cannot normalize upstream bytes. Generated companions
are binary. Authored JSON, tests and documentation follow normal text rules.
The original Large Office 24.2 fixture is distinct from the frontend startup
sample, which enables annual weather simulation.

## Runtime isolation and evidence

Every fixture uses controlled Chicago TMY3:
`USA_IL_Chicago-OHare.Intl.AP.725300_TMY3.epw`, SHA-256
`c7d4efcf93ba316a1d874352e743df5cf137ba5c0e3459eb2dc4b5442d5b7f5c`.
The selected installation supplies the verified EPW; it is not duplicated here.
This climate does not certify the native Miami DOAS or Oklahoma shop examples.

Annual SimulationControl/RunPeriod and additive purpose outputs affect separate
run copies only. Original/annualized/executed inputs retain distinct hashes and
change records. Geometry, equipment, schedules and tolerances are not silently
repaired. Run outputs belong under ignored `.runtime/energy-path-acceptance/`,
never this directory or installed `ExampleFiles/`.

Capture success is collection, not numerical approval. Independent native SQL,
complete calendar/ownership/knownness proof and every required field across all
eight groups must pass before separate review can authorize expectations.
Normal tests neither launch EnergyPlus nor create/update approvals.

`TestEnergyPathRealExpectedGeneratePayload` is opt-in lossless packaging:
prepare writes only a new `.runtime` artifact; install needs the matching
already-authored review header and writes only its new companion.
Existing artifacts are not overwritten.
`TestEnergyPathRealApprovedExpectedCatalog` checks all 19 approvals offline.
For deliberate engine runs and read-only/current-loader saved acceptance, use
the [maintained replay instructions](../../../../../../docs/energy-path.md#execution-modes).
