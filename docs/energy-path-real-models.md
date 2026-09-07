# Energy Path actual-model acceptance

Checklist section 22 is in progress. The frontend/SQL unit fixtures documented
in `energy-path-acceptance.md` do not constitute EnergyPlus model-run acceptance.
Official model inputs, licenses and a versioned catalog live under
`cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models`.
Coverage tags in that catalog are obligations to verify, not completed claims.

## Isolation and evidence

Original IDFs and licenses are preserved byte-for-byte, including line endings.
Run-period/control changes and purpose-output application affect only separate
runtime copies. The original, annualized and executed inputs have distinct
paths, hashes and change records. Chicago TMY3 is an explicitly controlled test
weather file; it is not described as each example's native weather location.

An explicit evidence-capture run preserves its inputs, original engine output,
run/plan manifests, actual application result, discovery information and SQL
observations under `.runtime/energy-path-acceptance`. Capture alone is not an
acceptance pass. Reviewed SQL selectors/rules and approved numeric expectations
must cover all eight required groups before acceptance can pass. Neither normal
tests nor capture can silently create or update approved expected manifests.

Independent SQL checks must distinguish weather simulations from sizing/warmup,
verify an actual full annual period and all twelve months, and retain the
distinction between signed source totals and directional allocated contributions.
Actual RDD alias resolution needs dictionary and reported SQL evidence; monthly
heating and cooling totals alone cannot prove simultaneous operation.

## First real-run findings (not an acceptance pass)

The first 25.1 Large Office annual execution completed in 37.21 seconds with
zero severe/fatal engine errors. It retained 15,684 warnings, including original
equipment operating-range warnings and requested-but-unavailable outputs. Its
closed SQL contains 862,128 ReportData observations and 8,772 weather Time rows
for 2017. These are evidence counts, not approved Energy Path expectations.

The run exposed issues that synthetic fixtures had not represented:

- Original hourly output requests survive additive purpose-output application.
  The shared result builder was constructing thousands of unused hourly graph
  periods before discarding them from the annual/monthly interface. Original
  engine output must remain intact; the Energy Path projection needs a bounded
  period-selection correction before this fixture can finish acceptance.
- EnergyPlus writes a NULL `Time.WarmupFlag` for hourly/daily/monthly aggregates.
  Its [25.1 SQL writer](https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/SQLiteProcedures.cc#L1424)
  explicitly binds the warmup flag for detailed/timestep rows but leaves that
  column unbound for aggregate rows. Requiring `WarmupFlag = 0` alone therefore
  incorrectly rejects this real annual result. The independent oracle must
  distinguish these documented aggregate intervals from unknown timestep flags;
  design environments and explicit warmup rows remain excluded.
- `Surface Inside Face Convection Heat Transfer Energy` is absent, while
  `Surface Inside Face Convection Heat Gain Energy` is reported monthly for 158
  surface keys. The actual dictionary name and key must survive alias resolution.
- A people-convective output exists for `BASEMENT` but not `TOPFLOOR_PLENUM`.
  A missing exact Zone key must not inherit availability from another Zone.

The period-selection regression now passes: original Hourly sources remain
available, while runtime Energy Path builds exactly Annual plus twelve monthly
graphs before per-Zone conversion. Paired annual/monthly node and link values
are unchanged; the frozen v1 adapter retains detailed periods. The initial
test-only result-building process was stopped after validating its exact PID,
executable and start time; no engine output was deleted. The fresh capture below
created its own completed run manifest; none was synthesized for the interrupted
result build.

The exact-key discovery regression also passes, including all 158 actual
surface identities and the absent plenum keys. The SQL oracle now handles
documented aggregate NULL flags, excludes explicit warmup/design-day rows and
unknown timestep flags, and keeps RunPeriod/Year scalars out of December.
Its unit tests reject incomplete calendars, duplicate observations, ambiguous
source aliases, unknown intervals, zero/null confusion and partial acceptance
group claims.

The fresh normal capture `real-large-office-25-1-20260907T162304.141598300`
completed in 124.710 seconds including shared result construction and evidence
collection. It has a normal successful run manifest, 13 graph periods, 19 Zone
results and nonempty driver/load/end-use/carrier stages. SQL and manifest
digests bind this capture to its executed input and engine files. Captured
discovery metadata predates the exact-key fix and is not presented as proof of
the corrected discovery path; that path has its separate actual-key regression.

This is still collection, not eight-group numerical acceptance. Complete
independent category/allocation/coverage comparisons and approved expected
manifests remain in progress. No expected manifest has been approved yet.

The extended capture matrix exposed further cases:

- Monthly-only SQL has no January 1 timestamp. Full-year proof must use
  cumulative `Time.SimulationDays` together with monthly endpoints; the SQL
  writer's fixed calendar-month `Interval` alone cannot prove elapsed coverage.
- Some actual RunPeriod energy dictionaries reference the final Monthly Time
  row. Their annual scalar must not be rejected or relabeled as December;
  its final-day/365-day provenance must be checked independently.
- The first Large Office payload reports Drivers 17/18 and Loads 2/14, but
  independent matching of requested alias groups to reported SQL gives 12/18
  and 9/14. Graph-derived driver terms were inflating availability, while
  canonical load deduplication removed genuinely reported diagnostic groups.
  Those initial quality counts are not approved expectations.
- The no-heating data-center example has two engine Severe messages from the
  indirect evaporative cooler's secondary-airflow iteration limit during the
  annual run. Exit code 0 does not override these errors: the capture is rejected
  and retained for diagnosis. No errors, model equipment or weather intervals
  have been suppressed to make it pass.

Both SQL representation guards now have faithful positive and negative tests.
Full monthly coverage requires exactly twelve complete monthly endpoints with
matching cumulative simulation days, even when hourly endpoints also exist.
An annual energy scalar attached to the terminal monthly row stays annual;
an annual rate attached there cannot borrow December's duration. Read-only
rechecks of the saved Large Office, ZoneGroup, Ideal Loads and ZoneMultiplier
SQL all pass the metadata collector. These are not approved numeric manifests.

The runtime completeness correction now counts requested thermal groups from
the original observations, before graph derivation and load deduplication.
A real-shaped regression locks the independently reviewed 18 driver and 14
load request groups, the 12/9 observed groups, explicit reported zeroes, and
exclusion of the five graph-derived extras. Graph nodes/links, source evidence
and the two canonical load nodes remain unchanged; frozen v1 tests pass. A
fresh shared-loader replay remains required to verify corrected actual payloads.

All 19 selected engine executions have finished. Eighteen produced normal
successful engine and canonical results; only the no-heating example was
rejected for engine Severe errors. Nine original metadata collections passed,
including all four Large Office versions. The other nine successful engine
runs were rejected by the SQL guards above and require corrected saved-evidence
verification, not replacement or deletion of their original outputs. No engine
process is left running. No fixture has yet passed eight-group acceptance.

The simultaneous example also has independent time-aligned operating evidence.
In `real-simultaneous-25-1-20260907T163311.967451100`, the CHILLERBANK cooling and
heating water mass-flow, inlet-temperature and outlet-temperature dictionaries
share actual non-warmup Zone Timestep observations in the weather environment.
There are 7,624 ten-minute intervals with both positive flows, cooling inlet
above outlet by more than 0.01 C, and heating outlet above inlet by more than
0.01 C. The first is SQL TimeIndex 13299 (2017-04-03 08:00): cooling flow
0.780015906 kg/s at 14.433971 to 6.67 C, and heating flow 0.937404375 kg/s at
56.737198 to 60 C. This is simultaneous water-side operation, not a claim based
on monthly totals or plant demand. The automated identity-bound check and the
fixture's eight-group numeric manifest remain to be completed.

Independent read-only SQL spot checks for this capture use weather environment
3, monthly dictionaries, `J / 3,600,000`, and no production reader/classifier.
Zone variables are joined by exact Zone name to SQL `Zones`; their effective
values multiply `Multiplier * ListMultiplier` once. Building meters are not
multiplied again. In this model, six middle-floor Zones have multiplier 10.

| Observed quantity | Raw kWh | Effective/building kWh |
| --- | ---: | ---: |
| Zone sensible cooling, summed | 916,474.344071 | 3,548,292.321448 |
| Zone sensible heating, summed | 152,711.384176 | 379,258.541907 |
| Zone lighting electricity, summed | 481,737.464429 | 1,565,646.759393 |
| Zone equipment electricity, summed | 716,594.638178 | 2,328,932.574079 |
| InteriorLights:Electricity | — | 1,565,646.759393 |
| InteriorEquipment:Electricity | — | 2,328,932.574079 |
| Cooling:Electricity | — | 511,048.202956 |
| Heating:NaturalGas | — | 1,473,192.758724 |
| Electricity:Facility | — | 5,712,004.038422 |
| NaturalGas:Facility | — | 1,541,685.323878 |

The agreement between independently multiplied Zone lighting/equipment and
their facility end-use meters is a multiplier cross-check, not a substitute
for the complete category/allocation/coverage expected manifest.

## Explicit execution modes

Use the repository Go toolchain and select the simulation package test by name.
Normal tests never launch EnergyPlus. Environment flags below apply only to the
invoking process; do not persist them in the machine or user environment.

| Test | Explicit inputs | Meaning of success |
| --- | --- | --- |
| `TestEnergyPathRealModelEvidence` | `EPATH_REAL_CAPTURE=1`; optional exact comma-separated `EPATH_REAL_FIXTURES` / `EPATH_REAL_VERSIONS` | Completed evidence collection, not acceptance |
| `TestEnergyPathRealModelAcceptance` | `EPATH_REAL_RUN=1`; reviewed recipe and expected manifest must already exist | New engine run agrees with independent SQL and approved eight-group expectations |
| `TestEnergyPathRealModelSavedEvidence` | `EPATH_REAL_VERIFY_DIR` points to one completed capture | Read-only same-run evidence/recipe check, not acceptance by default |

Saved evidence can explicitly rebuild through the shared public result loader
with `EPATH_REAL_VERIFY_REBUILD=1`; `EPATH_REAL_VERIFY_ACCEPTANCE=1` additionally
requires the approved eight-group expected manifest. Neither mode reruns an
engine, changes original artifacts, or writes expectations. Engine-run and
saved-run modes cannot be combined. Unknown/empty selectors fail rather than
silently skipping the requested model.

## Portable EnergyPlus 22.1 provenance

To test the lower supported version without changing installed applications,
the official [EnergyPlus 22.1.0 release](https://github.com/NatLabRockies/EnergyPlus/releases/tag/v22.1.0)
Windows x64 ZIP was downloaded into `.runtime/energyplus-22.1/downloads`.
The [published SHA-256 file](https://github.com/NatLabRockies/EnergyPlus/releases/download/v22.1.0/sha256sums.txt)
was independently read and matched against the downloaded archive:

| Item | Verified value |
| --- | --- |
| Archive | `EnergyPlus-22.1.0-ed759b17ee-Windows-x86_64.zip` |
| Bytes | 188,188,401 |
| Archive SHA-256 | `6d2aa63f903595d7da22e3ecca6e74dfc0ae23a5f2f26b8e3870ab440d3e7bef` |
| Entries | 3,608; checked for rooted paths, target escape and symlinks before extraction |
| Executable version | `22.1.0-ed759b17ee` from the extracted executable's `--version` |
| Executable SHA-256 | `8e56b46647e109944d8762df6f35eff2fa149c94dc43a78cbf17c69f56e3ee27` |
| Engine DLL SHA-256 | `1e35a0904d1d3106ec8635f75b666a8a5afb1681c1ac1ccc6ebe8f4e2b44fd3e` |
| IDD SHA-256 | `d0ac7568f1992cf2215b8e2453a788bb5fad1275a16ac07de04dbd84d541138d` |

The isolated package directory is
`.runtime/energyplus-22.1/package/EnergyPlus-22.1.0-ed759b17ee-Windows-x86_64`.
There was no installer, registry/PATH change or modification to the existing
23.2, 24.2 or 25.1 installations. The subsequent normal capture
`real-large-office-22-1-20260907T163856.727964100` completed through the engine,
shared canonical builder and metadata collector in 123.20 seconds. As with the
other versions, that completed capture still requires the reviewed eight-group
expected manifest before it can count as numerical acceptance.
