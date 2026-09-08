# Radiant Energy Path acceptance — approved

This is checklist section 22's eighth approved fixture. The seven prior
approvals are unchanged. Its separately reviewed original-IDF/SQL expectations
and saved acceptance pass; full repository/build and commit/push checkpoint
work is recorded below. This is not completion of the remaining eleven catalog
fixtures or checklist sections 23–26. Earlier failed diagnostics remain history.

## Preserved original evidence

The catalog original is EnergyPlus 25.1 `RadLoTempCFloHeatCool.idf`, SHA256
`c4988a4211b37a9508dd0265ee19cb1f4f7174c57e214a38ec3935e2f918388e`.
The preserved capture is:

`.runtime/energy-path-acceptance/25.1/radiant-25-1/real-radiant-25-1-20260907T163418.463444400`

Its native engine completes in 16.73 seconds with 56,132 retained warnings and
zero Severe errors. The SQL is 93,536,256 bytes, SHA256
`72ebff75d6a49f5a543df4fd3c951e009cd5a41b9215d035846e2cb03dcc2a82`.
Original, annualized and executed input fingerprints agree with captured
provenance. The annual copy has twelve 2017 Monthly periods and 52,560 ten-minute
periods; the original design-days-only controls are not annual evidence.

The original capture has no saved oracle-observations JSON. A read-only
independent SQL replay therefore collected 258 source identities in 33.429
seconds, without a new engine run or production candidate. Its new observation
artifact is `.runtime/energy-path-acceptance/oracle-cache/radiant-25-1-original-observations-01.json`.
Its SHA256 is `be43db16feb7565f3bada2a008711dffe2cb2a33258ae452054345ec56143217`.
Collection is not acceptance, and dictionary presence alone is not proof of a
known quantity or a complete twelve-month series.

## Original physical boundaries

WEST, EAST and NORTH each have multiplier one and one native
`ZoneHVAC:LowTemperatureRadiant:ConstantFlow` floor system. Exact original
EquipmentList references, explicit Zone references and the floor surface owners
must agree. Each parent equipment has separate heating and cooling water ports
connected to different plant demand branches. There are no terminal Coil objects,
AirLoops or Fan objects; fan-coil coil-reference ownership and VRF outdoor/terminal
ownership are not this model's contract.

Cooling uses both electric chillers and purchased district cooling, with
different on-peak/off-peak operation lists. Heating uses purchased district hot
water. A separate condenser loop and cooling tower are present. Neither an
electric-only cooling assumption nor an all-district assumption is justified.

Three central pumps and three embedded radiant pumps have distinct physical
roles. A broad Pumps meter cannot automatically reuse the earlier fan-coil
cooling-plus-heating load-share rule. Pump electricity and pump heat added to
fluid are not the same quantity; tower heat-rejection energy is not proven zero
by the absence of Fan objects.

The geometry has twenty opaque surfaces and one window: eight outside walls,
three roofs, three ground floors and six interzone faces. All three radiant
floors contain an internal heat source. The active heat delivery, surface-to-air
exchange and storage boundaries require an explicit double-counting review.
Lighting is present only in EAST and NORTH; WEST is absent, not an observed-zero
lighting source. People, equipment and infiltration have three original owners.

Some original header comments conflict with the actual ground, chiller and tower
objects. Typed objects and original numerical observations are authoritative;
those comments must not be copied into ownership or expected-value declarations.

## Observed request-key defect and load-selection boundary

The saved Basic Energy plan requests Monthly radiant heating/cooling outputs
using Zone names. Its resolution falls back, while the actual original SQL
radiant outputs use the equipment names `WEST ZONE RADIANT FLOOR`,
`EAST ZONE RADIANT FLOOR` and `NORTH ZONE RADIANT FLOOR` at Zone Timestep frequency.
Original wildcard Timestep requests must remain intact. Correct additional
Monthly requests need exact equipment keys and independently validated Zone
ownership, not name guessing or reuse of a selected Zone as the SQL key.

The six Monthly air-system sensible heating/cooling identities have twelve
finite rows each, with all 72 observations actually zero. The six original
radiant equipment heating/cooling Timestep identities instead have positive
monthly energy. Existing canonical selection considers material families before
authority preference, so an all-zero air-system family does not necessarily
exclude a material direct-equipment fallback. This is not authorization to sum
both families or redefine radiant equipment energy without checking its physical
meaning and surface-storage boundary.

## Primary-source semantics and retained measurement boundaries

EnergyPlus 25.1 defines Zone Air System sensible energy as excluding high/low
temperature radiant systems. These air-system quantities do not include Zone
or ZoneList multipliers. See the [official output definitions](https://bigladdersoftware.com/epx/docs/25-1/input-output-reference/group-thermal-zone-description-geometry.html#zone-air-system-sensible-heating-energy-j).

ConstantFlow radiant heating/cooling instead reports the sum of active surface
source/sink terms after multiplying by Zone and ZoneList factors. Its separately
computed Zone-air response is not this reported fluid/surface energy. Monthly
storage and other surface exchanges therefore prevent treating the latter as
same-month air delivery or as a measured equipment COP. See [EnergyPlus 25.1
implementation](https://github.com/NREL/EnergyPlus/blob/v25.1.0/src/EnergyPlus/LowTempRadiantSystem.cc#L5821-L5873).

The implementation now explicitly binds this surface-source observation only
to independently validated native owners and marks its thermal boundary. It
does not add a suppressed radiant observation to an air-system load. A mixed
system selecting the air observation needs an explicit warning that combined
HVAC delivery has not been established. Active-floor inside-face convection
must remain traceable context rather than a second independent passive driver.

## Original meter composition audit

The preserved MTD SHA256 is
`6046eefa4e329b521265247839c4e3f483e80088f54e4833f8d74d1e7e04fb63`.
Its lines 590–596 identify all six Pumps electricity constituents: three
embedded radiant pump outputs and CIRC PUMP, COND CIRC PUMP and HW CIRC PUMP.
Lines 642–644 identify Cooling electricity as LITTLE CHILLER and BIG CHILLER.
Lines 668–669 identify HeatRejection electricity as BIG TOWER, the additional
leaf in Facility's complete membership at lines 306–320.

Original observed Pumps electricity is 6647.0931270067 kWh. Embedded pump
electricity sums to 738.6610945765 kWh; their fluid heat gain instead sums to
652.2377465115 kWh. The latter matches the original motor/fraction settings
(0.883 times electricity, maximum monthly difference 1.46e-12 kWh) and is not
an additional electricity-meter constituent.

The central pump sum of 5908.4320324301 kWh and tower-related Facility remainder
of 3437.552850741 kWh are derivable differences, not separately observed SQL
component quantities. The preserved SQL contains no individual central pump,
chiller or tower output dictionary. No direct source, per-pump allocation or
known-zero value may be invented from those absences.

## Original service and auxiliary path audit

The original IDF independently connects all three radiant cooling ports through
`Zone 1/2/3 Cooling Branch` to `Chilled Water Loop` (lines 1054, 1094,
1130–1152 and 1254–1269). Its supply contains both electric chillers and
`Purchased Cooling`; the actual on/off-peak equipment lists include both
technologies (lines 1201–1223 and 1270–1319). All three heating ports similarly
reach `Hot Water Loop` through `Zone 1/2/3 Radiant Branch` and the Reheat
splitter/mixer; its heating operation names only `Purchased Heating`
(lines 1643, 1730, 1772–1794, 1809–1823 and 1837–1852).
Cooling and heating therefore each have the exact eligible owners `West Zone`,
`EAST ZONE` and `NORTH ZONE`. Service-specific Monthly load shares are modeled
`service_path_allocation`, not measured individual-Zone consumption.

`Big Tower` is on the condenser-loop supply; both chillers' exact condenser
ports appear on its demand branches (lines 1398, 1450–1456 and 1512–1526).
The year-round cooling operation lists only that tower (lines 1580–1595).
This proves a cooling-only shared path to the same three Zones. A distinct
HeatRejection electricity auxiliary with `cooling` weights and
`condenser_loop_load_share` is justified as a modeled allocation. It is neither
heating nor air-fan consumption, and does not claim that purchased district
cooling itself uses this tower or establish a measured Zone-level tower split.

The broad Pumps meter instead combines CHW, HW and condenser central pumps
with three separately owned embedded pumps (IDF lines 1382, 1627, 1854 and
1930–2046; original MTD lines 590–596). The union of downstream Zones does not
prove one complete cooling-plus-heating denominator across these different
scopes. Keep this mixed pool unassigned with no eligible-Zone allocation until
constituent energy and service partitions are independently established; retain
observed embedded-pump traces without fabricating central-pump observations.
Pump fluid heat gain remains outside the purchased-electricity sum.

## Request/owner regression checkpoint

The actual original request-key test first failed in 0.656 seconds: none of the
twelve expected equipment-keyed Monthly requests existed. After the bounded
native hook, `^TestEnergyPathRadiant` passed in 1.007 seconds (six top-level
tests plus 29 named subcases). This checks all three original owners, four exact
J/W aliases each, 25 ownership contradictions, selected/unselected Zones,
original wildcard Timestep retention, and unchanged non-radiant Zone requests.
This is a focused request checkpoint, not numerical acceptance or a new engine
capture. The subsequent measurement-boundary integration still requires its
own focused and end-to-end verification.

## Native capture after measurement-boundary corrections

The integrated production regression completed successfully in 3.404 seconds:
original requests/owners, Zone 7 × Group 3 already-total behavior, exact J/W SQL
identities, literal zero versus NULL/missing/duplicate/nonfinite observations,
active/passive surface separation, immutable multi-source load aggregation,
thermal-boundary propagation and existing EPATH051 multiplier contracts.
The regular frontend inspector test passed in 2.030 seconds. The cooling-tower
feature regression first failed in 0.927 seconds and then passed alongside
existing output recommendation tests in 0.935 seconds. The generic `cooling`
match had hidden the more specific CoolingTower heat-rejection feature; the
correction preserves existing cooling-output eligibility and adds its missing
meter recommendation.

Intermediate combined runs (1.510 and 3.353 seconds) are not passing evidence.
They exposed malformed hand fixture identity/period assumptions, an actual
input-map mutation in multi-source load combination, and the loss of a native
dictionary identity when every ReportData observation was absent. The final
passing run includes the corrections and counterexamples, not relaxed physical
or acceptance tolerances.

The new normal capture is
`.runtime/energy-path-acceptance/25.1/radiant-25-1/real-radiant-25-1-20260908T152059.721318500`.
Collection passed in 74.804 seconds (engine 15.71 seconds, 56,132 warnings,
zero Severe/Fatal errors), including the complete purpose-result build.
It is **capture only, not acceptance**.

- SQL: 93,548,544 bytes, SHA256
  `9a36262ab875cd690523fca0835f6b8c582b844167fc18f1692658aac038441e`.
- Original input SHA256 remains
  `c4988a4211b37a9508dd0265ee19cb1f4f7174c57e214a38ec3935e2f918388e`.
- Annual input SHA256:
  `0e08e81a887f57986c5b4ea8ef5e520674133647a3c501fdef792a9c82382a8d`.
- Executed input SHA256:
  `7c388478fd0970e368d635e2a1e0b0724edc0703c02212077598e749d16c6724`.
- New independent `oracle-observations.json` SHA256:
  `850ee3264660f23815d29ad2eee8a2877db95553ccf892d39857628f93c10b56`.

A separate read-only SQL comparison matched all 258 prior dictionary identities
and **4,574,728 original observations exactly**, including 2,004 Monthly,
4,572,720 Zone Timestep and four Run Period rows. Original time rows are unchanged.
The new SQL adds exactly thirteen Monthly identities and 156 finite positive
rows: twelve radiant J/W outputs and HeatRejection electricity. It does not
replace any original observation. The saved comparison script is
`.runtime/energy-path-acceptance/oracle-cache/radiant-original-sql-diff-20260909-01.py`.

The newly measured HeatRejection electricity is 3437.5528507403956 kWh, consistent
with the prior complete-MTD-derived tower remainder. It is now a genuine meter
observation, unlike the preserved old capture's derived difference. There are
271 new-capture source identities in total, including 180 Monthly identities.

## Independent source recipe and typed proof checkpoint

The combined native source/owner, active-surface context, load-node/Zone-service
and production multiplier/boundary focused tests passed in 16.392 seconds.
The preceding 16.263-second run failed only the duplicate-month hand test: the
independent SQL reader rejected dictionary 15 / TimeIndex 2 before frame
compilation. The corrected test accepts that exact duplicate-reader error,
not other reader failures, and still rejects any accepted duplicate observation.

`oracles/radiant-25-1.json` is a new reviewed source/model declaration, **not an
approved expected manifest**. It currently declares fifteen driver families, two native
combined loads, ten site meters, forty-two executed-plan availability groups,
two service paths, direct lighting/equipment and separate auxiliary policies.
Only the prior alias taxonomy was reused; requested names were intersected with
this original executed Monthly plan and observed alternatives were recomputed
from this SQL. No candidate quantities or another fixture's expected values
were copied.

A separate original-only audit verified the driver formula roster against
44 identities / 528 monthly observations: no missing values and 87 actual
zeroes. The internal sensible subtraction residual spans roughly
−2.217e−12 to +1.648e−12 kWh; the outdoor-minus-infiltration residual has magnitude
at most 3.411e−13 kWh. These are derived floating-point residuals, not observations
of exact zero or additional physical sources. Equipment latent and interzone
air each have 36 actual zero monthly observations. West lighting is absent,
not zero. Electricity:Building and Electricity:Plant remain overlapping
context and are not added to facility or end-use totals.

The complete focused native proof family, new quality/driver counterexamples,
cooling-only HeatRejection and existing auxiliary Zone/flow tests now pass in
17.353 seconds. HeatRejection's first hand test failed in 0.797 seconds because
its annual wrapper omitted `Kind: annual`; the fixture now supplies exact
monthly/annual kinds and includes a missing-kind counterexample. The actual
reconciliation evaluator and all 429 hand obligations were unchanged.

The new SHA-bound candidate was materialized in 11.904 seconds, without changing
the saved capture:
`.runtime/energy-path-acceptance/oracle-cache/radiant-25-1-native-candidate-01.json`.
Candidate SHA256 is
`76e1f4c25ed2f868022efcd30d0e3fc8a83466dc5dad2f23616f2db4fc638c8b`;
production SHA256 is
`53236ad8e8e1e4153dad93e81829ca3b6385eae8aa2d0e54eaf401e63d863c83`.
This is **candidate only, not expected evidence or acceptance**.

The first diagnostic failed before evaluation in 38.958 seconds: thermal
reconciliation incorrectly required a numeric cell for the explicitly retained
active-Zone aggregate context. Exact typed original context can now discharge
that cell-presence obligation; arbitrary missing context, changed sources,
pressure reuse and fabricated cells are still rejected. New and prior
reconciliation tests pass together in 8.059 seconds. The second diagnostic
failed before evaluation in 35.150 seconds because the existing multi-carrier
ledger compiler requires direct equipment consumption; this model instead has
two independently observed broad cooling pools. The bounded shared-pool proof
is in progress. Neither diagnostic produced a numeric comparison report.

A separate original request/navigation audit confirms all six selected native
Monthly J sources have the correct component and Zone references. Their nil
original-input ObjectIndex agrees with the saved temporary requests. EPATH131
matches exact type/name/equipment-key/frequency and opens that request's Output
detail row; it does not confuse the executed output indices or original
wildcard Timestep indices with an editable original-input object.

## Service-path and full diagnostic checkpoint

Original native equipment has two service paths per Zone: cooling to the chilled
water plant and heating to the hot-water plant. Generic type-based inference
had added heating on the chilled-water path and a plant-free ventilation path,
giving five paths per Zone. The false heating path propagated through the real
chiller/condenser connection and caused the cooling-only auxiliary guard to
reject HeatRejection allocation correctly.

The bounded native binder now checks original EquipmentList/Zone/Design/direct
Surface ownership and exact heating/cooling ports, Branch membership and plant
demand membership before legacy ID deduplication. It preserves genuine mixed
heating/cooling plants and non-native behavior; ambiguous or detached native
connections fail closed. The auxiliary guard is unchanged. Original path
regression first failed in 0.958 seconds, then all native plus prior hydronic
path tests passed (IDF 1.627 seconds, simulation 0.392 seconds). Synthetic
unequal cooling/heating weights verify tower 50 splits as 30/15/5 by cooling
only, while the mixed Pumps pool and heating-only tower case remain unassigned.

Candidate 01 is preserved but stale after this production correction. Current
candidate 02 was materialized in 11.166 seconds from the same capture:
`.runtime/energy-path-acceptance/oracle-cache/radiant-25-1-native-candidate-02.json`,
5,324,793 bytes, SHA256
`ec4008be6ff04f0f82014629382754d8435f41cc7d628f34aad99bfc52555f0c`;
production SHA256
`94439647ed4f03e9443166ff3dd3981a57e190faccf1ea3fab7c62a91345179c`.
The exact native two-broad-carrier proof is implemented, including monthly-first
allocation, known-zero district observations, original owner/source/method
identity, and unassigned pools when cooling is zero. Its focused field/mutation
and coverage tests pass in 0.706 seconds.

Diagnostic 03 failed before comparison in 34.796 seconds on original owner case:
the IDF spells `West Zone`, unlike its uppercase SQL key. Only the two declared
native owner names were corrected. Diagnostics 01–03 produced no JSON report.
Diagnostic 04 reached all eight comparison groups in 35.878 seconds and retained
6,864 failures (2,849 numerical/contract plus 4,015 coverage). Two audit defects
accounted for most failures, without a change to original quantities:

- The native J source checker incorrectly demanded `sum`; the existing canonical
  contract and older direct-equipment checker require `sum_report_data`.
  Hand fixtures and rejection cases now enforce that existing contract while
  preserving ownership, multiplier, value and unknown checks.
- The first recipe netted infiltration gain and loss before directional pressure
  allocation. EPATH-062 requires four separate sensible/latent gain/loss sources,
  as the approved Large Office recipe already does. WEST January's original
  latent gain 0.7283171751004865 kWh and loss 32.83495340866138 kWh must both remain.
  Only outdoor-air reconciliation uses signed sensible gain minus loss. Its
  tiny derived residual remains `balance.storage_other`, not an invented
  mechanical system or an observed zero. The corrected fifteen-family recipe
  SHA256 is `e719f1c1bd68de7dd3b8c3ea763218f1d89c659d33d2cb5feb85e398b982e696`.

The combined native/HeatRejection/previous auxiliary focused tests pass in
17.405 seconds. Diagnostic 05 then compared 9,615 checks in 36.526 seconds:
24 numerical/contract and 48 coverage failures remain. End uses, carriers,
ratios and Zone allocation pass both numerical and coverage checks. Remaining
failures concern active-floor context source metadata and an EAST April
mechanical-ventilation branch without independent physical evidence, plus their
annual/building and downstream completeness/residual effects. Reports 04 and 05
are preserved, not accepted as expected values.

The final twelve active-floor metadata failures came from index namespaces:
original surface indices 59/66/73 become 58/65/72 in the annualized geometry.
The exact original type/name stable surface IDs are unchanged and already
present in the candidate. The oracle now independently constructs and requires
those stable identities, while retaining original object ownership, SQL key,
multiplier and raw/effective proofs. Shifted-index positives and index-only,
foreign-type/name, duplicate and ambiguous identity negatives are included.

The EAST April fallback instead exposed a production precision omission:
outdoor −116.97529128601789 kWh minus signed infiltration
(1.3422223203915369 − 118.31751360640911 kWh) nearly cancels. Subtracting each
already-rounded three-decimal source manufactured +0.001 kWh. The fallback now
uses the existing original Monthly precision evidence before its unchanged
materiality check, as reconciliation already does. No topology/classification
rule or tolerance is weakened, actual tiny values remain in original evidence,
and incomplete precision evidence is not replaced with rounded data or zero.

The original literal regression first failed in 0.564 seconds. A first combined
post-fix run failed in 22.705 seconds only because the new negative-residual
hand test assumed an order for three provenance IDs collected from a map. Its
assertion now requires the exact unique unordered set (missing/extra/duplicate
still fail); formula and scalar checks are unchanged. The complete outdoor,
Monthly-precision, prior EPATH-063, production Radiant and independent Radiant/
HeatRejection/previous auxiliary focus then passes in 23.500 seconds. A new
current-production candidate and full diagnostic are still required; candidate
02 is now stale and remains preserved.

## Complete independent diagnostic 06 — passed, not yet approved

Candidate 03 materialization passes in 11.352 seconds. The new file
`.runtime/energy-path-acceptance/oracle-cache/radiant-25-1-native-candidate-03.json`
is 5,272,188 bytes, SHA256
`eb12295b46ac1082052ad9a045ddaddb7b7a60d1a2f1aca7c4c4b9c65dcf0b4d`;
current production SHA256 is
`4e12b15721da4fdb45a07f57db540b78ff0db1fbb282eb98af310cfa077ee3f4`.
No original SQL or captured result was overwritten.

Diagnostic 06 passes in 37.250 seconds: all 9,615 independent checks have zero
numerical/contract failures and zero coverage gaps. Group counts are Drivers
3,011; Loads 2,378; End uses 456; Carriers 279; Ratios 260; Completeness 312;
Residuals 1,833; Zone allocation 1,086. The exact report is
`.runtime/energy-path-acceptance/oracle-cache/radiant-25-1-native-diagnostic-06.json`.
This is a passing diagnostic, not an approved expected manifest. A separate
unapproved pending export and independent numerical/provenance review follow.

## Independent pending review and approved saved acceptance

The separate pending export passes in 37.261 seconds. Its 3,191,013-byte artifact
is `.runtime/energy-path-acceptance/oracle-cache/radiant-25-1-pending-review-01.json`,
SHA256 `724aadaf7d9ddb22508838bb438b10d783a3887fb77de61f34781c61148de1f4`.
It remains immutable unapproved review material; approval is a separate header.

A second implementation, the read-only Python audit
`.runtime/energy-path-acceptance/oracle-cache/radiant-pending-independent-audit-20260909-01.py`,
recalculates 5,376 selected quantities from 98 original Monthly sources and
1,176 observations, without Go or candidate scalars. All pass. This includes
native J/W agreement, actual air-system zeros, load and site totals, separately
signed drivers, monthly-first HVAC and carrier shares, paired ratios, cooling-
only tower allocation and wholly unassigned mixed Pumps. Maximum J/W difference
is 3.64e-12 kWh; maximum scalar summation-order difference is 2.19e-11 kWh and
maximum ratio difference is 8.89e-16. It is a partial secondary arithmetic audit,
not a claim to manually reproduce all graph provenance obligations.

Independent registry/provenance auditing also passes via the ignored
`radiant-pending-registry-audit-01.mjs`: 9,615 unique keys preserve 3,096 known
zeros, 429 explicit nulls and 260 valid found/total pairs. All 3,038 coverage
records over 52 contexts retain 6,571 required fields and 7,557 required selector
bindings, with no dangling/duplicate coverage. Roles are 2,882 primary, 78
non-flow and 78 explicitly referenced accounting records. Original model,
annual/executed input, SQL, capture/result/run manifest, engine/IDD/DLL, weather,
recipe, candidate sidecar and current 121-file production manifest hashes agree.
No candidate scalar is read by this audit.

All seven prior fixtures pass current-code saved rebuilding with fourteen
approved header/companion hashes unchanged: Large Office 81.859 seconds,
Small Office 20.408, Ideal Loads 31.031, PTAC 23.757, PTHP 25.546, Fan Coil 9.123
and VRF 79.364. This complete replay supersedes the earlier partial four-fixture
attempt; that attempt stopped during an unfinished test patch and did not reach
its final hash assertion.

Root explicitly approved the immutable pending fingerprint above after both
reviews. Preparation passes in 1.442 seconds, the review header was authored
separately, and companion installation passes in 1.973 seconds. The lossless
79,780-byte companion retains every metric; SHA256 is
`2e45d52ea57b070db2235ad35b685acc05bba82e1f29ef246d1e9c91d9bdda43`.
The eight-fixture catalog integrity guard passes in 2.18 seconds. Actual saved
acceptance passes all 9,615 metrics/eight groups in 73.75 seconds (combined
simulation package 76.101 seconds), with no engine run or captured file writes.

The localized inspector now names an active radiant surface rather than assuming
all validated emitters are floors. Its Building/Zone and Monthly/Annual boundary
and unchanged-number browser test passes in 2.064 seconds. This is automated
browser acceptance, not a native Wails end-to-end session.

Still required for this checkpoint: full repository verification/build and
normal commit/push. Section 22 is **8/19 approved**; District Energy is next.

The first full repository verification does **not** pass: app 21.328 seconds,
CLI 6.072, input 1.694, frontend/browser 164.700 and IDF 7.742 pass, but simulation
fails in 332.349 seconds at four pre-existing EPATH-070 assertions. Build was not
reached. Two hierarchy cases used a Zone-keyed radiant hand source without a
validated original equipment owner; that fixture must supply real native
ownership without changing its authoritative 30 kWh expectation. The other
two assertions expose a real omission: unowned native sources had a Context
section but no context role/category/component after leaving load selection.
Those sources now retain known non-additive thermal meaning while their Zone
remains unresolved and no load is created. Four aliases and four invalid-owner
keys are covered by expanded tests. Current-code focused and full verification
are still required; no passing gate or commit is claimed for this first run.
The already independently approved expected values remain unchanged. Candidate
03 is stale after the metadata fix and a new SHA-bound current candidate is
required for saved acceptance and all prior-fixture replays.

The correction passes all EPATH-070, production Radiant, outdoor-precision and
independent native oracle focused tests in 20.795 seconds. The two existing
SQL/review context assertions were not changed. Only the hierarchy hand fixture
now supplies the validated equipment-keyed original owner; its value and
source-ID expectations remain unchanged.

Current candidate 04 materializes in 11.046 seconds: 5,272,188 bytes, SHA256
`d45ba5392cada5a607c49a7ca9fb1b40b2d7d5e377087c814afe347332ea7796`;
production SHA256 is
`213533a445cb20d257b378db40c409a474782c2cdad6c8384b7f2a8f8c5bc87d`.
Read-only structural comparison against preserved candidate 03 finds only
192 reconciliation source-ID ordering differences. Exact ID membership/length
and the first ID are unchanged; every numeric token and all other metadata,
unknown/null and status fields are identical. This comparison is diagnostic,
not a source of expected values.

Candidate 04 passes saved acceptance against the **unchanged** approved 9,615
metrics/eight groups in 73.80 seconds (76.027-second package with catalog guard).
The old candidate/pending review remains preserved historical approval evidence;
expectations are original-SQL-derived, not dependent on a production revision.
The seven prior fixtures and full repository/build gate are now being rerun
against this latest production revision before commit/push.

That latest-code seven-fixture replay now passes completely, again with all
fourteen prior expected hashes unchanged: Large Office 79.801 seconds, Small
Office 19.034, Ideal Loads 30.790, PTAC 23.549, PTHP 24.797, Fan Coil 8.992 and
VRF 77.844. The full repository verification/build gate is being retried; the
earlier failing run is not counted as a successful gate.

The second complete `scripts/verify.ps1` passes without exclusions: app 19.664
seconds, CLI 5.422, frontend/browser 159.405 and simulation 320.483; input, IDF
and tabular pass from cache. Windows production Wails build passes in 7.63
seconds and updates `build/bin/semantic-idf-v0.4.4.exe`. Slow CPU-active Go cache
validation was allowed to finish. Normal commit-hook verification/build and
push are the remaining checkpoint steps.
