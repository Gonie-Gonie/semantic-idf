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
and the two canonical load nodes remain unchanged; frozen v1 tests pass.

That explicit replay exposed a further real-result rejection: three annual
link-ID groups contain distinct monthly/provenance branches with the same
endpoint-only ID. They are mechanical-ventilation cooling, exterior-wall
cooling, and heating conversion. The latter contains both service-path and
Zone-load fallback allocation, so deleting either branch would lose evidence.
The projection's duplicate-ID guard correctly rejects this payload. Completed
graphs now give every colliding branch a stable, escaped semantic qualifier,
while retaining paired values, source/path identities and previously unique
IDs. Exact semantic duplicates still fail validation; neither array ordinals
nor arbitrary branch deletion are used to hide an ambiguous identity.

The corrected shared-loader replay passed in 104.981 seconds. It verified
Drivers 12/18, Loads 9/14, End uses 9/10 and Carriers 2/2 on the actual canonical
result and selected Building view, two canonical loads, thirteen periods and
nineteen Zones. Fresh discovery matched all 158 surface aliases to their exact
reported key/name/unit/frequency, retained sixteen exact People keys and left
the three absent plenum People keys unresolved. All original capture file
hashes, timestamps and directory entries remained unchanged. This establishes
the corrected loader/discovery path, not eight-group numerical acceptance.

All 19 selected engine executions have finished. Eighteen produced normal
successful engine and canonical results; only the no-heating example was
rejected for engine Severe errors. Nine original metadata collections passed,
including all four Large Office versions. The other nine successful engine
runs were rejected by the SQL guards above and require corrected saved-evidence
verification, not replacement or deletion of their original outputs. No engine
process is left running. No fixture has yet passed eight-group acceptance.

An isolated numerical trial of the no-heating model added only
`HVACSystemRootFindingAlgorithm, RegulaFalsiThenBisection, 5` to a separate
executed copy. Equipment, schedules, weather, timestep and convergence
tolerances remained unchanged, and the baseline artifact hash check passed.
The trial failed with the same two Severe errors at the same January/April
timestamps; warnings increased from 2,135 to 3,158. Its original output remains
under `numerical-trial-no-heating-25-1-hybrid-5-20260907T171012.195010700` and
cannot be used as an accepted fixture or approved expected provenance.

The upstream investigation distinguishes the message from a proven cause:
[General::SolveRoot](https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/General.cc#L193)
can return failure after a collapsed bracket, although this cooler labels that
failure as an iteration-limit error. The dry-cooler branch has a finite output
transition that could prevent the target residual from converging; saved
fifteen-minute SQL averages do not prove the exact failing internal call.
[Upstream PR 11731](https://github.com/NatLabRockies/EnergyPlus/pull/11731)
changes warning/documentation behavior, not the numerical solver. A read-only
review of official 25.1 building examples found no additional confirmed
cooling-only/no-space-heating candidate; nearby examples have connected
baseboards, heating coils or radiant heating. No model was relabeled, no
equipment removed, and no Severe error suppressed to close this requirement.

The simultaneous example also has independent time-aligned operating evidence.
In `real-simultaneous-25-1-20260907T163311.967451100`, the CHILLERBANK cooling and
heating water mass-flow, inlet-temperature and outlet-temperature dictionaries
share actual non-warmup Zone Timestep observations in the weather environment.
There are 7,624 ten-minute intervals with both positive flows, cooling inlet
above outlet by more than 0.01 C, and heating outlet above inlet by more than
0.01 C. The first is SQL TimeIndex 13299 (2017-04-03 08:00): cooling flow
0.780015906 kg/s at 14.433971 to 6.67 C, and heating flow 0.937404375 kg/s at
56.737198 to 60 C. This is simultaneous water-side operation, not a claim based
on monthly totals or plant demand. The automated identity-bound SQL check now
passes: all 52,560 eligible intervals are exactly ten minutes, with 7,624
matching simultaneous intervals. It joins six exact name/key/frequency/unit
identities on the same TimeIndex and checks original catalog/input/engine/
weather/normal-manifest provenance and artifact hashes before and after. Unit
counterexamples reject monthly coincidence, disjoint times, warmup/design
environments, zero/negative flow, missing/nonfinite values and duplicate
identities or observations. The fixture's eight-group numeric manifest remains
required; operating evidence alone does not approve Energy Path quantities.

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

## Independent expectation review rules

The compact oracle recipe may declare exact SQL identities, ordered surface
classification rules, output-family preferences, Zone ownership and service
memberships. It must not read the application's classification, allocation or
quality functions to produce expected numbers. SQL `Zones` and `Surfaces`
provide independent multiplier and physical ownership evidence. Monthly signed
surface values are summed by Zone/category before directional pressure is
split; completed Zone-month allocations are then summed for annual/Building
results. A synthetic pre-allocation closure cannot enter the share denominator.

Surface signs are checked against the actual engine, not inferred from the
application: the [25.1 surface heat-balance reporter](https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/HeatBalanceSurfaceManager.cc)
defines positive inside-face convection as heat entering the surface from air.
The Zone-air contribution therefore uses the opposite sign.

Reported source pressure and a visible flow node have different coverage when
actual delivered service is zero. For example, the Large Office Basement ground
floor has 13,120.098969 kWh of annual raw heating-direction pressure. Actual
heating occurs only in January, February and December. Its visible allocated
flow is 28.885 kWh, with visible-node raw pressure 3,266.464 kWh for those three
months. The remaining raw source observations must remain traceable; absent
flow nodes cannot be interpreted as absent or zero SQL source measurements.

Approval requires exact units/domains, known-zero versus absent-value checks,
unambiguous scope/period/row selectors, paired thermal/site ratio checks, valid
requested-output counts, and complete metric coverage. Reconciliation rows
must be selected by their actual identity; unsupported filters cannot be
silently ignored. Numeric tolerances must be finite, explicitly bounded and
justified by the three-decimal serialization/aggregation precision, not chosen
to hide a physical or ownership mismatch. The expected manifests remain absent
until those safeguards and independent metric comparisons pass.

The first reviewed source recipe, `oracles/large-office-25-1.json`, contains
literal source identities, physical memberships and comparison rules, not
approved candidate numbers. Its independent monthly SQL compilation passes
for nineteen Zones and 3,768 signed monthly source cells. Its unrounded cooling
and heating totals agree with the independent spot checks above. All 163
distinct requested names in its availability rules occur in the actual
executed output plan; meter requests are read from their `Key Name` fields.
Candidate comparisons and the complete coverage ledger are still in progress,
so no numeric expected manifest is approved by this recipe alone.

The first complete diagnostic now runs all eight groups against the corrected,
hash-bound Large Office candidate. It checked 17,469 independent metrics
(3,374,329 bytes if serialized) and **rejected** the candidate in 62.548 seconds.
These rejection counts include shared validation failures before a numeric
comparison; they are not counts of distinct incorrect physical quantities.

| Group | Candidate-bound checks | Rejected checks |
| --- | ---: | ---: |
| Drivers | 15,250 | 11,476 |
| Loads | 1,712 | 1,224 |
| End uses | 109 | 86 |
| Carriers | 30 | 24 |
| Ratios | 52 | 52 |
| Completeness | 4 | 0 |
| Residuals | 78 | 72 |
| Zone allocation | 234 | 216 |

A concrete shared blocker is duplicate monthly reconciliation identity.
Building January contains 110 rows but only 94 unique IDs. For example,
`reconcile.driver.internal.basement.M1` labels two different observations:
888.564 kWh latent and 9,756.799 kWh sensible. Their source sets differ and both
must survive. The independent graph guard rejects these ambiguous rows instead
of selecting one. The annual wrapper does not have this particular collision.

The diagnostic also deliberately rejects unresolved presentation-precision
cases, such as December cooling electricity of 0.0004987871280347543 kWh, whose
propagated rounding interval includes zero. A known SQL source is not a missing
source, but a pruned display node and its ratio require separate presence rules.
Zero-crossing pressure intervals, strict candidate-extra/required-selector
coverage, required numeric wire presence, complete thermal and per-Zone
allocation closure, and this denominator policy remain acceptance gates. No
expected values were written, and failed checks were not loosened to pass.

The heating conversion's serviced thermal scope is also checked independently.
The original Large Office has exactly sixteen `ZoneHVAC:EquipmentConnections`
and three separate return-plenum Zones. January's SQL effective heating is
94,504.151627 kWh in the sixteen conditioned Zones and 29,310.660434 kWh in the
three plenums. The service-path conversion numerator covers the former, not the
entire nineteen-Zone total. July/August have zero conditioned-Zone heating and
positive plenum heat; their Zone-load fallback is not a direct equipment
measurement. A ratio expectation must preserve that method/scope distinction
instead of substituting a Building-total numerator or hiding the fallback.

The corrected replay also exposed a precision-driven availability bug in June:
the service-path pair is 0.157 kWh thermal / 1,501.071 kWh site, but rounding its
positive dimensionless ratio to three decimals erased the ratio kind. The
builder now retains the positive paired ratio when that rounding would produce
zero; ordinary rounded ratios and all paired energy quantities remain unchanged.
A new hash-bound replay returns efficiency 0.00010459198798724378 and June ratio
availability 2/2 instead of 1/2. Comparison of all 260 period graphs found three
affected conversion links and no node/link/reconciliation quantity or identity
changes; reconciliation source-ID ordering differed but the complete multisets
were preserved. Original engine captures were not modified.

The browser presents these known small values as locale-aware `<0.01`, including
ribbon labels/tooltips, KPIs and link/node inspectors. Numeric attributes and
JSON retain the underlying ratio. Actual-app tests cover the June pair, a tiny
COP, unchanged ordinary 0.85/4 ratios, unavailable/invalid inputs, three locales,
immutable geometry on selection, and zero Run/Analyze calls. Existing ratio,
KPI and inspector regressions also pass.

## Explicit execution modes

Use the repository Go toolchain and select the simulation package test by name.
Normal tests never launch EnergyPlus. Environment flags below apply only to the
invoking process; do not persist them in the machine or user environment.

| Test | Explicit inputs | Meaning of success |
| --- | --- | --- |
| `TestEnergyPathRealModelEvidence` | `EPATH_REAL_CAPTURE=1`; optional exact comma-separated `EPATH_REAL_FIXTURES` / `EPATH_REAL_VERSIONS` | Completed evidence collection, not acceptance |
| `TestEnergyPathRealModelAcceptance` | `EPATH_REAL_RUN=1`; reviewed recipe and expected manifest must already exist | New engine run agrees with independent SQL and approved eight-group expectations |
| `TestEnergyPathRealModelSavedEvidence` | `EPATH_REAL_VERIFY_DIR` points to one completed capture | Read-only same-run evidence/recipe check, not acceptance by default |
| `TestEnergyPathRealLargeOfficeSavedReplay` | `EPATH_REAL_REPLAY_DIR` points to the exact 25.1 Large Office capture | Corrected shared-loader and exact-key discovery regression, not eight-group acceptance |
| `TestEnergyPathRealSimultaneousSavedSQL` | `EPATH_REAL_SIMULTANEOUS=1` | Time-aligned, identity-bound dual water-side operation in the retained simultaneous fixture, not eight-group acceptance |
| `TestEnergyPathNoHeatingHybridSolverTrial` | `EPATH_REAL_NUMERICAL_TRIAL=1` and explicit failed `EPATH_REAL_BASELINE_DIR` | Separate one-object numerical experiment only; no accepted fixture defaults or expectations are changed |
| `TestEnergyPathRealOracleMaterializeCandidate` | `EPATH_REAL_ORACLE_CAPTURE_DIR` and a new explicit `.runtime` `EPATH_REAL_ORACLE_SNAPSHOT_NEW` path | One shared-loader rebuild; new candidate and provenance sidecar only, never overwrites an existing snapshot |
| `TestEnergyPathRealSQLModelSavedCandidate` | `EPATH_REAL_ORACLE_CAPTURE_DIR` and `EPATH_REAL_ORACLE_SNAPSHOT` | Independent eight-group diagnostic; any mismatch fails and does not approve expectations |

Saved evidence can explicitly rebuild through the shared public result loader
with `EPATH_REAL_VERIFY_REBUILD=1`; `EPATH_REAL_VERIFY_ACCEPTANCE=1` additionally
requires the approved eight-group expected manifest. Neither mode reruns an
engine, changes original artifacts, or writes expectations. Engine-run and
saved-run modes cannot be combined. Unknown/empty selectors fail rather than
silently skipping the requested model.

Materialized candidates bind the original capture, SQL, executed input, engine,
weather and all current non-test Go source contents (including uncommitted
changes). Candidate bytes have their own hash and `acceptance: false` marker.
Changed production code or source provenance invalidates the snapshot; an old
snapshot is retained as evidence and is not silently refreshed or reused.

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
