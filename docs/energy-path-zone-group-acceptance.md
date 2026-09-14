# ZoneGroup: native equipment and multiplier boundaries

Engineering work only; the remote UI is unchanged. This eleventh catalog
fixture now has independently reviewed expectations. Saved-run and repository
gates are recorded below; the overall nineteen-fixture goal remains active.

## Original model and source boundaries

The unmodified EnergyPlus 25.1 `MultiStory.idf` has nine representative Zones,
each with a WindowAirConditioner, a Fan:OnOff, a DX single-speed cooling coil and
a convective electric baseboard. Original EquipmentList, EquipmentConnections,
typed component ports and outdoor-node declarations establish the exact owners.
No central AirLoop or unrelated path is borrowed for these direct packages.

Each representative Zone has 24 square metres. Original Zone and floor
ZoneGroup factors are independently compared with both SQL fields, not merely
their product. Effective building area is 1,440 square metres. The ordinary
AllZones list selects internal gains; it is not another multiplier.

| Native observation | Effective accounting | Physical role |
| --- | --- | --- |
| Coil/crankcase/fan electricity | Already model total; factor one | Separate purchased inputs; exclude overlapping WindowAC package total |
| Baseboard electricity | Already model total; factor one | Purchased heating input |
| Baseboard Total Heating | Already model total; factor one | Non-additive response, not another Zone load |
| Zone Air System sensible energy | Zone factor times Group factor once | Canonical delivered thermal load |
| Zone Hot Water Equipment District Heating Energy | Zone factor times Group factor once | District-water interior equipment, not HVAC heating |
| Facility and broad end-use meters | Already model total; factor one | Independent reconciliation boundaries, not additional consumers |

Convective electric baseboards do not acquire radiant recipients. Native
capacity-method and efficiency defaults are respected without rewriting the
input; invalid selected capacities remain invalid. Editable field comments
cannot override actual native multiplier or ZoneGroup ownership positions.

Source provenance distinguishes Monthly consuming observations from Hourly
chart and Rate companions. Missing, NULL, duplicate and wrong-unit evidence
cannot stand in for native observed zero. Exact source-to-path bindings do not
fall back to another system when ownership is ambiguous. A subtotal containing
both observed local cooling and allocated central cooling retains both kinds
of evidence through monthly/annual aggregation and stored JSON.

## Preserved real captures

Original SHA-256:
`c7165328a5f3a90aa81ac3f95928600079cf9f7a109fd615ccc6686caf518ba2`.
The September 7 capture is retained. New normal-runner captures add native
output requests without changing the physical model, annualization or weather.

The first new capture, `real-zone-group-25-1-20260914T155913.394057900`, completed
in 103.327 seconds. It has SQL SHA-256
`1cab549f2d953441de96ecabffb691eb3233c0fd0a93c130f64fb7b393c6b886`.
Independent audits verify nine owners for each native cooling/crankcase/fan
family and 72 baseboard Energy/Rate Monthly/Hourly identities. Monthly values
have twelve observations; Hourly values have 8,760. Actual weather environment
index is four, type three, with native WarmupFlag NULL.

Native cooling and fan annual sums are 46,992.05968821516 and
1,387.209444590907 kWh. Their monthly broad-meter closure differences are at
most 2.183e-11 and 5.685e-13 kWh. All nine crankcase outputs are actual zeros,
including 108 Monthly and 78,840 Hourly zero observations. Every observed
baseboard Total Heating value exactly equals Zone Air heating times the original
Zone and Group factors: 108 Monthly and 78,840 Hourly comparisons. These are
independent native-source checks, not quantities inferred from the graph.

The same audit found an additional real omission: HotWaterEquipment district
consumption existed as representative Zone variables, but the building-wide
district meters were not requested. This SQL has no Tabular tables, so its HTML
annual report cannot masquerade as a SQL fallback. The first new capture and
its materialized candidate are retained but cannot approve the corrected code.

The correction requests native DistrictHeatingWater:Facility and
InteriorEquipment:DistrictHeatingWater, with version-compatible aliases and
Hourly companions. A separate internal-gain feature flag prevents coexisting
electric space heating from creating a false district Heating request. Existing
Zone direct-use accounting remains representative-factor-once.

The second new capture, `real-zone-group-25-1-20260914T161342.768401800`, completed
in 101.267 seconds. Its SQL SHA-256 is
`c72db1c09745099d7e7ae304ef98d72ed67db9d6678f9b9a388e6c64fc1ed5cf`; executed-input
SHA-256 is `21312038f41574bb15cedc1b052c9cd533d18cccf081179bb197068c0b4c5119`.
Both new district meters contain twelve Monthly and 8,760 Hourly observations,
without NULLs. Each annual meter is 11,336.763487621001 kWh; the maximum monthly
difference from independently multiplied Zone consumption is 1.683e-11 kWh.
The root checked all 78 preservation-baseline hashes and file lengths unchanged.

Independent recapture comparison verifies all 1,191 original dictionaries,
5,053,118 common native observations and 8,772 time records are exactly unchanged.
There are no missing, duplicate or changed common values. The only four new SQL
dictionaries are the two district meters at Monthly and Hourly frequency,
adding 17,544 observations. Both new meters exactly equal the independently
multiplied Zone sum at every one of the 8,760 hourly timestamps. Each has 2,403
genuine hourly zeros and no monthly zero, NULL, missing or negative observation.

The official model retains 7,168 physical-model warning occurrences, including
DX frost and runtime convergence warnings. The first new capture did not
increase the original count. The second has 7,172 total warning occurrences:
four additional invalid-key warnings are the legacy district aliases requested
at both frequencies on the modern engine. The remaining error-log body is
unchanged except timestamps and elapsed time. Source-accounting correctness does
not certify HVAC design quality; no physical input or convergence tolerance was
changed to conceal these warnings. Both SQL files retain their native
Simulations completion flags as the string FALSE; successful completion is
established by engine/runner terminal results and the native ERR success record,
not by rewriting or misreporting these SQL flags.

## Verification state

Focused IDF and simulation gates pass, including native exact owners, valid
defaults, missing/zero distinctions, mixed provenance, original multiplier
fields, native/shared fan separation and a thousand-surface monthly capacity
regression. HWE tests independently check model-total 330 against Office raw
10 times 32 plus Lab raw 5 times 2. Same-SQL modern/legacy/reverse aliases and an
unequal annual Tabular shadow cannot multiply that amount or fill observed
monthly zeros. Two v2 JSON reloads retain the numerical and source boundaries.

Second-capture candidate materialization passes in 34.812 seconds. Independent
recipe compilation produced 21,413 checks. The first complete diagnostic failed:
the new recipe/checks incorrectly required native zero electric latent sources
on positive compound equipment flows, classified resistance heating as COP,
used incompatible zero-presentation flags on fan raw/effective fields, and
selected legacy allocated ledger IDs for fully native direct Zone consumption.
Independent source review confirmed the actual totals; this failed diagnostic
did not generate a pending expectation or approve the fixture.

The bounded corrections retain every mandatory source scalar and all eight
coverage groups. Only exact native Monthly zero leaves can be optional on a
flow trace; missing/NULL, nonzero values and signed cancellation cannot qualify.
Heating uses the established load-to-site-energy classification. Tiny paired
display ratios retain their original endpoint quantization bounds rather than
being forced to nominal equipment efficiency. Native fan ledger identities
require the complete original-owner/source cohort, including observed zero
months. No production quantities or acceptance tolerances were changed by this
diagnostic correction.

The corrected focused gate passes in 37.239 seconds. Its first attempt exposed
a new hand-test fixture that did not actually serialize zero-valued fields;
explicit JSON zero and omitted-field cases are now tested separately, without
changing the production reader's missing-value semantics.

The second complete diagnostic passes in 281.538 seconds: all 21,413 checks,
eight groups, zero numerical/contract failures and zero coverage gaps. The
immutable pending SHA-256 is
`f123e415717bb10389930e94e6843e41e79ded715bac119b8a57643c26ace0b0`.
Its required-key SHA-256 is
`bba1120400bc929a0e1b1112ea4e24a3151bb6f0609e434d685bfb6ccdb6276b`.

A separate original/native-SQL arithmetic audit compares 5,407 fields against
a basis fixed before the pending artifact existed, including all 884 ratio
and 498 carrier fields. Ordinary native differences stay within independent
64-ULP operand-scale bounds, with a maximum 1.383e-10 kWh. All 36 baseboard
context scalar fields separately match the existing per-row 0.001-kWh transport;
the largest difference from a lossless native annual sum is 0.001924 kWh. This
context rounding is not substituted for physical load or purchased consumption.

The independent registry audit checks all 21,413 identities across 130 contexts,
6,441 coverage records, 13,351 required fields and 15,535 required bindings. It
separately generates and matches all 1,354 source-only keys against original
ownership and native dictionaries; coverage records alone do not contain a
Sources collection. The 177 generic Hourly companions are explicitly
non-additive and bind their selected Monthly authority, not another invented
set of consuming scalar metrics. Ninety copied Building allocation ledgers
retain their exact four-field referenced-accounting contract, not Zone-local
accounting. All original/candidate/SQL/production/engine/weather hashes agree.

All ten prior fixtures pass current-code saved-run rebuilding: Large Office
81.887 s, Small Office 19.615 s, Ideal Loads 33.051 s, PTAC 24.045 s, PTHP
25.044 s, Fan Coil 9.032 s, VRF 79.024 s, Radiant 84.450 s, District 21.369 s
and Mixed Heating 234.203 s. All twenty prior expected artifact hashes remain
unchanged. The original and both previous captures are preserved.

Root explicitly reviews the pending artifact above and authors the new expected
header. The deterministic 179,030-byte companion preserves all metrics, including
7,086 explicit zero metrics and 1,154 typed nulls. Companion SHA-256 is
`afd5d9848c7f13c4bce975ef94a8a97b147fba9cb2823d1cdb9503c02b29ce70`;
uncompressed SHA-256 is
`deaaae0ece2581a01a6dabab1c6a618ceb731ee1b85cf551c41018d4e3a342bc`.
Preparation and guarded installation pass in 12.576 and 12.760 seconds; neither
step derives expectations from the candidate or rewrites the approval header.

Fresh current-code saved acceptance passes: all 21,413 independent metrics and
approved values agree in 567.89 seconds. The eleven-fixture approved-artifact
guard also passes; the combined command completes in 570.904 seconds. No engine
rerun, captured-file replacement or expectation update occurs in this replay.

The first full repository gate reaches a single simulation test failure in
552.382 seconds: an older PTAC discovery assertion rejects the now-reviewed
WindowAC fan output name. The updated test requires the exact native Fan:OnOff
family, still rejects Rate/unknown aliases, and retains unchanged PTAC request
counts. App, CLI, input, frontend/browser, IDF and tabular tests pass; Wails is
not reached in this failed gate.

Final peer review also finds a malformed zero-field `NodeList;` can panic when
an unrelated Zone equipment connection legitimately has blank air ports. The
bounded production guard rejects blank references before lookup and iterates
fields within their actual length. Regressions retain the exact valid owner and
service while still rejecting real competing ports in either duplicate-list
order. The focused final guard gate passes: IDF 0.734 seconds, simulation
23.346 seconds. All 78 preservation-baseline files remain unchanged.

This parser-safety change makes candidate02 historical, not a current-code
snapshot. The independently reviewed expected header and companion are unchanged.
Fresh post-guard saved acceptance passes all 21,413 metrics in 572.91 seconds;
the combined eleven-fixture catalog/saved command passes in 575.932 seconds.
Earlier ten-fixture replay timings above precede this bounded parser guard.

The second full repository gate passes without exclusions: app 21.899 seconds,
CLI 5.963, input 1.382, frontend/browser 184.073, IDF 9.107 and simulation
551.041; tabular passes from cache. Production Windows build passes in 8.104
seconds. Remote fetch still shows no incoming commits. Normal commit-hook
verification/build and push finish this engineering checkpoint; no original,
previous capture or approved expected value is rewritten.
No overall goal completion is claimed.
