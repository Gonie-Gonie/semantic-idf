# Energy Path actual-model acceptance

Checklist section 22 is in progress. The frontend/SQL unit fixtures documented
in `energy-path-acceptance.md` do not constitute EnergyPlus model-run acceptance.
Official model inputs, licenses and a versioned catalog live under
`cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models`.
Coverage tags in that catalog are obligations to verify, not completed claims.

Current checkpoint: Large Office, Small Office, Ideal Loads, PTAC and PTHP 25.1
have explicitly reviewed expected manifests and passing saved-result acceptance
for all 46,224, 17,675, 17,121, 17,021 and 17,113 independent metrics respectively.
PTHP source/ownership, whole-row-zero proofs and review evidence are detailed in
[PTHP acceptance](energy-path-pthp-acceptance.md); the preceding PTAC checkpoint
is recorded in [the progress ledger](energy-path-progress.md). The remaining 14
catalog entries, including Large Office 22.1/23.2/24.2, are not yet approved.
Earlier findings below are retained as a chronological record, not presented
as the current acceptance status. Section 22 and later checklist sections remain
in progress.

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

At that checkpoint this was still collection, not eight-group numerical
acceptance. Complete independent category/allocation/coverage comparisons and
approved expected manifests remained in progress; none had yet been approved.

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

## Complete accounting and context coverage follow-up

The next independent compilation adds thermal reconciliation (including
Zone-only contributions), monthly-first Building carrier branches, Zone fan
branches and carrier subtotals, positive site-residual presentation, and all
nine availability/ratio/accounting quality fields for every scope-period.
Annual ambiguous HVAC branch presence retains discrete completed-month choices:
a possible zero-or-full monthly meter is not permission to split that month
fractionally between paired and direct consumption.

The unchanged-candidate `large-office-25-1-diagnostic-coverage-06.json`
contains 46,224 checks and finishes in 40.680 seconds. It fails with 2,230
numeric/contract failures and 3,696 coverage gaps. All 9,828 reconciliation /
site-residual checks pass. The failed artifact is preserved, not an expectation.
Investigation separates two production defects from two oracle errors:

- Zone quality recalculates Building-wide allocation coverage using only its
  repeated fan/pump rows, omitting cooling/heating accounting. In October the
  same-period complete Building coverage is 81.584%, but the Zone displays
  43.991%. The correction copies only allocation percentage, unassigned
  percentage and status from the matching Building period; other Zone quality
  fields remain local, and a missing month cannot borrow Annual coverage.
- The fan consumption branch preserves the broad Fans meter but loses the
  exact reported AirLoop pool used for its allocated value. That pool must
  remain on the carrier-qualified branch. Load-weight observations belong to
  the allocated node's context, not additional site consumption.
- The initial fan proof omitted legitimate broad-meter and exact selected-Zone
  load-weight context from its node-source allowance. The corrected proof binds
  those original identities independently and still requires the exact pool on
  both node and consumption branch; foreign loops/Zones remain rejected.
- Zone carrier names are case-insensitive EnergyPlus identities. SQL
  `BASEMENT` / `CORE_MID` and canonical `Basement` / `Core_mid` agree. Only that
  comparison is corrected; period, carrier, unit, domain and basis stay strict.

The remaining 208 residual-group coverage failures in report 06 are failed
Zone carrier-closure quality prerequisites (16 conditioned Zones x 13 periods),
not unchecked reconciliation rows. Coverage and acceptance gates remain enabled.

After these corrections, the new immutable candidate
`large-office-25-1-candidate-coverage-02.json` is materialized in 20.110 seconds
from the same original SQL, without an engine run. Its SHA-256 is
`386b37d58280e9c21d2313846cabc75647c89920f72c2915d58bb8628d7da1a6`.
The recipe SHA-256 remains
`57bf54aee4b2b3477da22f7fcd5e871d90696268c5b13e13990e950e3294a085`.

`large-office-25-1-diagnostic-coverage-07.json` completes in 40.480 seconds.
All **46,224 numeric/contract checks pass**, and all **13,996 ledger records**
have complete required field coverage, with **zero coverage gaps**:

| Group | Independently checked metrics | Failures | Coverage gaps |
| --- | ---: | ---: | ---: |
| Drivers | 15,250 | 0 | 0 |
| Loads | 11,836 | 0 | 0 |
| End uses | 533 | 0 | 0 |
| Carriers | 1,018 | 0 | 0 |
| Ratios | 1,300 | 0 | 0 |
| Completeness | 1,560 | 0 | 0 |
| Residuals | 9,828 | 0 | 0 |
| Zone allocation | 4,899 | 0 | 0 |

The independent numeric evidence contains 8,928,359 JSON bytes. Original
capture files and every failed/successful diagnostic are retained. This is the
first complete-record Large Office diagnostic, still explicitly
`acceptance: false`: reviewed expected-manifest approval and the other required
models/adapter versions remain separate section 22 work. The new thermal
reconciliation compiler does not yet prove interzone-pair rows for models that
contain them; this actual capture has no such rows, not an accepted exception.

An additional explicit diagnostic, `large-office-25-1-diagnostic-coverage-08.json`,
passes the same 46,224 checks and complete ledger in 41.310 seconds and writes
`large-office-25-1-pending-coverage-01.json`. This is a new **unapproved** review
artifact containing the independent metrics, full coverage ledger, exact key
registry digest and candidate/recipe/production/original-run provenance. It is
not an expected manifest and cannot be read as one. No approved expected file
is created or updated by a passing diagnostic.

## First approved expected manifest and saved acceptance

The 2026-09-08 explicit review approves Large Office 25.1 expectations from
pending artifact SHA-256
`16c1d29274ad9fa33ceabdf30917fe9e9b054c66e5aa4efa5bed0302a98c7618`.
Separate read-only checks confirm the 46,224-key registry, 13,996-record ledger,
44,278 known metrics (including 15,296 exact zeros) and 1,946 explicitly unknown
metrics. Independent monthly-to-annual checks cover 133 Zone/family combinations;
494 Zone/period/carrier component sums also agree. A separate original-SQL
calculation checks Core_mid lighting/equipment, its exact VAV_2 fan pool, HVAC
shares and the July/August gas energy that remains unassigned when served
heating load is zero. These checks do not copy candidate values into expectations.

The authored approval is
`testdata/energy_path_real_models/expected/large-office-25-1.json` in the
simulation package. All 46,224 metrics are retained in its
`large-office-25-1.metrics.json.gz` companion: **394,508 bytes**, compared with
8,928,225 uncompressed JSON bytes. Compressed SHA-256 is
`c0ab6abb34a501d677e706a06eb22f76f5dd3d140f3f2df5673d410d4699db2a`;
uncompressed SHA-256 is
`998ef9d88e54bb7f2e11e89f6102df65b8e267fedcb8409d3b61d7d098193c51`.
This is lossless storage, not a smaller selector set. Exact key/count/status,
known/unknown and numeric comparisons are unchanged after loading.

`TestEnergyPathRealModelSavedEvidence` passes in **72.320 seconds** with
`EPATH_REAL_VERIFY_ACCEPTANCE=1`, the preserved capture directory and
`EPATH_REAL_VERIFY_SNAPSHOT` pointing to candidate `coverage-02`. It reports
independent SQL and approved expectations verified for all eight groups and
46,224 semantic metrics. The source/wire/SHA/complete-coverage gates remain
enabled. No engine rerun, canonical rebuild or original artifact write occurs.
This is the first numerical acceptance checkpoint, not completion of section
22 or the later performance/cleanup/user-flow requirements.

The companion generator has two explicit modes selected by
`EPATH_REAL_EXPECTED_PAYLOAD_MODE` and requires
`EPATH_REAL_EXPECTED_PENDING` plus `EPATH_REAL_EXPECTED_REVIEW_SHA256`.
`prepare` also requires a new `.runtime` `EPATH_REAL_EXPECTED_PAYLOAD_NEW`
destination; it produces bytes and a descriptor, never approval. `install`
requires the matching review header to exist at the unchanged catalog path and
generates only its new companion. Both verify original capture, current code,
candidate and recipe provenance, complete metric/coverage evidence and the
explicit review fingerprint. Existing files are never overwritten.

Focused compressed-reader, generator and checked-in-artifact tests pass in
2.136 seconds. They retain plain inline-v1 compatibility and reject ambiguous
inline/companion headers, corrupt/truncated/extra-member gzip data, duplicate
JSON members, missing explicit value fields, path escape and changed hashes,
counts or required keys. The explicit install step preserves the authored
review header; ordinary tests cannot silently create or approve expectations.

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

With explicit saved acceptance, `EPATH_REAL_VERIFY_SNAPSHOT` may instead select
an existing SHA-bound candidate through the original-wire reader. It requires
`EPATH_REAL_VERIFY_DIR` and `EPATH_REAL_VERIFY_ACCEPTANCE=1`, is mutually exclusive
with rebuild, and still requires the catalog's approved expected manifest.
Missing fields, stale production hashes or changed original artifacts fail;
the reader does not normalize away invalid original candidate data.

For the saved-candidate diagnostic, an additional explicit
`EPATH_REAL_ORACLE_PENDING_NEW` destination may save pending review metrics only
after every independent check and coverage obligation passes. Its destination
must physically remain inside `.runtime` and must not already exist. Failed
checks, incomplete groups/registry, changed provenance or an existing file are
rejected. This path neither approves nor overwrites expected fixtures.

Materialized candidates bind the original capture, SQL, executed input, engine,
weather and all current non-test Go source contents (including uncommitted
changes). Candidate bytes have their own hash and `acceptance: false` marker.
Changed production code or source provenance invalidates the snapshot; an old
snapshot is retained as evidence and is not silently refreshed or reused.

## Small Office complete-coverage diagnostic

The next fixture remains the original Small Office 25.1 capture
`real-small-office-25-1-20260907T162852.542430100`. Its SQL SHA-256 is
`101559a393b67a3c1d89811df39e86299ab4a902d1f148b0e27f4d0537f7c6fc`;
the engine completed successfully with 498 retained warnings and zero severe
errors. No model changes or engine rerun were needed for this comparison.

Independent preparation adds five exact `Air System Fan Electricity Energy`
Hourly/J pools, each tied to its original single served Zone, and two Monthly/J
direct-use declarations for the five actual Lights/ElectricEquipment owners.
Attic has neither direct-use source nor fan path. The independently observed
`Pumps:Electricity` annual value is 3.5376923076923375e-8 kWh, not an absent or
exact-zero source, and its domestic-hot-water-only pool remains unassigned.
All existing recipe physics, source preferences and precision remain intact.

Two test-oracle omissions are corrected with failing-before/passing-after
regressions. Exterior `Floor` belongs to the Ground / floor category without
changing its Outdoors boundary; zero delivered loads must not erase the signed
surface observations. Thermal reconciliation now respects each detail family's
explicit Zone membership: a reviewed nonmember contributes only arithmetic
zero, whereas missing applicable observations and fabricated outside-member
cells still fail. The Large Office approved saved acceptance remains passing
after these corrections (46,224 checks, 73.260 seconds).

The new `small-office-25-1-candidate-coverage-01.json` is rebuilt from original
SQL in 3.670 seconds and bound to production SHA-256
`f638eacd43c0ced5e1abde7eee5433bd073af7e72a1e578e09d2aa05b207c0ae`.
Its SHA-256 is
`29c086e60e50fba1c077130c1fac299168fa403c3bd458a782ad390723dca7da`.
The separately preserved `small-office-25-1-diagnostic-coverage-01.json`
compiles all 17,675 independent checks but fails in 9.130 seconds with 1,013
numeric/contract mismatches and 1,606 coverage failures across 4,866 records.
This is explicitly a failed diagnostic, not acceptance or an approved expected
manifest. The eight check-group counts are Drivers 6,115; Loads 4,624; End uses
535; Carriers 342; Ratios 455; Completeness 546; Residuals 3,354; Zone allocation
1,704. The failure record is retained while its causes are corrected; it is not
replaced with a smaller passing subset.

The complete mismatch audit identifies four causes rather than 2,619 unrelated
defects. The production HVAC service model did not attach native air-side DX
conditioning to its terminal path and mistook `NoReheat` for a heating source.
Five original, continuously connected DX-wrapper/coil and gas-coil supply
branches now supply typed cooling/heating evidence; the terminal alone supplies
neither thermal source. Exact selected-loop demand-node membership prevents a
Zone with two loops and two terminals from cross-assigning their services.
The new inference is limited to validated single continuous branches and
supported native coil types; it does not claim support for unresolved parallel
branches or arbitrary unitary wrappers. Existing water/plant-backed paths remain
with their original builder.

The remaining three causes are independent-oracle omissions. Annual parallel
driver branches need their actual contributing-month ownership, not the source
union of every parallel branch. A combustion load/fuel quotient above one is
labelled Load / fuel, not Efficiency: the original SQL confirms this in five
Building months and 22 Zone-months. Finally, M9's five independently reported fan
pools sum to 841.0611476728559 kWh, whereas their separately rounded allocations
sum to 841.060 kWh. The allocated proof now sums counted per-pool intervals while
the expected broad meter retains its separate, narrower source interval. This
does not rescale pools, round SQL centers into new expectations, invent missing
observations or authorize an unassigned policy inconsistent with reviewed pools.

## Small Office approved expected manifest

The completed implementation produces
`small-office-25-1-candidate-coverage-02.json` in 3.920 seconds, from the unchanged
original SQL. Candidate SHA-256 is
`a36ba68e1eb21035f81c7c2c301401523213f5cceed2c70b3e557e8cf83ec077`,
with production-source SHA-256
`3f555ee13196e6a5761bb8f25141153d785fc674b72831d767835255293c1fbd`.
Diagnostics `coverage-02`, `coverage-03` and `coverage-04` all pass the full
17,675 checks, with zero numeric/contract failures and zero required-field gaps
across 5,074 records. Their elapsed times are 9.470, 10.910 and 10.500 seconds.
The corrected cooling branches increase the record count; no failing record
or metric group was removed to obtain these passes.

The separately preserved `small-office-25-1-pending-coverage-01.json` has SHA-256
`ce02df1bf85f2c0828df2fa8a002f4f11fd033923a2d8fe2f2234dcbc4eda6a0`.
Independent read-only reviews checked all metric/coverage identities and
provenance, 42 Zone monthly-to-annual family sums, 156 carrier component sums and
91 Core calculations from the original SQL. They also checked Attic zero loads
without invented direct-use sources, actual August driver ownership, seasonal
Load / fuel classification and exact fan pools. Known values include 6,218 exact
zeros; all 653 null values retain explicit partial/unavailable/not-applicable
status. Coverage includes 4,827 primary, 78 referenced-accounting, 26 context and
143 non-flow records. The positive 3.5376923076923375e-8 kWh water-system pump
consumption remains unassigned rather than disappearing or being allocated to
the HVAC Zones.

Root explicitly approves this reviewed independent expectation set in
`expected/small-office-25-1.json`. The opt-in payload generator creates only its
146,896-byte lossless metric companion; it cannot author approval or replace
existing files. Companion SHA-256 is
`af69ece75a9e2b6943f3f095f02ecb506086893fc97badd32f5385eea1efdc03`.
Its uncompressed 17,675-metric array has SHA-256
`59d96c3bc3ab714fd820d0836564832473f10f6572086d41bcf3e63b90126b2f`,
and exact sorted-key digest
`df54a3e29b40095b6023ba4d501760a2034bc9acb6edd11da2001322121bc5e3`.
The separate saved-original-wire acceptance passes in 17.040 seconds, comparing
every approved value, identity, count and status in all eight groups. The normal
offline catalog guard now requires both approved fixtures. The recipe's retained
draft-review text describes its preparation history; explicit approval resides
only in the separate expected header and is limited to this exact fixture.

The new exact monthly driver-owner/source assignment also receives a Large
Office regression check. Its first full replay fails explicitly at the existing
65,536-visit search bound, preserved as
`large-office-25-1-diagnostic-coverage-09.json`. The correction removes only
assignments to owners with no actual link or source leaves: optional zero there
is equivalent to whole-month omission, while mandatory positive months cannot
be omitted. The cap, numeric intervals, source coupling and rejection of
fractional-month splitting remain unchanged. Twelve months with many absent
owners reproduce the original failure and now pass. With current-production
`large-office-25-1-candidate-coverage-03.json` (SHA-256
`e697369616155fec51e25f70ee49d6222737833ebb4c75502a97542ad5a9c216`),
the unchanged Large Office approved acceptance passes all 46,224 metrics in
76.340 seconds. No original engine, input, SQL or approved Large Office
expectation was changed. Ideal Loads is next in section 22; the remaining 17
catalog entries and later checklist sections are not approved by these passes.

## Ideal Loads: group subtotals and annual-only sources

The next original fixture is `5Zone_IdealLoadsAirSystems_ReturnPlenum.idf`,
SHA-256 `1fc3cae54a39c004522db324ab7f172ce123d146e9fd63548393d4921f1a9786`,
capture `real-ideal-loads-25-1-20260907T162941.603417700`, SQL SHA-256
`c43c777904b93b313617f8dcc8e40388bec4b3ac3549e4279d15f13c5019849d`.
Its normal full-year run completed in 4.650 seconds with five retained warnings
and zero Severe/Fatal errors. All six Zones have multiplier one; SPACE1-1 through
SPACE5-1 each owns an Ideal Loads system and PLENUM-1 is their common return
plenum. Actual Zone Air System sensible observations exist for all six Zones,
so that existing canonical authority remains ahead of Ideal Loads alternatives.
PLENUM-1's 27,758.043436912194 kWh heating observation must not be replaced by zero
because it has no conditioning unit of its own.

The first dictionary-only audit incorrectly inferred no HVAC consumption from
missing district ReportData meters. Inspection of the complete original SQL
corrects that inference before implementation: its utility End Uses table
reports annual district cooling 19,149.37 kWh and district water heating
7,129.51 kWh, with corresponding Total End Uses carrier cells. EnergyPlus 25.1
[defines Ideal Loads supply conditioning as district energy consumption](https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/doc/input-output-reference/src/overview/group-zone-forced-air-units.tex#L7),
and [registers those exact resources](https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/PurchasedAirManager.cc#L634).
These modeled annual consumption values are not evidence of physical utility
infrastructure, but are valid annual site-energy fallback under EPATH-050. The
monthly electricity observations total 35,724.5 kWh; annual total site energy is
62,003.38 kWh including district consumption. Source-energy and peak-demand
tables are different physical quantities and cannot substitute for these cells.
All twelve monthly candidate graphs lack district consumption/conversion links;
Annual alone uses the actual tabular source. Neither zero substitution nor
monthly spreading of annual energy is permitted.

The actual candidate also exposed a production error: `Electricity:Building`
was classified as an unknown Other end use and added again to its lighting and
equipment components. Native resource `Building`, `HVAC` and `Plant` subtotals
now retain exact original source values, units, frequencies and observed zeros
as inspector context, without contributing another end use or becoming a
Facility total. The matcher is restricted to native two-part resource/group
names; ordinary carrier-qualified custom end uses remain unchanged. The
new classification and full SQL graph regression fail before the fix and pass
after it. Raw V1 decoding/migration is unchanged; canonical rebuilds receive
the corrected classification.

Preserved candidate `ideal-loads-25-1-candidate-coverage-01.json`, SHA-256
`282f04691facbd516f8b3d14f4cf24a0cfabb2cbca900a97fa2b15aa46d0701c`,
shows the duplicate branch and 42.383% overmapped site closure. New candidate
`ideal-loads-25-1-candidate-coverage-02.json`, rebuilt in 5.690 seconds, SHA-256
`26cc706e4684624d4d1951a3764db3704f82f5b1432420a6d5230932d4067b01`,
has 100% complete site closure while preserving both original Building subtotal
sources (Monthly dictionary 68 and RunPeriod dictionary 70), each 35,724.5 kWh.
The original district values and their annual-only temporal coverage remain
unchanged. Comparing the two preserved candidates also verifies all 604 original
source identities, numeric values, units, frequencies and input-source IDs are
unchanged, as are the complete thermal node payloads in all 91 scope/period
graphs. Current production-source SHA-256 is
`858222b1427254e393edec501491665ae80a01a807538520580c95e6c351f7bc`.

The independently reviewed draft recipe records 16 driver families, six-Zone
sensible loads, five-Zone direct lighting/equipment, three Monthly electricity
meters, four exact Annual Tabular selectors and 40 actual-plan availability
groups. It does not contain copied candidate expectations. A new independent
Tabular reader validates a unique exact utility report/facility/table/row/column,
original cell ID, explicit energy unit, fixed-decimal source precision and full
annual Weather coverage. It retains the reported 0.01 kWh precision as a bounded
half-unit interval, distinguishes actual zero from missing/NULL observations,
and never creates Monthly frames. Focused tests include wrong units/reports,
duplicate cells, invalid calendar and source precision; the actual original SQL
four positive cells and literal-zero steam cell pass with unchanged SQL hash.

The dedicated Annual Tabular integration is now implemented across independent
frames, original-source identity and value proofs, site flows/residuals,
Building/Zone service allocation, Zone carrier subtotals, quality and required
record coverage. Tabular originals use their exact reviewed cell identity, not
invented ReportData dictionary IDs. Monthly-source `Annual = sum(months)` remains
strict; Annual-only cells never create Monthly quantities. Monthly absence is
verified by explicit node/link/allocation absence proofs before a closure check
can exclude it, rather than treating an unknown value as zero. Reported Monthly
electricity remains independently checked alongside absent Monthly district
consumption.

The reviewed recipe also declares 20 original Monthly Ideal Loads Supply Air
Latent Cooling/Heating observations: Energy [J] and Rate [W] for each of the five
actual equipment owners. Their exact equipment-to-Zone ownership and raw/effective
source quantities are independently verified. Existing EPATH-071/092 inspector
contracts retain these sources as non-additive context/Breakdown on selected
load and conversion trace. They are not added to the canonical Zone Air System
sensible load, and cannot substitute for its required numeric source evidence.
Wrong ownership, context metadata and source-wrapper substitutions are rejected.

The failed comparisons remain preserved against candidate coverage 02:
`ideal-loads-25-1-diagnostic-coverage-01.json` checks 16,993 obligations and reports
6,504 failures (2,276 numeric/contract and 4,228 required-field coverage).
After the explicit annual absence and 20 non-additive source integrations,
`ideal-loads-25-1-diagnostic-coverage-02.json` checks 17,073 obligations and reports
1,246 failures (480 numeric/contract and 766 coverage). Their remaining cause was
original Hourly [W] reconciliation dependencies retained inside derived source
input IDs while the independent recipe selected only Monthly numeric evidence.

Twelve explicit alternate temporal observations now bind the six surface and
six internal-convective aggregate owners, including the reported-zero plenum.
An independent SQL calendar audit requires every one of the 8,760 distinct 2017
Weather hours, finite values and exact one-hour intervals. Integrated original
months agree with the selected Monthly surface rate or internal-convective
energy authority (maximum observed discrepancy below 5.2e-12 kWh). These aliases
are provenance only: primary source requirements, cells, allocation quantities
and their precision remain unchanged. Only each original Hourly inspector
source's raw/effective display has a separately counted bound for its nonzero
hourly decimal3 normalization and final presentation stages. Real zeros remain
exact zeros. Wrong family, owner, period, missing hour and alias-only authority
are rejected, as are moving driver provenance to a load and hiding duplicate
physical links behind a different context-source list.

Preserved `ideal-loads-25-1-diagnostic-coverage-03.json` passes all 17,121
numeric/contract checks and required fields across 4,047 coverage records in
15.252 seconds: all eight groups have zero failures and zero coverage gaps.
It uses unchanged candidate coverage 02 and original SQL; recipe SHA-256 is
`d42097b62c9b2a7597152e74e0d201fb6d1dde9f9f1fcaca8b9a1d134daa9a60`.

The strengthened dictionary guard also rejects unobserved duplicate identities,
including case-only and Monthly-counterpart duplicates. Its first negative test
failed before the guard and passes after it. A separate immutability test now
compares the same numeric frame byte-for-byte before and after trace compilation;
two independent numeric compilations can otherwise differ by preexisting
map-order floating-point noise. Final temporal-source tests pass in 18.783
seconds, and all DriverLinks tests including 23 new temporal counterexamples
pass in 24.534 seconds.

Diagnostic 04 repeats the complete 17,121-check pass in 16.396 seconds and
creates a separate pending artifact with SHA-256
`ea2f0324588495f7ac42483d1540c9b25df4b380f06d8d0ad9ea5df72f457be2`.
Independent read-only reviews verify every provenance hash, the 105-file
production digest, 91 scope-period contexts, 5,809 actual zeros, 2,328 explicitly
typed nulls and 455 valid count pairs. Coverage contains 3,904 primary records
and 143 typed non-flow source-correspondence links; its 8,594 required field
slots all resolve to reviewed selectors. No blanket context exemption is added.
The diagnostic 03/04 coverage registries are identical. A separate original-SQL
arithmetic review compares 3,135 numeric/unavailable expectations across all 78
Zone-period contexts, plus the 20 latent original sources, with maximum difference
1.46e-11 kWh. It confirms annual-only district ownership, plenum load preservation
and electricity subtotal exclusion.

Root explicitly approves only this reviewed independent fixture in
`expected/ideal-loads-25-1.json`. Its lossless 131,365-byte companion has SHA-256
`db321d182089809a58f98b4363b09a56a6e3bed2af2bb5a383efe7f8cb2c93f8`,
uncompressed metric SHA-256
`a1a85f512b31aa933d03cdcd293d2ffff20ddfaf156026568f9ec69218514342`,
and full sorted-key SHA-256
`151ab8198c9420d2f8f75f8a69e96d55611363a1edbf7dadb1237860f7a47e5d`.
Separate saved-original-wire acceptance passes all 17,121 values, identities,
counts and statuses in 28.405 seconds. The offline integrity guard now requires
all three approved fixtures. Final unchanged-expectation Small Office and Large
Office regressions pass all 17,675 / 46,224 metrics in 18.255 / 77.710 seconds.
Original files, candidates and failed diagnostics
remain intact. This is the third of 19 approvals, not completion of section 22
or later checklist sections. Normal full verification and Wails build remain
the required commit gate.

Final normal repository verification passes without exclusions: app 17.388
seconds, frontend/browser checks 148.242 seconds and simulation 164.133 seconds.
The Windows production executable builds successfully in 6.333 seconds. PTAC is
the next unapproved fixture in catalog order; no later fixture or checklist
section is marked accepted by this checkpoint.

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
