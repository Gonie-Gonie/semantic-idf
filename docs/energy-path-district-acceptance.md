# District Energy acceptance

District is the ninth approved engineering fixture in checklist section 22.
Original observations, source declarations, candidate diagnostics, independent
expected approval and saved acceptance are separate evidence gates. The overall
engineering goal is not complete; subsequent equipment/source checks remain.

## Original evidence

- Fixture: `district-energy-25-1`, EnergyPlus 25.1.
- Original: `5ZoneFanCoilDOAS_ERVOnAirLoopMainBranch.idf`, SHA-256
  `3b7712676a8ea7cb034cec7eb0b843731d12ba79cf72d3d54f3b3e6481b765da`.
- Preserved capture: `.runtime/energy-path-acceptance/25.1/district-energy-25-1/real-district-energy-25-1-20260907T163605.594498300`.
- Original SQL SHA-256:
  `3949ede8734d220bdbb1a02a1d95902ceebcb4a2f03dc71d7570578c72e276a2`.
- Native engine completion: 20.67 seconds, eight warnings and no Severe errors.
  Completion does not certify the model's design assumptions; warnings remain.
- Independent saved SQL observation replay passes in 4.229 seconds without an
  engine run. It collects 424 identities over the complete 2017 weather axis;
  the observation JSON SHA-256 is
  `51293eaa8b56641cc55639bcc329b0471c88d7cbe10aedbf5920b8cd15cd948d`.

All 347 Monthly identities have twelve unique, finite, non-NULL observations:
4,164 rows. The remaining 72 Hourly variables and five Run Period meters retain
their own frequency. Monthly `WarmupFlag` is NULL in this original SQL; the
existing non-warmup NULL/zero reader handles it without discarding observations.

## Reviewed physical boundaries

Six Zones comprise PLENUM-1 and SPACE1-1 through SPACE5-1, all with effective
factor one. Only the five SPACE Zones own equipment: each has a local FourPipe
FanCoil and a terminal on the shared DOAS AirLoop. Exact hydronic cooling and
heating branches reach separate district cooling and district heating water
plants. The DOAS water coils legitimately occur on both air and water branches.
Multiple delivery routes do not turn one Zone's delivered load into two loads.

Monthly Zone Air System Sensible Cooling/Heating Energy is the load authority.
The PLENUM has 24 genuine zero observations; its absent people, lighting,
equipment and infiltration identities must not be fabricated as measured zeros.
Latent Zone load is absent, not zero. Purchased district energy uses
`load_to_purchased_energy`, not COP or combustion efficiency.

The 46 passive surfaces retain eight exterior walls, one roof, five ground
floors, six openings and 26 interzone faces. There is no active Radiant surface
exclusion. Four gross infiltration gain/loss families apply only to the five
SPACE Zones. Their outdoor aggregate remainder is machine-scale cancellation,
not a separately measured mechanical-ventilation driver. System/DOAS recovery
thermal observations remain context rather than another delivered Zone load.

Ten numerical site sources include an actually observed zero HeatRecovery
electricity meter. District end-use and facility meters close exactly each
month. Electricity Building/HVAC/Plant are overlapping contexts, not added
consumption. Missing Cooling/Heating Electricity and obsolete district aliases
are availability gaps, not zero site sources.

The broad Fans pool contains one DOAS fan and five local fans. A real original
Hourly DOAS supply-flow series exists, but cannot divide the whole six-fan
unmeasured pool; Fans remain unassigned. Two single-service plant pumps share
the five served Zones. The proposed `plant_loop_load_share` estimate weights
each month's broad Pumps by cooling-plus-heating Zone load, then sums months.
It is not measured per-pump energy. Exact topology eligibility, deduplicated
path provenance and fail-closed inventory tests are required before acceptance.

## Original request and regression history

The independently inspected 45 requested availability families imply Drivers
13/19, Loads 9/14, End uses 7/9 and Carriers 3/3. These ratios are request/source
coverage, not a claim that every Zone owns every alias. The 969 saved requests
are temporary and have no original object index. Original Hourly output
requests must remain intact alongside Monthly requests; no heavy individual
fan/pump electricity or airflow request is being added to Basic Energy.

The literal original-only recipe is now independently reviewed: sixteen driver
families, forty-five availability groups, ten site sources, six sensible-load
owners and five served/direct-use owners. All 159 selector-owner resolutions
match exact original dictionaries; all request names and 64 observed name/unit
alternatives match the separate original audit. Recipe SHA-256:
`24b97fa062def5d4e51404a00371bebfc3f05a4eae7d87389c9ee80a08d8c2d0`.
This is source review, not expected approval.

The first focused regression run fails (IDF 1.306 seconds, simulation 2.054).
Original pump inventory, duplicate-route weights, malformed global inventory,
mixed-fan nonallocation and original Hourly request preservation pass. Three
separate findings are being handled without changing original quantities:

- Two new test assumptions need correction: the existing side label is
  `Demand`, and unassigned Fans retain an allocation residual equal to their
  unassigned energy rather than zero.
- Generic Output navigation chooses an earlier Hourly request for a Monthly
  variable or meter. The new regression reproduces four frequency mismatches;
  a bounded frequency guard now requires the actual source boundary while
  preserving input defaults, alias policy and native direct-source handling.
- A disconnected terminal outlet can still obtain a central plant-backed
  service path through the ADU display outlet. The raw terminal/ADU continuity
  evidence is being checked at the graph-edge authority boundary.

These failures are historical; the current integration evidence follows.

## Current remote integration — 2026-09-14

Remote changes through `52b6026` are integrated without conflicts, preserving
all eight local task files and an explicit recovery stash. The user approved
the remote UI and narrowed remaining work to engineering completeness. This
fixture does not require restoring removed UI controls or inspector actions.

The generic source/output match now requires the exact reporting frequency,
including the documented Hourly default and Run Period normalization. A known
contradiction between native terminal outlet and ADU outlet no longer creates
false validated/serves graph edges. Unknown composite terminals retain existing
handling; original fields and diagnostic evidence are not rewritten.

Remote adds actual Hourly chart requests. The live District plan now has 967
Monthly/Hourly pairs and two support requests: 1,936 total. Existing original
Hourly identities retain exact original indices and fields; unmatched requests
are temporary. The saved capture and its 969-request plan remain immutable.
Focused IDF/simulation tests pass in 1.420/4.458 seconds, including District
topology, allocation, purpose requests and current Hourly/area regressions.

Current candidate `district-energy-25-1-candidate-20260914-01.json` is 13,037,530
bytes, SHA-256 `f3a116849010d5a7b21610cb714f101eda6bc30262d557794358f0ce385f4ee0`;
production fingerprint is
`4cc2c151507345ca1e01b1af74f446e8b3a9f78530d9891be8ebbdb217d71a2a`.
Materialization passes in 4.827 seconds without an engine rerun. The separate
diagnostic passes in 10.721 seconds and pending export in 10.544 seconds:

- Drivers: 5,386; Loads: 4,210; End uses: 469; Carriers: 513.
- Ratios: 455; Completeness: 546; Residuals: 3,588; Zone allocation: 1,834.
- All 17,001 checks pass with zero numerical/contract failures or coverage gaps.

Independent Python recalculation from original SQL/IDF passes 3,198 selected
core quantities and 5,386 driver/raw-source quantities, without candidate
scalar inputs. Maximum absolute discrepancies are 3.638e-12 and 1.819e-12 kWh;
J/rate consistency is within 1.137e-12 kWh. A separate registry/provenance audit
passes all 17,001 unique keys across 91 contexts, 5,287 coverage records, 11,474
required fields and 13,176 required selector bindings. It preserves 5,757 known
zeros and 650 explicit nulls (221 partial, 416 unavailable, 13 not applicable).
Historical September 9 evidence is preserved but stale after remote integration.

The current independent pending file is 5,581,849 bytes, SHA-256
`83f9984140399e7e5f303ad8c92e9b73f29b6660d570b970cd80d9de22f6839f`.
Its exact required-key SHA-256 is
`7e427660bab6b4824f0cbe0e772a03eb10925a1aa2ac9eb9cbd0d78ae937680e`.
All eight previously approved fixtures now pass current-code saved rebuilding
and independent expected acceptance, with all sixteen existing header/companion
hashes unchanged: Large Office 82.948 s, Small Office 19.687, Ideal Loads 31.654,
PTAC 24.512, PTHP 26.055, FanCoil 9.591, VRF 81.766 and Radiant 86.406.

Root explicitly approves this immutable independent pending SHA-256 on
2026-09-14 after the original physical/source review, complete diagnostic,
separate original-to-pending arithmetic and registry/provenance audits, and
unchanged prior-fixture replay. The 65 zero-to-machine-residual transitions
versus the historical pending are confined to heat residuals at at most two
ULPs of their original loads: 37 zero-to-tiny and 28 tiny-to-zero. All 576 original
source metrics and all statuses/null states are unchanged. Independent map
summation order explains this difference; no residual is coerced to zero and
no acceptance tolerance is relaxed.

The manually authored expected header and generated 139,549-byte companion
preserve all 17,001 metrics. Packaging prepare/install passes in 2.840/3.398
seconds. Companion SHA-256 is
`99f61783c1aaca45be9fde246445e64c1d6de72ef73955c47ebc5b9de412642f`;
uncompressed SHA-256 is
`a5ffbaf85d3a3c05eb5745944627386a53e58551d49a4c415828c145cd5c22e8`.
Separate saved acceptance passes all eight groups and 17,001 metrics in 19.28
seconds, with the nine-fixture catalog integrity guard passing in 2.33 seconds
(21.760-second package). The exact current original-wire candidate passes all
production/capture/input/engine/weather/SQL fingerprints without artifact writes.
Full repository/build and normal commit/push are the remaining checkpoint gates.

The first complete repository gate after remote integration fails before build.
Frontend checks expose three remaining contracts for removed navigation UI and
one HVAC snapshot/loop-state browser regression. The IDF disconnected wrapper-
inlet counterexample also reports a false heating path. These are being separated
into stale assertions versus actual connectivity/data defects; the approved UI
will not be restored merely to satisfy obsolete tests. The simulation package
then fails in 385.104 seconds on four legacy SQL fixtures that omit reporting
frequency but expect exact Output-object identity. No successful full build or
commit/push is claimed from this run.

The correction retains the remote UI: three obsolete cross-panel test contracts
are replaced by standalone/local-state guards, and the HVAC browser test checks
the exact inline temperature/setpoint comparison and intentionally shared time
selection independently of loop-local graph settings. Its exact numerical,
source, scope, multiplier and no-backend assertions remain. IDF now rejects a
DX wrapper's contradictory native outer ports before establishing the branch's
conditioning paths; child-coil failures remain service-local. Focused IDF and
frontend checks pass in 3.680/22.346 seconds.

The four sparse SQL fixtures now explicitly declare Monthly energy and Hourly
rate reporting boundaries with matching output plans. All original numerical
and object-index assertions remain, with explicit frequency checks added.
The first corrected run also exposes newly identifiable daily/hourly rate
periods; exact additional period/integration checks ensure Monthly energy is
not copied into them. The corrected simulation focus passes in 2.469 seconds.
Because the physical wrapper guard changes production, current-code District
evidence and all prior approvals are being replayed before the full-gate retry.

Latest production fingerprint
`b0ba0285f38d074c6c288124747ac39755455911688167929f5c2bfa03bce452`
is bound to the separate preserved candidate 02, SHA-256
`4092225b0498b52ce1694aba45b141eacc1af61dc347e7128fdb6f29b6f7a891`
(13,037,530 bytes; materialization 4.164 seconds). It passes all unchanged
17,001 approved metrics and the nine-fixture integrity guard in 21.760 seconds.
All eight prior fixtures again pass with their sixteen expected hashes unchanged:
Large Office 82.533 s, Small Office 19.537, Ideal Loads 33.102, PTAC 25.284,
PTHP 26.824, FanCoil 9.461, VRF 81.777 and Radiant 87.534. Independent peer
review finds no weakened engineering assertions or unintended guard broadening.
The full repository gate is now retried. The reopened application must close
before the final executable can be replaced; it is not forcibly terminated.

The second complete `scripts/verify.ps1` gate passes without exclusions: app
24.260 seconds, CLI 7.141, input 1.737, frontend/browser 191.598, IDF 9.209,
simulation 376.871 and tabular from cache. Production Windows Wails build
passes in 6.08 seconds. No application was forcibly terminated. The generated
26,463,232-byte executable has SHA-256
`abbf6b7f96fed50e71d8caccfc84bb84c766d73d6b41bbcfc8b560ef1bbc9a90`.
Normal commit-hook verification/build and push finish this checkpoint.

### Remote area and Hourly chart boundary

A separate original-SQL audit checks 105 area scopes and 29 actual Hourly
traces (254,040 values), including all twelve Zone sensible cooling/heating
sources. Building area is 927.2 m² including the 463.6 m² PLENUM explicitly
included in EnergyPlus total floor area; zero load is not a reason to exclude
its area. The exact 8,760-point annual axis is retained. No monthly value is
expanded into fabricated Hourly observations.

Hourly chart conversion uses the existing explicit 0.001 kWh quantization, not
lossless native values. There are 211,196 actual SQL zeros; 61 positive values
round to zero across 22 identities. Maximum per-hour rounding error is
0.000499991 kWh. The largest annual chart-versus-native-Monthly difference is
0.025485746 kWh and monthly difference 0.012586176 kWh. Unrounded original
Hourly/Monthly totals agree within 3.638e-12 kWh. This bounded chart precision
does not change the source-aware Monthly Energy Path accounting or redefine
rounded display zeros as independently observed source zeros.
