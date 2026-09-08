# Fan Coil: source review and acceptance

Status: **approved**, with separate saved acceptance passing all 8,966 independent
metrics and required fields. The catalog is now 6/19, not complete. This document
preserves the failed attempts leading to approval. The previous PTHP checkpoint
was committed and pushed as `efe5540`; this checkpoint's full verification/build
and normal commit/push gates follow the fixture acceptance below.

## Immutable evidence

The original `FanCoilAutoSize.idf` has SHA256
`d940e67912d5d4669773c11521b499ca671dae2d67ff8ce538c4a183ca0b43d3`.
Its closed capture is
`.runtime/energy-path-acceptance/25.1/fan-coil-25-1/real-fan-coil-25-1-20260907T163224.387644700`.

| Artifact | SHA256 |
| --- | --- |
| Annual input | `96e52a527da6b359ab6b6499f3fb61a3da0077612e590ca74ba3b366784c4c2b` |
| Executed input | `92826e6a0d1ab3961f485d0088f3426fdc45b409eef879fb4b5d0c4e315187b1` |
| SQL | `c0004ad8c0310942375b1a2ad601ba6eee5b44ad5774fee2b3e5dbc4e080bb91` |
| Run evidence | `0e44c76527c4e63f227c7bb4042a1a098127306ecf191dac1138fa4b4ca8f18f` |

EnergyPlus completed in 7.73 seconds with seven native warnings and no Severe or
Fatal error. The stored error parser records eight issues because it separately
retains the unavailable-variable header. Neither count is suppressed. This
checkpoint reuses the unchanged SQL; topology/result rebuilding is not another
engine run.

All 167 Monthly dictionary identities (155 variables and twelve meters) have
twelve finite non-NULL observations. All 36 Hourly identities have 8,760 valid
observations. Calendar endpoints, durations and cumulative days describe the
complete 2017 Weather environment. Five RunPeriod energy meters retain their
annual provenance despite sharing the terminal Monthly Time row.

## Independently reviewed physical membership

Three Zones each own one FourPipeFanCoil, one local ConstantVolume fan, one water
cooling coil, one water heating coil and a local outdoor-air mixer. There is no
AirLoop. The Chilled Water Loop serves each cooling coil; the Hot Water Loop
serves each heating coil. Purchased Cooling and Purchased Heating are the
respective plant sources. A coil's two legitimate typed references are its own
terminal and its actual plant Branch, not two Zone owners.

The original service builder incorrectly formed the Cartesian product of each
terminal's combined services and plant loops. A regression first reproduced
CHW-to-heating, HW-to-cooling and plant-to-ventilation paths. The bounded native
Fan Coil binding now checks typed coil ownership, exact water nodes and unique
demand-side Branch/PlantLoop membership. Foreign conditioning/source metadata
cannot win earlier same-ID deduplication. Actual shared heating/cooling plants
remain mixed; ambiguous, detached and unresolved connections remain ineligible.
Local electric heating retains its own coil without borrowing the cooling plant.

The original three local fans have no individually requested energy or qualifying
measured pool. Six original inlet/outlet Hourly mass-flow series do exist, but do
not isolate fan energy. Broad Fans remain unassigned. Both pumps resolve to their
separate single-service plants and all three Zones; broad Pumps use the existing
explicit cooling-plus-heating `plant_loop_load_share` estimate. This is not a
measured individual pump or Zone allocation. No automatic fan/pump/airflow output
requests were added and genuine ambiguity cannot be hidden by scope filtering.

All 21 heat-transfer surfaces retain signed category/Zone/month aggregation before
directional splitting: eight exterior walls, three roofs, one window, six paired
interzone walls and three self-referencing floors. The self boundaries are
effectively adiabatic and map to Other/storage, not ground or interzone exchange.
Lighting is reported only for EAST and NORTH; WEST has no invented zero-valued
lighting observation. People and equipment have all three original Zone owners.

## Original SQL arithmetic

Annual values below are independently summed from original SQL, not copied from
candidate nodes or approved as an expected manifest.

| Quantity | kWh |
| --- | ---: |
| Sensible cooling load | 14427.656269580733 |
| Sensible heating load | 12599.419172334354 |
| Cooling / DistrictCooling | 13244.775268173524 |
| Heating / DistrictHeatingWater | 17952.516746306806 |
| Electricity facility | 27415.150272317926 |
| Interior equipment electricity | 19036.880199999538 |
| Interior lights electricity | 7178.9526128 |
| Fans electricity | 1075.463544791437 |
| Pumps electricity | 123.853914727300 |

District end uses equal their matching facility meters in every month. The four
electric end uses close Electricity:Facility within 9.78e-11 kWh per month.
Electricity:Building/HVAC/Plant are overlapping context totals, not additional
consumption. Both thermal-to-purchased-energy ratios are Load / purchased energy,
not COP or combustion efficiency. Water-coil thermal transfer is not direct
purchased consumption. Missing latent loads and cooling/heating electricity are
not zero observations.

A second read-only SQL calculation independently derives all monthly district and
pump shares and then sums twelve completed months. It inspects twenty exact
Monthly/J identities (240 finite observations, 26 actual zeros, no missing/NULL
values), without reading candidate values or the source recipe.

| Zone | Allocated pumps | District cooling | District heating | Total Zone electricity |
| --- | ---: | ---: | ---: | ---: |
| WEST | 39.794439372148 | 4593.235726114984 | 3842.047141927870 | 7654.547039372117 |
| EAST | 36.161192844655 | 3904.011525681980 | 5097.048171424077 | 8330.381192844656 |
| NORTH | 47.898282510498 | 4747.528016376560 | 9013.421432954858 | 10354.758495310500 |

These are original-SQL arithmetic values in kWh, not rounded display values or an
approval. Zone electricity includes only reported direct lighting/equipment and
allocated pumps; the 1075.463544791437 kWh broad fan meter remains outside all
Zone totals. NORTH in August has 0.0329492183053412 kWh real sensible heating load
and observed-zero purchased district heating, not missing consumption. Its pump
share is 7.044336938021751 kWh and Zone electricity is 917.446558338022 kWh.

The draft declares eleven pressure families and nine numerical site identities.
A separate source review matched all 73 selected Monthly identities and all 42
availability groups: 150 distinct requested names occur in the actual plan and
49 observed name/unit pairs match SQL. Availability is Drivers 11/17, Loads 9/14,
End uses 6/8 and Carriers 3/3. Missing infiltration/ventilation/mixing cannot be
subtracted from actual outdoor-air zeros as though they were observed.

## Newly exposed zero-pressure boundary

The first current-code candidate materialized successfully in 1.69 seconds at
`oracle-cache/fan-coil-25-1-candidate-20260908-01.json`. The subsequent independent
compiler **failed**, before writing its diagnostic report or any pending expected
artifact: `north zone/8/heating: positive delivered load has no observed pressure
denominator; explicit unassigned proof required`. This is not a passing diagnostic
or acceptance result, and candidate 01 is preserved.

Original Monthly Heating Energy, dictionary 626, is 118617.1858992283 J
(0.03294921830534119 kWh). Independent Hourly Heating Rate, dictionary 688, has
13 positive observations among 744 August hours and integrates to the same value.
The delivered load is real, not zero or missing. Yet all signed monthly pressure
families and surface-category nets are nonnegative: there is no matching heating
pressure. This condition occurs only for NORTH ZONE in August in the reviewed
three-Zone, twelve-month dataset.

In particular, total internal sensible gain minus People, Lighting and Equipment
is approximately +3.98e-13 kWh in the original SQL. Per-source three-decimal
rounding turns that remainder into -0.001 kWh in candidate 01, which consequently
assigns the full displayed 0.033 kWh heating load to that derived pressure. Its
trace names internal aggregate 15, People 27, Lights 193 and Equipment 341, plus
load 626. This is distinct from the specified zero-pressure Other/storage fallback,
whose physical raw/effective pressure is zero and whose source is the actual load
alone. The compiler correctly rejected the unproved denominator.

Work now separates original calculation precision from presentation rounding and
adds an explicit original-SQL zero-pressure proof. Global allocation tolerances,
missing/NULL semantics and the five previous approved expectations must not be
weakened. Eight-group numeric/field diagnostics, explicit review, saved acceptance
and full repository verification remain required before approval.

## Focused regression evidence so far

- Original hydronic regression: actual RED before the topology change (1.091 s).
- Final hydronic plus existing HVAC ServiceModel/AirLoop checks: PASS (2.323 s).
- Four Fan Coil auxiliary/output-plan regressions: PASS (0.954 s).
- Candidate 01 materialization: PASS (1.69 s), not acceptance.
- Candidate 01 independent frame compilation: FAIL (3.01 s), as described above.

The small original-value precision regression first failed in 0.664 seconds; a
strengthened version failed in 0.641 seconds, explicitly exposing the nonzero
physical pressure and incorrect internal-source trace. After the canonical-only
Monthly calculation shadow was introduced, it passes in 0.651 seconds. Both Zone
and Building, August and Annual retain the displayed 0.033 kWh load and equal
link endpoints, with exact zero physical raw/effective/signed pressure and only
the original heating-load source. General SQL rounding, direct consumption and
original measured-source quantities are unchanged. The full seven-test precision
suite (nineteen subtests) passes in 2.076 seconds. It includes missing/NULL and
nonfinite rows, selected surface membership, unavailable alternative dictionaries,
Monthly alias selection, genuine sub-display-precision signals, and a multiplier
of ten applied exactly once. A separately metered site quantity remains model-total.

The explicit zero-pressure compiler proof also passes all six new tests together
with existing driver-link and reconciliation tests (38.040 seconds). It rechecks
the complete original physical roster before allowing a load-only synthetic
fallback, and rejects even 1e-9 contamination of a pure fallback's exact-zero
physical pressure. Mixed annual/Building contributors retain their own values;
generic allocation tolerances and sharing rules are unchanged.

Candidate 02 materialized from the same saved SQL with the precision fix in
1.66 seconds. Its next independent compilation failed in 2.99 seconds because
allocated `pumps.electricity` had no independent Zone carrier proof. This is a
test-coverage gap, not a passing diagnostic: no diagnostic 02 report, pending
expected, or acceptance artifact was emitted. Both candidates remain preserved.
New test-only proofs now independently derive monthly pump shares, sum completed
months for Annual, and separate the broad consumption-meter trace from the
own-Zone load-weight trace. These additions must pass their focused regressions
and the complete saved-result comparison before any expected approval.

The nine Auxiliary Zone/Flow focused tests pass in 0.645 seconds. Diagnostic 03
then evaluates all 8,966 required SQL metrics and preserves a failed report:
371 numeric/contract failures and 420 coverage failures all arise in the August
Building/NORTH contexts where the new fallback node lacks its proper period
identity. This exposes a separate fallback metadata defect; scalar-only focused
tests were insufficient. The graph context checks are retained, and a regression
must first reproduce the actual Monthly wire identity before production is fixed.
No expected artifact was emitted or approved.

The strengthened native regression first reproduces the missing M8 Period in
0.601 seconds. Production now passes the existing load period into the fallback
constructor and preserves it on the node; no numerical or source field changes.
The full precision suite passes in 2.105 seconds, including exact native
Building/Zone and M8/Annual graph validation. Building links retain a unique
contributing Zone where the existing contract requires it, and the deliberately
sparse unit fixture retains only its actual Annual/M8 wrappers.

Candidate 03 materialization passes in 1.64 seconds. Diagnostic 04 then passes
all 8,966 metrics and mandatory record fields with zero failures in every group
(4.65 seconds): Drivers 2,666; Loads 2,180; End uses 402; Carriers 279; Ratios 260;
Completeness 312; Residuals 1,833; Zone allocation 1,034. It creates only the
unapproved `fan-coil-25-1-pending-20260908-01.json` review artifact. Recipe SHA256
is `b7e5c047c5b91f798d1d1a79235de37d9c50237446a2a92039dca6ca12625bbc`,
candidate SHA256 `17a164b0e963cae9ed76b935fafff6ca1fac9633fcacf62f064f34fe8ba23d23`,
and production SHA256 `b4b9f531b3b7428992d0c1ca62b507c3a64f6cc3cde0fc1cd59862dbdc8f318b`.
The independent pending review and separate saved expected acceptance remain
required; this passing diagnostic is not approval.

All five prior approvals pass current-code saved rebuilding with their original
headers and metric companions unchanged: PTHP 25.215 seconds (17,113 metrics),
PTAC 23.331 (17,021), Small Office 18.877 (17,675), Ideal Loads 30.675 (17,121),
and Large Office 82.207 (46,224). Each verifies all eight groups. The original
captures are validated before and after and are not rewritten.

The independent pending arithmetic review matches 1,934 exact keys (1,678
numeric and 256 unavailable) against original SQL, with no discrepancy. It also
checks 124 annual/twelve-month sums, whose maximum floating-operation-order
difference is 3.64e-12 kWh. This is a separate cross-check, not a claim to manually
reproduce every one of the 8,966 compiled obligations. WEST's 52 zero-contribution
obligations and absent lighting nodes remain absence, not reported-zero lighting.
Original NORTH August pressure families are all nonnegative, so matching heating
pressure is exactly zero; its real candidate node and link both retain only
`sql-rdd-626` and Period M8. The actual Building storage aggregate is mixed with
other Zones' physical contributions and is not claimed to have zero pressure.

Paired annual ratios follow the existing completed monthly conversion branches,
not an unrelated whole-year quotient when some months have load but zero site
consumption. The independent review checks 104 such paired ratios. Building
annual cooling is 0.9304204699066528 and heating 0.7017976585151678; both remain
Load / purchased energy, not efficiency or COP. Full annual source-load totals
above are still retained and are not replaced by this paired subset.

## Explicit approval and saved acceptance

Root explicitly reviews and approves pending artifact SHA256
`6324f440b8b608cd1d4c32fb2a5e7eff0352d82c1848cb083ece6db2f611e55d`.
Separate registry/provenance review confirms all 8,966 sorted unique keys,
3,087 known zeros, 373 explicit nulls and 260 valid count pairs. All 2,728 unique
coverage records span 52 contexts: 2,572 primary, 78 non-flow and 78 referenced
accounting records, with 5,851 required fields and 6,709 exact selector bindings.
No candidate record or required selector is missing; nested unavailable latent
details and contractually omitted allocation zeros retain their original meaning.
Original capture, input, SQL, engine, weather, recipe and current production
fingerprints all match independently.

The hand-authored approval header and generated 74,133-byte lossless companion
retain every metric. Companion SHA256 is
`51cc4427f0fd55edca4c7a62a4112e14f0be6dcb9b2edfba13a5eb1bb2a621db`;
uncompressed metric SHA256 is
`fa0bb3471cd639b48fa85617193d73d8badfbd25744d1f67e0978e208ef38c5e`.
Separate saved-original-wire acceptance passes all 8,966 metrics in 8.07 seconds;
the same command's six-fixture offline guard also passes (package 9.874 seconds).
This is fixture result-transformation approval, not design/climate certification
or completion of the other thirteen fixtures and checklist sections 23–26.

The full `scripts/verify.ps1` gate passes without exclusions: app 21.187 seconds,
CLI 5.765, input 1.580, frontend/browser 167.646, IDF 7.098, simulation 307.114,
and tabular successful from cache. The Windows production Wails build succeeds
in 7.706 seconds and replaces the closed application's executable. Normal
commit-hook verification and push follow this full gate; no test is skipped to
make the checkpoint pass.

A separate original-SQL risk review covers 960 internal-remainder cells across
the five prior approved fixtures. All 480 latent cells are exactly zero; the
largest absolute sensible remainder is 6.912e-10 kWh. Source-by-source rounding
could produce false sensible remainders in 138 cells, including nineteen Large
Office multiplier-ten cells at 0.010 kWh and the others at 0.001 kWh. These are
independent arithmetic projections, not current-candidate differences or new
expected values. The five existing approvals must be replayed unchanged.
