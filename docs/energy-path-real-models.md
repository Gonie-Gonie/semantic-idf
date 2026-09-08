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
positive plenum heat. A subsequent topology audit confirmed that the existing
fallback escapes the known service recipients: 375.593807663247 and
1,393.570040051900 kWh of gas are assigned to three unserved plenums. This is a
production allocation defect, not an approved alternative allocation policy.
The known connected scope must remain bounded even when all its current loads
are zero; energy without a valid denominator must remain Building-unassigned.
The genuinely missing-topology fallback is a separate compatibility case.

The same independent audit found four already-reported Hourly/J
`Air System Fan Electricity Energy` series, keyed to VAV_1, VAV_2, VAV_3 and
VAV_5. Their twelve monthly pool sums match `Fans:Electricity` to less than
5.1e-10 kWh. The original IDF connects the first three loops to their own five
conditioned Zones and VAV_5 to Basement. Existing allocation tests cover
preseeded separate fan pools, but actual SQL discovery currently discards these
four series and allocates the broad meter across all sixteen Zones. Real
per-loop ingestion and non-double-counted pool allocation remain required;
new heavy output requests are not needed or authorized by this fix.

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

The candidate reader now checks original JSON numeric presence before invoking
the application's compatibility decoder. Node values, each sensible/latent
breakdown value, paired link quantities, reconciliation expected/explained/
residual values, five quality count pairs and quality percentages must be
present, non-null finite numbers in every Building/Zone/period graph. Optional
raw/effective/allocation fields may remain omitted/unknown, but explicit null
cannot masquerade as a reported zero. Tests reject missing/null/string/boolean
numbers in all four nesting contexts and retain exact reported zeroes. The
preserved `large-office-25-1-candidate-20260908-0350.json` passed the initial
numeric-presence gate in 2.113 seconds. A subsequent count invariant also rejects
`found > total`; that older candidate has Zone counts of 9/0 and 2/0 and does
not pass the strengthened gate. This historical check proved numeric presence
only, not numerical acceptance.

### Second actual-candidate diagnostic: structural blocker removed

`large-office-25-1-candidate-after-reconciliation-01.json` was materialized once
in 103.150 seconds from the same immutable SQL capture. Constructor-owned
sensible/latent reconciliation identity is retained privately until the v2
boundary, which qualifies colliding rows consistently across all periods.
Labels distinguish Sensible and Latent. Stored combined annual accounting is
split only when all twelve unique months, three signed sums, semantics and the
exact source union agree. Unknown/conflicting source components are not guessed,
and the frozen v1 JSON identity and aggregation remain unchanged.

The raw-wire and interval-aware independent diagnostic then completed in
63.691 seconds. It still **failed**, with 218 mismatches across the same 17,469
checks (3,261,116 bytes of numeric evidence). Monthly reconciliation guard
failures were eliminated; the remaining discrepancies are now inspectable
numeric/presence/metadata cases rather than ambiguous graph identities.

| Group | Checks | Remaining mismatches |
| --- | ---: | ---: |
| Drivers | 15,250 | 183 |
| Loads | 1,712 | 0 |
| End uses | 109 | 2 |
| Carriers | 30 | 0 |
| Ratios | 52 | 25 |
| Completeness | 4 | 0 |
| Carrier residuals | 78 | 0 |
| Zone allocation | 234 | 8 |

The test-only interval policy keeps exact raw SQL centers, explicitly bounded
optional presentation ranges, and independently checked conversion endpoints.
Only fully observed quantities whose proven presentation interval includes zero
may be pruned. Unknown/null data, negative directional quantities, wrong units,
and ratios inconsistent with their actual paired values still fail. A tiny
positive denominator is not relabelled as a physical zero.

Two follow-up corrections are separately regression-tested, awaiting another
actual candidate. The allocation planner now receives period-independent known
service topology; July/August's unavailable denominator cannot escape to the
three plenums. Derived Zone balance terms also retain native-Zone-equivalent
raw metadata alongside their already-effective result. Previously a factor-10
derived difference incorrectly contributed its model-total value to the raw
column. In January, the affected Building storage/heating raw sum was
10,236.296 kWh; applying the original Zone multipliers to those same effective
terms gives 7,453.820 kWh, within the independently computed raw interval around
7,453.81703904 kWh. Effective pressure, delivered load and allocated contribution
are not rescaled. The source remains explicitly `derived_formula`, and its raw
equivalent is labelled as calculated, not a new SQL measurement. Tests cover
Zone/ZoneGroup factors, signed monthly values with annual net zero, unchanged
original observations, repeated preparation and JSON reload.

The first ratio mismatch samples also exposed an oracle schema assumption:
canonical end-use nodes may express their service through typed `endUse` while
omitting redundant `serviceKind`. The strict reader now accepts only the exact
matching typed cooling/heating end use in that case; contradictory explicit
service, wrong type, paired units or quantities remain rejected. These checks
do not approve the unreviewed original fallback allocation.

### Third actual-candidate diagnostic: physical values and remaining wire cases

`large-office-25-1-candidate-after-real-fixes-01.json` was materialized once in
118.253 seconds. The independent diagnostic completed in 73.889 seconds and
still **failed**. It now includes 255 additional fan-pool checks, for 17,724
checks and 3,307,635 bytes of numeric evidence. The complete failed-check report
is retained as `large-office-25-1-diagnostic-after-real-fixes-01.json` alongside
the candidate and provenance; it does not write or approve expected manifests.

| Group | Checks | Remaining mismatches |
| --- | ---: | ---: |
| Drivers | 15,250 | 46 |
| Loads | 1,712 | 0 |
| End uses | 117 | 0 |
| Carriers | 30 | 0 |
| Ratios | 52 | 0 |
| Completeness | 4 | 0 |
| Carrier residuals | 78 | 0 |
| Zone allocation | 481 | 216 |

All previously failing derived raw quantities now agree with their independent
SQL intervals. The 46 driver failures are omitted, exactly reported zeroes in
Zone-scoped source details: 19 InterzoneAir sources and four OutdoorAir sources,
each with raw/effective scalar checks. The original source-level zero fix did
not yet propagate the scoped source's own proof to its detail wrapper.

The 208 new fan failures are a selector-contract error, not missing fan nodes:
canonical end-use nodes deliberately omit a single carrier because they can
aggregate multiple carriers. For example, Basement January contains the fan
node with 448.694 kWh, its exact VAV_5 source, and an outward electricity link.
The fan oracle incorrectly required `node.carrier = electricity`. The remaining
eight checks describe wholly absent cooling allocation rows for January's
reported zero and December's fully observed, sub-display-precision amount.
Their presence policy must be proved at the whole-row level, not by treating
arbitrary missing fields as zero.

The fan ingestion now independently reads existing exact Hourly/J SQL sources
without adding heavy output requests or double-counting the broad meter. Four
reviewed pools retain the original 5/5/5/1 connected Zone partitions, while
unallocated energy remains explicit. Independent saved-SQL checks confirm all
four 8,760-hour axes and twelve monthly pool sums. Production regressions also
cover legitimate simultaneous Monthly reporting, explicit owner-path limits,
and unrelated unresolved fans without discarding a valid measured pool.

Known reported zero proof requires valid observation rows, not dictionary
presence. Monthly sources must cover the complete observed reporting axis
exactly once; missing/null/duplicate/invalid rows do not prove zero. A genuine
three-month run can establish zero through its three reported months without
pretending to be a twelve-month run. Frozen v1 source JSON is unchanged.

The run-level quality fallback no longer combines an observed numerator with
an unknown requested denominator (9/0 or 2/0). Such counts remain unknown as
0/0 with partial/unavailable status; the observed inventory and limitations
remain in the explanation, including allocated Zone carrier subtotals. Known
requested counts remain exact. The strengthened raw-wire gate passes on this
third candidate. This does not retroactively repair older stored metadata.

The full repository verification also exposed two regressions before the
checkpoint: the known-recipient filter omitted legitimate IdealLoads `mixed`
service paths, removing conversion links. The corrected filter accepts exact
indexed mixed recipients for cooling/heating, while excluding ventilation-only
or unserved Zones and retaining zero-denominator energy as unassigned.
Focused regressions pass; full verification is rerun before committing.

Finally, the test-only candidate reader now bypasses the application's
permissive result/period/summary compatibility readers. Original graph values,
units, invalid links, duplicate rows and source scalar presence reach the
independent checks without repair or pruning. This closes a separate acceptance
gap; it does not change the application's saved-result compatibility behavior.
Another immutable actual candidate and complete comparison are still required.

### Original-wire recheck and fan rounding correction

`large-office-25-1-candidate-after-real-fixes-02.json` was built in 112.291
seconds and compared without compatibility repair in 50.305 seconds. The
17,724 checks (3,307,663 bytes) reject only four fan Zone-month values; all
other comparisons, including the 46 scoped zeroes and eight wholly pruned
allocation fields, pass. Original failed candidates and reports remain intact.

Each remaining case is the last recipient of a five-Zone fan pool. The old
apportionment independently rounds earlier recipients and gives their combined
rounding remainder to the last Zone. That can exceed the last Zone's own
one-quantum allocation bound: for example, Perimeter_bot_ZN_4 April is 77.124
kWh against an independently bounded interval of [77.1240275297,
77.1264612075] kWh. The oracle's bounds and source centers are unchanged.

Fan pools now use largest fractional remainders for every pool, not only when
tiny allocations would turn negative. Every recipient receives either floor
or ceil of its own exact three-decimal quota, the rounded pool total is exactly
preserved, zero-weight recipients stay zero, and tied rounding ownership uses
stable semantic identity. Rotation/reversal tests cover tiny and ordinary
pools; fan, EPATH-094/100/101 and frozen v1 regressions pass in 7.533 seconds.
`large-office-25-1-candidate-after-real-fixes-03.json` was materialized in
106.066 seconds. The original-wire comparison then **passed all 17,724 checks**
in 58.151 seconds (3,307,689 bytes of numeric evidence), with the same independent
centers and intervals. The separately saved `large-office-25-1-diagnostic-after-real-fixes-03.json`
has an empty failures array and remains explicitly `acceptance: false`.

The passing groups contain 15,250 driver, 1,712 load, 117 end-use, 30 carrier,
52 ratio, four completeness, 78 carrier residual and 481 allocation checks.
This is the first passing complete numeric diagnostic for this candidate, not
approval of the full fixture suite. Required-selector/extra-record coverage,
complete per-Zone service/direct-use and link-closure evidence, approved
eight-group expected manifests, other model types and the unresolved no-heating
engine result still require completion in section 22. No later checklist item
is marked complete on the strength of this diagnostic.

The checkpoint's full `scripts/verify.ps1` passes after the apportionment fix:
main package 24.826 seconds, CLI 7.896 seconds, actual frontend checks 160.543
seconds, simulation 107.165 seconds, and a successful Wails production build
in 10.101 seconds. The commit hook repeats the repository verification; it is
not bypassed.

### Full-record coverage checkpoint: exact Zone flows and provenance

The next test-only checkpoint adds independent Driver-to-Load link, per-Zone
HVAC/carrier allocation and direct lighting/equipment proofs. Expected values
come from the preserved SQL observations and reviewed IDF ownership, not from
candidate graph totals or production allocation helpers. Monthly calculations
precede Annual sums, SQL Zone multipliers apply once, and known HVAC recipients
cannot expand into unserved plenums when their load denominator is zero.
Direct-use declarations enumerate the sixteen original owners of each output;
undeclared, missing, NULL, duplicated and negative observations fail.

Link checks bind both endpoint quantities, direction, units/domain, basis and
exact original SQL source identities. Driver provenance traverses derived
inputs to the required original leaves. HVAC carrier branches retain their own
carrier sources and exact Zone-load evidence; lighting/equipment links cannot
invent a thermal conversion ratio. Optional redundant service tags are allowed
only when typed endpoints identify the correct service; contradictory tags fail.
Negative fixtures cover swapped Zone allocations with unchanged Building totals,
wrong source ownership, cross-carrier substitution, missing branches and
duplicate records.

Real-model compilation now also requires a complete-record coverage ledger.
It rejects unchecked primary node/link/reconciliation/quality fields, absent
required selectors, unexpected records and conflicting Annual wrappers. An
isolated scalar unit fixture may intentionally omit this gate; real candidate
and acceptance compilation cannot. Coverage failures prevent the corresponding
group from being approved even when every compiled numeric check passes.

The fresh, current-production-code snapshot
`large-office-25-1-candidate-coverage-01.json` was materialized in 21.740 seconds
without rerunning EnergyPlus or changing the original capture. Its SHA-256 is
`2ddb90dbf5b7f1a71088a998f4d1d39c743deef73bebe1c141146bdc12fb95e8`.
The bound original SQL remains
`8ea939c1d962aa37b2c246e8c833427de7a38ef3ed6d98f241e9ed9cc8e5bc46`.

The first numerically passing full-record diagnostic in this checkpoint,
`large-office-25-1-diagnostic-coverage-03.json`, completed in 29.900 seconds.
All **31,928 numeric and contract checks pass**, producing 6,066,305 bytes of
numeric evidence. The full diagnostic still **fails** with 12,127 uncovered
record-field obligations across a 13,996-record ledger:

| Group | Numeric/contract checks | Numeric/contract failures | Coverage gaps |
| --- | ---: | ---: | ---: |
| Drivers | 15,250 | 0 | 0 |
| Loads | 11,836 | 0 | 0 |
| End uses | 117 | 0 | 596 |
| Carriers | 30 | 0 | 348 |
| Ratios | 1,040 | 0 | 260 |
| Completeness | 4 | 0 | 1,036 |
| Residuals | 78 | 0 | 9,367 |
| Zone allocation | 3,573 | 0 | 520 |

These gaps are not 12,127 proven numerical mismatches, nor are they accepted
exceptions. They require independent reconciliation totals, remaining Zone
subtotals and per-context quality/status evidence before acceptance. The next
work remains inside section 22: thermal and carrier reconciliation, then
complete availability/ratio/closure/allocation quality proofs, followed by the
other official model recipes and approved eight-group expected manifests.

Earlier diagnostic attempts remain distinct. Report `coverage-01` contains
632 redundant-service-tag failures plus 12,759 coverage gaps. The next attempt
stopped during compilation and did not create report `coverage-02`: subtraction
rounding made a redundant symmetric-interval validation reject three large,
otherwise valid SQL observations. The correction checks the explicit-bound
consistency condition only when explicit bounds exist. Regression tests use
the exact twelve SQL observations and assert unchanged numeric centers, error
budgets and bound bits; invalid explicit bounds, NaN/Infinity and negative
error budgets still fail. No expected interval was enlarged to obtain the
passing numeric comparison in `coverage-03`.

Pre-commit cross-review adds sum-preserving duplicate-node and provenance
mutants. A second reported-meter Building node cannot hide behind an unchanged
category sum. Moving a required driver source onto the load endpoint cannot
prove the driver's own contribution. Cosmetic RuleID/explanation or derived
source-ID changes cannot disguise a duplicate physical branch; independently
distinct original sources or paths still permit legitimate parallel branches.

The stricter `coverage-04` diagnostic retains 31,928 checks and finds four
contract failures, plus 12,133 coverage failures. All four contract failures
refer to one Annual mechanical-ventilation/heating branch and its aggregate
checks. Its 0.001 kWh contribution occurs only in April. The canonical Annual
load includes Basement heating source 1930, but Basement has no displayed
April heating load: its visible heating occurs in January, February and
December. Requiring that source on this April-only branch would manufacture
temporal overlap. The complete canonical load endpoint and the branch's
month-local provenance therefore need separate independent presence rules;
the original candidate and failed report are preserved. No numeric center or
tolerance is changed.

The corrected proof keeps every independently positive completed-month load
contributor required on the canonical Annual endpoint. Each branch separately
requires contributors from months in which its own driver category has a
proven positive allocation interval; the all-incoming proof also uses that
branch's category. Independent two-Zone, different-month fixtures accept the
unrelated-month omission while rejecting a missing Annual endpoint contributor
or an active-month link source. Existing source-moving, duplicate-branch and
zero/tiny interval regressions remain passing. Focused driver-link tests pass
in 6.520 seconds and coverage tests in 0.621 seconds.

The final unchanged-candidate diagnostic,
`large-office-25-1-diagnostic-coverage-05.json`, completes in 29.930 seconds:
all 31,928 numeric/contract checks pass again (6,066,386 bytes of numeric
evidence), including the strengthened endpoint, temporal and physical-branch
proofs. Its remaining 12,127 coverage failures have exactly the group counts
shown above, with no Driver or Load gaps. The full diagnostic remains a
failure and explicitly `acceptance: false`; numerical success does not erase
the still-unchecked reconciliation, subtotal and quality fields.

All original capture files and earlier diagnostic artifacts are retained.
This checkpoint changes test/oracle code and documentation only, writes no
approved expected manifest, and leaves section 22 incomplete.

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
| `TestEnergyPathRealOracleCandidateWireSaved` | `EPATH_REAL_ORACLE_WIRE_PATH` points to one preserved candidate | Read-only original numeric presence, including older snapshots; no SQL comparison or acceptance |

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
