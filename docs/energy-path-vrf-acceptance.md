# VRF Energy Path acceptance

This is checklist section 22's seventh approved fixture. All 16,360 independent
metrics and required record fields pass, the expected artifact is separately
reviewed, and saved-original-wire acceptance passes. The six prior model
approvals remain unchanged. Repository-wide verification/build and normal
commit/push are the remaining checkpoint gates; section 22 is 7/19, not complete.

## Original model and additive consumption boundary

The catalog original is EnergyPlus 25.1 `DOAToVRF.idf`, SHA256
`0b533d3ec7abc449bc592f2853af6fcd994afe8d407075c16b5ec7d0f3eded4c`.
The original `VRF Heat Pump` owns the terminal list TU3, TU4, TU1, TU2, TU5.
The exact Zone owners are SPACE1-1 through SPACE5-1; PLENUM-1 is not a terminal
recipient. Both original InletSide and SupplySide DOAS mixer connections matter.

The first native contract is bounded to this reviewed electric, air-cooled,
non-heat-recovery VRF variant with draw-through ConstantVolume terminals and no
supplemental heater. Duplicate or contradictory typed ownership is rejected,
not repaired by filtering to a selected Zone.

The original meter dictionary defines two additive boundaries:

- Cooling electricity = five terminal cooling electricity observations + outdoor
  cooling electricity + outdoor crankcase electricity.
- Heating electricity = five terminal heating electricity observations + outdoor
  heating electricity + outdoor defrost electricity.

The crankcase is a **cooling** constituent in this model. The PTHP heating
crankcase contract must not be reused. Terminal coils' thermal energy and fan
electricity are not additional constituents of those two boundaries.

Both local consumption and shared outdoor consumption can serve the same Zone.
A known-zero terminal does not exclude its Zone from the outdoor pool. Nor does
the terminal's measured electricity establish that the entire Zone HVAC service
has been directly measured.

## Why a new normal capture was required

The preserved earlier capture is:

`.runtime/energy-path-acceptance/25.1/vrf-25-1/real-vrf-25-1-20260907T163246.984246400`

It contains the original manual Timestep outputs for four outdoor consumption
variables and TU1's two electricity variables, but not the individual TU2–TU5
consumption measurements. A broad meter residual cannot identify four separate
terminals' consumption. Original manual requests must not be mistaken for absent
outputs merely because they are not in a temporary-output plan delta.

The new purpose plan adds 14 exact-key Monthly/J consumption requests. A selected
Zone retains the complete original five-owner sensible cooling/heating load
roster (10 Monthly context measurements), so the shared denominator cannot
shrink to the selected Zone. Original Timestep requests and TU1 Fan Electricity
Rate remain unchanged. No new hourly/timestep fan or pump output is requested.

New preserved capture:

`.runtime/energy-path-acceptance/25.1/vrf-25-1/real-vrf-25-1-20260908T130012.807798800`

`TestEnergyPathRealModelEvidence` completed successfully in 60.096 seconds
(package time). This is capture-only evidence, not expected acceptance, and the
package time is not the native engine duration. All 14 consumption identities
have 12 non-NULL Monthly/J observations. The new SQL is 81,842,176 bytes.

| Original reporting key | Cooling dictionary | Heating dictionary |
|---|---:|---:|
| TU1 | 1494 | 1524 |
| TU2 | 1528 | 1529 |
| TU3 | 1532 | 1533 |
| TU4 | 1530 | 1531 |
| TU5 | 1534 | 1535 |
| VRF HEAT PUMP, main | 1540 | 1543 |
| VRF HEAT PUMP, auxiliary | 1555 crankcase | 1548 defrost |

These are native dictionary identities in this capture, not stable model-wide
IDs. They must be resolved from name/key/unit/frequency and exact original
ownership in each run. Source Output jump indexes refer to Output:Variable
requests; original equipment indexes belong to separate typed target metadata.

### Independent old/new capture comparison

The original-only read-only comparison found no changed existing physical result:

- Native engine completion: 17.62 seconds, 11,724 warnings, zero Severe. The
  ERR differences are run timestamp and duration; the warnings are not suppressed.
- The executed document adds exactly 14 Monthly Output:Variable objects; no
  physical object changes or request deletions. The plan grows from 961 to 975.
- All 3,984 old Monthly values (332 dictionary identities) are exactly unchanged.
- All 455,520 old Timestep values (13 identities, 35,040 each) are exactly
  unchanged, including every original TU1 Fan Electricity Rate sample.
- The 168 new Monthly constituent observations contain ten actual zeros. Their
  values agree with available original TU1/outdoor Timestep J integrals within
  1.82e-12 kWh. Five local plus two shared constituents close each service's broad
  meter in every month to at most 3.410605e-12 kWh floating-order difference.
- The TU1 fan annual integral remains 3,713.348706241785 kWh. It is not relabeled
  as an AirSystem Hourly/J pool or added to the broad Fans meter again.
- Original IDF, annualized IDF, weather, engine, IDD and DLL hashes are unchanged.

New immutable fingerprints:

| Artifact | SHA256 |
|---|---|
| SQL | `2e5d6f46d1d0fcf89cd433352faf51fb17a24f47ba0ebe4f0a35679c4408cc0e` |
| Executed IDF | `43d89b5f4dc4e8c3edcd1701cb1180112f6cd2c7079c4845574b32da3217e7dd` |
| Provenance JSON | `4e5a7d1e715faf09b84a03b8b9fa1406463ca1173055bbe4b87390cc24314719` |
| Run plan | `7433cedf27dc3567ffc50b97c4b0a5496278b2bce80512beeddf05a9771a7d59` |

The completed capture predates the subsequent Source Output-index correction
and canonical VRF projection. Its original SQL remains suitable immutable input;
its saved graph is not a current-production or approved expected snapshot.

## Observation and allocation contract

The SQL parser consumes these private observations during the existing compact
ReportData walk. They do not become extra ordinary Building end-use series.
Only dictionary and Time metadata are read separately. Original Monthly/J values
remain unrounded for allocation; the existing published source scalar applies a
single final presentation rounding.

Observation rules:

- Exact native type, reporting key, non-meter J unit, Monthly frequency, Basic
  Energy purpose and request scope must agree.
- Actual Weather Monthly calendar observations define the required axis. A
  genuine three-month run is not padded to twelve months.
- A present zero is known. Missing dictionary/row, NULL, negative/nonfinite
  values, duplicate identities and duplicate monthly rows are not known zero.
- An invalid month does not erase unrelated valid months. A missing dictionary
  does not receive an invented source identity; an observed all-NULL dictionary
  retains its real identity without an annual numeric value.
- Original Timestep J/W observations do not heal missing Monthly constituents or
  count a second time.
- Each observation records its individually validated request. The whole
  system's request-completeness flag cannot erase thirteen valid observations
  because one other request is missing.

The canonical load selector remains authoritative. A small load observation
shadow keeps the original J values for precisely those selected Monthly source
IDs. Its resolved Zone/ZoneGroup multiplier is applied once to load weights;
native site-energy consumption is already model-total and is not multiplied.

For every actual month and service, allocate each complete shared outdoor source
over **all** original owners using their known service loads. A missing owner
load does not reduce the denominator. Missing either outdoor main or auxiliary
keeps that service's shared pool unallocated, while valid source context remains
visible. Annual quantities sum the actual monthly results; no annual reweighting
is performed.

Local and allocated consumption have separate knownness. A known partial subtotal
must not supply a complete HVAC conversion ratio. Direct-only policy may retain
known terminal consumption, but must not present a terminal-only COP as if it
included the shared outdoor equipment.

The generic HVAC allocator keeps its direct-first rules. A private reservation
subtracts the full observed VRF local/outdoor subtotal before unrelated, already
eligible non-VRF recipients receive a broad meter's remainder. Even an
unallocated VRF outdoor pool stays reserved. If a VRF constituent is unknown,
the remaining meter cannot distinguish it from another central system: retain
exact direct observations and explicit unassigned accounting, without guessing
a new ownership split.

A different broad carrier cannot acquire a VRF recipient from the electric VRF
service path alone. Excluding such an unproved share does not establish that it
belongs to other recipients; their prior shares are not inflated. A separately
identified loop and exact direct Zone observations retain their existing paths.

## Focused verification to date

- Original request RED: 0 of 14 Monthly consumption requests, followed by PASS.
- Two shifted EquipmentList reference counterexamples also reproduced a genuine
  failure before the exact extensible-field-position guard was implemented.
- Typed ownership, complete selected-scope denominator, manual output retention,
  PTAC 25/PTHP 35 requests and no heavy auxiliary outputs: PASS 11.692 seconds.
- Reader RED: zero VRF cohorts where one original system was required.
- Reader final: seven tests and 30 subcases PASS, 8.913 seconds. Fan series keeps
  four original W samples in the synthetic test; its existing common Time axis
  has 16 rows (12 Monthly plus four Timestep), not four.
- Pure allocator RED: empty monthly/annual allocation roster.
- Pure allocator eight tests plus four load-shadow/bridge tests and three
  reservation tests: PASS 0.947 seconds, with unknown, zero, precision,
  multiplier, scope, duplicate, permutation and multiple-system cases.
- Output-index regression: all 14 physical equipment indexes were incorrectly
  emitted as Output request indexes (RED 0.926 seconds). After exact Monthly
  request-index lookup, eight reader tests and 34 subcases PASS 9.501 seconds,
  including absent request index, differing Timestep index and conflicting
  request guards. The original physical target indexes remain unchanged.
- Complete VRF-prefix integration tests PASS 17.735 seconds, including the
  original SQL reader through private load evidence, v2 projection and saved
  JSON source knownness, plus the separate-carrier reservation guard.
- Two additional regressions first failed (0.613 seconds): new Zone consumption
  nodes lost original HVAC navigation, and the service ledger retained native
  rather than completed source-budget display totals. The correction retains
  original physical target and service-path links while keeping Output request
  indexes separate. Display services now sum displayed constituent budgets;
  the unrounded observation/allocation plan remains unchanged.
- The final focused allocation/projection/reservation run passes 20 top-level
  tests in 1.296 seconds, including the two failures above and an explicit
  completed-display reservation test. Per-source integer shares use the original
  unrounded effective load weights directly, not precomputed rounded shares.
- Independent typed original-source, SHA/path binding and integer-allocation
  tests pass ten top-level tests in 1.654 seconds. A hand-authored test assertion
  initially pooled two separately rounded sources (7.333 instead of 7.334);
  correcting that assertion is not a production fix. Native continuous shares
  remain separately checked. A tiny load may be a positive native weight but
  display as zero: it must not create a displayed conversion pair.
- A current-code candidate was materialized from the preserved SQL in 8.88
  seconds (9.434-second test package), without running EnergyPlus. Candidate 01
  is 10,499,998 bytes, SHA256
  `3b27fd3baa4778a476ff6ecf660c5cf85939ebaae1b9a9bb9b55d29591b16f18`.
  It is not accepted and is now stale after the following bounded correction.
- Existing direct lighting/equipment carrier evidence was overwritten by the
  VRF merge (RED 0.605 seconds). The merge now preserves an existing
  `direct_zone_energy` carrier basis while the combined VRF service node remains
  `service_path_allocation`. All seven projection tests pass in 1.276 seconds.
  Candidate 01 is preserved; subsequent diagnostics require a fresh SHA-bound
  candidate rather than reusing or overwriting it.

None of these focused tests, nor capture completion, replaces final v2 graph,
source-wire, independent original-SQL expected, coverage, prior-model replay,
full verification, build and commit-hook acceptance.

## First complete independent diagnostic — rejected, preserved

The source-only saved SQL audit passes in 32.953 seconds without a production
builder, candidate or expected artifact. It proves 14 original identities,
168 Monthly observations, ten actual zero rows, source-specific integer closure,
all-owner denominators and Monthly-before-Annual pairing. Original broad meters
and constituent sums agree within a counted 128-ULP summation bound; presentation
rounding tolerance is not used to establish this physical source identity.

Candidate 02 is rebuilt from the same immutable SQL in 8.91 seconds (package
9.044 seconds), SHA256
`58023c58e8ee90d631b76c97cf733ef5f458993b44bb99a971006dea5ab81196`.
Diagnostic 01 runs all 16,360 independent checks and **fails** in 36.609 seconds:
260 numeric/contract failures plus 564 coverage failures. Drivers, loads,
end-use energy, carriers, completeness and Zone allocation numeric checks pass.
The remaining failures identify integration omissions in the independent oracle:

- 65 Zone ratio-quality checks omitted the other original owners' denominator
  observations from the conversion-only source union. These weights must not
  become the selected Zone's delivered-load or consumption sources.
- 195 Zone reconciliation checks expected an inherited scoped ID instead of the
  fresh canonical plain carrier/period ID produced by this complete native path.
  All 65 actual rows' quantities and Zone/period/basis/status metadata agree.
- 44 allocated-source selectors reached a generic target validator before their
  dedicated typed VRF source check. A bounded dispatch reproduces the failure
  (0.612 seconds) and then passes source/coverage tests in 1.272 seconds, retaining
  all 132 source obligations without unknown-to-zero fallback.

The full diagnostic and both candidates remain preserved. No expected artifact
has been approved. Typed source-role and exact ledger metadata checks are also
being strengthened independently; the generic numeric guards remain unchanged.

The conversion-only original-weight union now passes all focused native consumer
and existing generic quality tests in 12.629 seconds. Other owners' loads remain
allocation evidence only; they do not become the selected Zone's delivered load
or measured consumption. This focused result does not yet replace the required
second full diagnostic.

## Complete independent diagnostic 02 — passed, not yet approved

Diagnostic 02 passes in 37.382 seconds against the unchanged Candidate 02 and
original SQL: all **16,360** independent checks, zero numeric/contract failures
and zero required-field coverage gaps. The exact group counts remain drivers
5,854; loads 4,522; end uses 534; carriers 171; ratios 455; completeness 546;
residuals 2,964; and Zone allocation 1,314. No failed selector was removed.

The bounded integration corrections and their focused results are:

- Native carrier ID selection now requires the original complete local/shared
  service roster, exact Monthly meters, all-owner allocation proof, exclusively
  unassigned auxiliaries and no other allocated fan/direct-HVAC route. It does
  not search the candidate for an ID that happens to match. New ID regressions
  and existing ZoneCarrier tests pass in 6.802 seconds.
- Native ledger checks independently rederive Direct and Allocated columns from
  each source's role, owner and rounded Monthly budget. Annual sums bind the same
  source identities across all twelve months. Exact ID, service, method, basis,
  Zone and period metadata are checked. Coordinated Direct+0.001/Allocated-0.001
  changes to Monthly, Annual and candidate rows are rejected. Fifteen ledger,
  service and consumer tests pass in 1.116 seconds.
- The pending writer had the same generic `sources/allocatedValue` selector
  omission as coverage (RED 0.487 seconds). It now uses the identical complete
  typed source guard. All pending-export regressions pass in 2.803 seconds,
  retaining 44 allocated source fields and 40 genuinely unknown scalar fields
  without widening the generic selector contract.

Only after the complete passing diagnostic, a new unapproved review artifact was
written to `.runtime/energy-path-acceptance/oracle-cache/vrf-25-1-pending-review-01.json`.
Its quantities come from the independent SQL evaluator, not candidate scalars.
Separate original-to-pending arithmetic also passes: a read-only Python audit,
without the Go compiler or candidate scalars, recalculates 2,655 core quantities
and explicit nulls from 43 original sources and 516 Monthly observations. All
match; maximum floating summation-order difference is 1.4552e-11 kWh. All 104
native display-ledger keys agree exactly. A further 182 quality statuses agree
(84 overmapped, 98 partial). This is a separate partial arithmetic check, not a
claim to manually reproduce all 16,360 compiled obligations.

Native physical closure and displayed accounting remain distinct. Annual Cooling
ledger E/D/A/U is 10983.599/688.817/10294.775/0.007 kWh; Heating is
9013.367/326.341/8687.029/0.002 kWh. The signed Heating residual is -0.003 kWh,
whereas positive monthly overlap sums to 0.005 kWh. The recorded overmapped
quality status reflects this displayed ledger; it is neither erased nor asserted
to prove excess native physical consumption.

The independent registry/provenance review passes: pending SHA256
`32610d187d2d4c2e33ca3ecc11d6369fc62ed7ea6a27621eae6957ca5a6a7008`,
5,034,894 bytes. The exact 16,360-key registry preserves 6,333 known zeros, 696
explicit nulls and 455 valid found/total pairs. All 4,477 coverage records across
91 Building/Zone-period contexts retain 9,597 required fields and 11,137
required-field selector bindings. Roles remain 4,256 primary, 143 non-flow and
78 explicitly referenced accounting records. No missing/duplicate selector,
field, context or scalar presence was found. Original/capture/SQL/recipe,
candidate sidecar and current production hashes agree; the review reads no
candidate scalars and does not approve or mutate the pending artifact.

All six earlier approvals also pass current-code saved rebuilding with unchanged
expected headers and companions: Large Office 80.764 seconds, Small Office
19.093, Ideal Loads 30.958, PTAC 23.758, PTHP 25.131 and Fan Coil 9.357.

Root explicitly reviewed and approved the immutable pending SHA above, then
prepared the lossless 129,633-byte companion (2.911-second package), authored the
expected header separately and installed the companion (3.031 seconds). The
payload preserves all 16,360 metrics; its SHA256 is
`cf2c8eefcc30309076fbb14408e4443a31f0a5de812a29c5c5f32200800b5a27`.

The first saved-acceptance attempt still **fails** in 64.625 seconds, despite the
seven-fixture header/payload integrity guard passing. The acceptance assertion
has its own second SQL read; unlike the diagnostic and collection paths, it did
not bind the original IDF bytes before recompiling the VRF ownership proof. The
failure correctly rejects missing original equipment identity. That assertion
now calls the existing exact path/hash-bound original reader before evaluation.
No production, candidate, recipe, pending or approved expected value changes.
All actual oracle entrypoints were re-audited. The rerun passes all 16,360 approved
metrics in eight groups: saved acceptance 71.45 seconds; combined package
74.032 seconds including the seven-fixture integrity guard and original-context
path/hash counterexamples. The preserved first failure is not counted as a pass.

## Remaining checkpoint and checklist work

Canonical projection/source integration and the independent capture comparison
are implemented and focused-tested. The new original-only
`oracles/vrf-25-1.json` is reviewed but explicitly **not fixture acceptance**: it declares 16
pressure families, seven electricity site observations, 40 availability groups,
five original terminal owners and 14 separate native consumption sources.
Cooling uses the existing COP contract; heating conservatively uses the
delivered-load/site-energy ratio, not an asserted equipment-rated COP.

Independent pending review, immutable expected packaging, all six prior-fixture
replays and saved expected acceptance are complete. Full repository verification,
Windows production build and normal commit/push follow this checkpoint.
The next fixture is radiant heating/cooling; twelve catalog fixtures remain.
Checklist sections 23–26 remain after all 19 fixture approvals.

The full `scripts/verify.ps1` gate now passes without exclusions: app 20.296
seconds, CLI 5.381, frontend/browser 164.607 and simulation 318.018; epinput,
IDF and tabular pass from cache. Windows production Wails build passes in 8.137
seconds. Normal commit-hook verification/build and push follow.
