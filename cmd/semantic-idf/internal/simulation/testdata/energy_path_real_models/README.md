# Actual Energy Path model fixtures

This directory contains original EnergyPlus example models for checklist section
22. The catalog is a test plan, **not evidence that acceptance has passed**.

## Current status

- All 19 original models are present: the full 16-model 25.1 fixture set and
  Large Office 22.1/23.2/24.2 compatibility fixtures. All four version-specific
  license files are included with verified original bytes.
- Large Office 25.1 has its first explicitly reviewed expected manifest and
  passes saved-original-wire acceptance for all 46,224 independent metrics and
  complete record coverage. Its 394,508-byte compressed companion retains every
  metric, explicit unknown, count and status; it is not a summary subset.
- Small Office 25.1 is also explicitly approved: all 17,675 independent metrics
  and full required-field coverage of 5,074 records pass saved-original-wire
  acceptance. Its 146,896-byte companion preserves all metrics, actual zeros,
  explicit unknowns and tiny positive unassigned pump energy. Independent review
  verifies the physical DX/gas paths, exact fan pools/direct-use owners and
  original SQL calculations. The Large Office approved regression still passes.
- Ideal Loads 25.1 is now explicitly approved with a 131,365-byte lossless metric
  companion. Its actual Annual Tabular district consumption is preserved separately
  from Monthly electricity observations. Independent annual-cell, frame, source,
  service, carrier, quality and required-field coverage integration is implemented,
  preserving strict Monthly absence and existing Monthly-to-Annual sums. Twenty
  exact original Monthly latent Energy/Rate observations remain non-additive
  inspector context, bound to their five actual equipment owners, not additional
  sensible load. Preserved diagnostics report 6,504 failures over 16,993 checks,
  then 1,246 over 17,073 (480 numeric/contract and 766 coverage). Twelve exact
  Hourly reconciliation aliases now have full-calendar and Monthly-equivalence
  proofs, without changing numeric authority. Diagnostic 03 passes all 17,121
  checks and required fields across 4,047 records in 15.252 seconds. Diagnostic 04
  repeats that pass before separate provenance/registry and direct original-SQL
  arithmetic reviews. Saved acceptance against the explicitly approved expected
  manifest passes all 17,121 metrics in 28.405 seconds. The offline integrity
  guard requires all three approved fixtures. The other 16 catalog entries,
  including the three older Large Office versions, remain unapproved; later
  checklist sections are not complete. Full repository verification and Wails
  build are required by the normal commit gate.
  The native Building meter subtotal double-counting defect found in this capture
  is fixed, with original observations retained as non-additive source context.
- All 19 engine executions have finished (18 normal results; the no-heating
  model rejected for Severe errors). Catalog paths where files are not yet
  present remain intended destinations, not completed acceptance claims.
- No generated SQL, fabricated expected totals or synthetic oracle recipes are
  supplied by this catalog.
- `output_alias_discovery` nominates Large Office 25.1 for a required check.
  It is satisfied only by a real non-exact RDD/MDD resolution whose resolved
  output is reported in the actual SQL result. A filename, simulated dictionary
  or missing output alone is not proof.
- Heating/cooling absence must be established from actual equipment and delivered
  load/consumption evidence, not by assuming predicted load is zero. Simultaneous
  heating/cooling must be observed in overlapping actual reporting intervals.

## Catalog contract

`catalog.json` has schema
`semantic-idf.energy-path-real-model-catalog/v1` and a `fixtures` array.
Every row identifies:

- `id`: stable fixture/filter identity.
- `version`: required EnergyPlus major/minor version.
- `modelPath`, `modelSHA256`: original checked-in model and exact-byte SHA-256.
- `engineSourceFilename`: original path relative to the official installation.
- `sourceURL`: the corresponding official release, not a claim that a GitHub
  LF-normalized source download has the same bytes as the installed file.
- `licensePath`: unchanged license from that version's installation.
- `weather.filename`, `weather.sha256`: required official weather file identity.
- `types`: intended coverage obligations; tags do not establish passed results.
- `oraclePath`: independently reviewed SQL-oracle recipe destination.
- `expectedPath`: reviewed numeric acceptance manifest destination.

All catalog paths except `engineSourceFilename` and the weather basename are
relative to this directory. IDs, paths and hashes must remain unambiguous.
Absent required engine/model/weather inputs must not silently pass an explicitly
requested actual-run acceptance check.

## Original-byte provenance and licenses

Models were copied without editing from `ExampleFiles` in the official locally
installed EnergyPlus version 23.2, 24.2 and 25.1 distributions, and the verified
official EnergyPlus 22.1.0 Windows portable distribution. Each copy was
SHA-256 checked against that installation's original. The catalog stores the
model hashes. Per-version license copies retain the original copyright notice,
redistribution terms and disclaimer:

| EnergyPlus version | Original license SHA-256 |
| --- | --- |
| 22.1 | `b34ed244dc8c0693f07524337091285566d1b0ab9299395b50a96522af301757` |
| 23.2 | `081b43b6cc5066f56442164cc6d74bd98ed2a8d44b03a5a87e061c48c3adfd91` |
| 24.2 | `b43f1553459a4bcc49d180b42123a64a54fcbb6213cd99ac6ac6aa32cb1c1a05` |
| 25.1 | `ce67ca926130d81acd0f5158d4efd61aeab3ded3a8826024b050abcf85278f97` |

The fixture-local `.gitattributes` marks `models/**` and `licenses/**` as
`-text -whitespace`. This prevents Git from changing original line endings during
add or checkout and invalidating these byte hashes, and excludes unchanged
upstream whitespace from authored-code linting. Catalog, documentation, tests
and authored oracle/expected JSON retain the repository's normal text rules.
Generated `expected/*.metrics.json.gz` companions are explicitly binary so Git
cannot change their checksum-bound bytes.

The copied official Large Office 24.2 file is deliberately **not** the separately
vendored frontend sample, which enables annual weather simulation. Its original
installation bytes are preserved here.

## Controlled weather and runtime adaptation

Every fixture uses the official
`USA_IL_Chicago-OHare.Intl.AP.725300_TMY3.epw` with SHA-256
`c7d4efcf93ba316a1d874352e743df5cf137ba5c0e3459eb2dc4b5442d5b7f5c`.
The verified 22.1, 23.2, 24.2 and 25.1 copies have identical weather bytes. The runner
resolves and verifies this file from the selected installation; the EPW is not
duplicated here.

Chicago is an explicitly controlled test climate, not a claim to match every
example's native location. In particular, the DOAS-to-PTAC/PTHP/VRF examples
identify Miami, and the PV/battery shop identifies Oklahoma City. Their model
objects remain unchanged in this directory.

Most original examples enable only design days or short weather periods.
Therefore:

1. The runner copies/parses the selected original into a separate run directory.
2. Annual SimulationControl/RunPeriod changes and the normal Basic Energy Path
   output plan are applied **only to that run copy**.
3. The runner records the original hash, prepared-input hash, actual engine and
   weather identity, exact runtime changes and output plan.
4. Numeric manifests must describe the actual reporting environment and period;
   a short run or sizing/design-day result must not be presented as a full year.
5. Geometry, equipment and schedules are not silently repaired or replaced.
   Any required model adaptation beyond run controls/output requests needs a
   separately reviewed and recorded decision.

Do not run or write outputs into `models/` or an EnergyPlus installation's
`ExampleFiles/` directory.

## Intended fixture coverage

| Fixture ID | Official example | Intended coverage |
| --- | --- | --- |
| `large-office-25-1` | `RefBldgLargeOfficeNew2004_Chicago.idf` | large_office, output_alias_discovery |
| `small-office-25-1` | `RefBldgSmallOfficeNew2004_Chicago.idf` | small_office |
| `ideal-loads-25-1` | `5Zone_IdealLoadsAirSystems_ReturnPlenum.idf` | ideal_loads |
| `ptac-25-1` | `DOAToPTAC.idf` | ptac_pthp, ptac |
| `pthp-25-1` | `DOAToPTHP.idf` | ptac_pthp, pthp |
| `fan-coil-25-1` | `FanCoilAutoSize.idf` | fan_coil |
| `vrf-25-1` | `DOAToVRF.idf` | vrf |
| `radiant-25-1` | `RadLoTempCFloHeatCool.idf` | radiant |
| `district-energy-25-1` | `5ZoneFanCoilDOAS_ERVOnAirLoopMainBranch.idf` | district_energy |
| `mixed-heating-fuels-25-1` | `5ZoneElectricBaseboard.idf` | mixed_heating_fuels |
| `zone-group-25-1` | `MultiStory.idf` | zone_multiplier_or_group, zone_group |
| `zone-multiplier-25-1` | `5ZoneSwimmingPoolZoneMultipliers.idf` | zone_multiplier_or_group, zone_multiplier |
| `pv-storage-25-1` | `ShopWithPVandBattery.idf` | pv_storage |
| `no-cooling-25-1` | `Furnace.idf` | no_cooling |
| `no-heating-25-1` | `2ZoneDataCenterHVAC_wEconomizer.idf` | no_heating |
| `simultaneous-25-1` | `CentralChillerHeaterSystem_Simultaneous_Cooling_Heating.idf` | simultaneous_heating_cooling |
| `large-office-23-2` | `RefBldgLargeOfficeNew2004_Chicago.idf` | large_office |
| `large-office-24-2` | `RefBldgLargeOfficeNew2004_Chicago.idf` | large_office |
| `large-office-22-1` | `RefBldgLargeOfficeNew2004_Chicago.idf` | large_office |

PTAC and PTHP are separate official models. `MultiStory.idf` exercises a
ZoneGroup multiplier (the middle group has multiplier 8);
`5ZoneSwimmingPoolZoneMultipliers.idf` exercises explicit Zone multipliers.
The Large Office cross-version rows are additional compatibility checks, not
substitutes for the complete 25.1 model-type set.

## Acceptance artifacts

After successful actual runs, independent raw-SQL checks must precede locking
the expected manifests. Each manifest covers driver-category totals,
cooling/heating loads, end-use totals, carrier totals, ratios, stage
completeness, residuals and Zone allocation coverage. Preserve missing versus
reported zero, source versus normalized units, and declared numeric tolerances.

Use the shared Purpose plan/application, simulation runner and canonical Energy
Path builder. Saved actual SQL and run provenance can subsequently be replayed
through `LoadEnergyPathProjection` without another simulation. A missing oracle
or expected file remains pending acceptance; generating a manifest directly from
the implementation under test is not an independent validation.

Large Office's expected JSON is an explicitly authored review/provenance header.
Its `metricPayload` descriptor names a same-directory gzip JSON array and fixes
compressed/uncompressed SHA-256, full metric count and exact required-key digest.
The loader restores all metrics and retains the original acceptance comparisons.
It rejects conflicting inline/compressed payloads, missing numeric presence,
duplicate members, checksum/count/key changes, escaping paths, oversized data,
truncation, extra gzip members and trailing data.

The opt-in `TestEnergyPathRealExpectedGeneratePayload` is a deterministic
packaging step, not approval: `prepare` writes only a new `.runtime` artifact;
`install` requires the already authored catalog review header to name the exact
reviewed pending SHA and payload descriptor. It writes only a new binary
companion. Ordinary tests, captures and passing diagnostics cannot generate or
overwrite approval headers. `TestEnergyPathRealApprovedExpectedCatalog` keeps
already approved artifacts required in ordinary offline integrity tests.
