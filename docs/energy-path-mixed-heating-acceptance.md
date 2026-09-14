# Mixed heating: source boundaries and acceptance evidence

This checkpoint concerns engineering correctness only. The remote UI is unchanged.
The `mixed-heating-fuels-25-1` fixture is the tenth independently approved
numerical fixture. Saved acceptance passes all 17,275 metrics. Repository/build
and commit gates are recorded separately below; the overall goal remains open.

## Original model and physical boundaries

The unmodified EnergyPlus 25.1 `5ZoneElectricBaseboard.idf` contains five served
Zones and a return plenum. SPACE2-1 and SPACE4-1 have radiant-convective electric
baseboards, while the central natural-gas boiler also serves those Zones through
the main heating air coil. Owning a local heater does not exclude a Zone from
central heating. Exact EquipmentList, EquipmentConnections, native component
ports, branch and loop membership establish these relationships.

- Native baseboard electricity is already model-total. Apply no second Zone
  multiplier. Zone Air System sensible load remains the aggregate thermal
  boundary; baseboard Total Heating is non-additive response context.
- The ten configured recipient surfaces retain their observed convection.
  They can contain both environmental and HVAC response, not an isolated passive
  or baseboard-only heat gain. Configuration fractions do not measure operation.
- Separate native local and shared-boiler electricity budgets. An unidentified
  meter remainder stays unassigned. An observed zero is different from missing
  data. Monthly energy is the consuming budget; Hourly and Rate companions are
  trace context, not extra energy.
- Keep direct and allocated carrier branches separate, including when both use
  electricity. A mixed heating subtotal and its conversion cannot claim that all
  of the selected Zone's energy was directly observed.
- Fans use the sole original supply fan's five-Zone air-loop ownership and a
  cooling-plus-heating load-share estimate. This is not native fan metering or a
  measured airflow split. Mixed hot/chilled/condenser pump electricity remains
  unassigned because individual consuming budgets were not observed.
- HeatRejection is a broad meter. Its MTD roster has one tower-fan constituent,
  but the SQL does not contain native cooling-tower Electricity E/R variables.
  Related Zone shares are cooling-load estimates, not direct tower measurements.

## Preserved live-run evidence

The original September 7 capture is retained. A separate normal runner capture
`real-mixed-heating-fuels-25-1-20260914T125004.684985500` adds correct native output
requests without changing the physical model or weather. It completed in 64.146
seconds; saved-evidence metadata verification passed separately in 24.787 seconds.

| Identity | SHA-256 |
| --- | --- |
| Original model | `cc69ffb0c991ca82951bfe4df351bb51280f97cca92155515ef1cd772bcc1d58` |
| Executed input | `46574cd884284c5e16faef9a02a13eaa5124407666fabf48b501b2b1fdbabb1c` |
| New SQL | `d104dfb1fd7ec9f7f7e9001d59cc02b282217e9bc9e4d51bb31812c4a93cfc32` |

The existing 222,871 SQL observations have identical values and timestamps, with
no missing or duplicate identities. The 8,772-row weather time axis and annualized
input are unchanged. All 46 preserved capture/approved-expectation hashes match
the preflight baseline. The new run has 23 warnings, zero Severe and zero Fatal
errors; this is not a warning-free claim.

Independent native SQL sums are 251.68286880208026 and 455.3018727624689 kWh for
the two baseboards, totaling 706.984741564549 kWh with actually observed zero
boiler ancillary electricity. The maximum monthly native meter/constituent
closure difference is 2.842e-13 kWh. Native baseboard Total Heating responses are
221.1551861493879 and 398.20613250492454 kWh and are not extra delivered loads.

## Acceptance gates

The first immutable candidate rebuilt in 20.397 seconds. Its independent
17,275-check diagnostic rejected it in 37.721 seconds. This exposed two production
defects that simpler equipment fixtures did not reveal: a zero/missing shared
electricity budget could acquire a gas-carrier fallback share, and the same served
Zone thermal load could be counted once per carrier while remaining below the
larger Building load that includes the unserved plenum. Both fixes now have actual
V1-preallocation-to-v2 and JSON round-trip regressions that now pass focused
verification. The second immutable candidate also passed 592 separately derived
original-IDF/native-SQL arithmetic checks, including every Zone's monthly and
annual heating branches and honest meter/constituent display-rounding differences.

The same diagnostic found missing independent proof of legitimate Hourly
companions in derived Monthly/Hourly source traces. The source quantities are
not additional drivers. Their original native Hourly/Monthly identity, calendar,
ownership and sum correspondence must be proven per contributing source before
those trace records can count as covered. Failed candidates and diagnostics are
retained rather than overwritten or accepted by relaxing tolerances. Exact
Hourly-companion proof and its missing/NULL/duplicate/calendar/owner/multiplier
mutation tests pass. The second full diagnostic has zero failures and coverage
gaps in drivers, end uses, carriers, completeness, residuals and Zone allocation.
It rejected 52 context checks and 26 ratio checks (208 failures including
coverage), and was not accepted.

Those findings established precise provenance boundaries: the original and executed
Output:Variable indices must be independently bound; non-additive Hourly native
electricity retains a measured model-total effective value, not an allocation;
the Building thermal conversion may not borrow an unserved plenum's source from
its larger canonical load node; and a directly observed baseboard electricity
branch must cite its exact consuming delivery path, not unrelated central
cooling/gas paths. Missing or ambiguous topology does not invalidate observed
consumption, but must not be replaced with guessed path provenance.

Production tests cover mixed local/shared consumption, exact branch amounts,
monthly-to-annual aggregation, one multiplier application, missing/NULL/duplicate
observations, known zeros, DirectOnly behavior, stored JSON and VRF coexistence.
Independent original-IDF/SQL proofs cover owners, recipients, central service
paths, source-local budgets, native precision and actual-frequency requests.
Original physical identity and executed-file object indexes are bound separately.

The corrected production tests pass in 27.344 seconds, including exact source
rosters, all owner/port ambiguity checks, and two JSON reloads. Candidate 03
materializes in 20.250 seconds. Its complete 17,275-check diagnostic passes all
eight groups with zero numeric/contract failures and zero coverage gaps in
113.205 seconds. Compared with candidate 02, all 2,925,313 numeric fields are
identical; only provenance changed.

## Reviewed approval and saved acceptance

Root explicitly approves independent pending SHA-256
`a4f285b693b4ecf7b476051c1902a785dbe788b21ea4bb83dadb6f42ea7fee7f`.
The reviewed recipe SHA-256 is
`a9189e3b38649a5b722161f7d8d5bd274aab18bbb6b3f9a04a9f4d3e4ae727e2`.
Separate original-IDF/native-SQL arithmetic matches all 510 reviewed references.
The independent registry audit verifies 17,275 unique keys over 91 contexts,
5,863 known zeros, 702 explicit nulls and 5,383 coverage records containing
11,775 required fields and 13,445 required selector bindings. Every provenance
hash matches its actual original/capture/engine/weather/recipe/current-code file.

The manually reviewed header and 140,577-byte lossless companion retain every
independently computed metric. Companion SHA-256 is
`30b07d5b579adc626ba197728882cb1a2ed81cdc8d4b5330ee6a2fcecd01a636`;
uncompressed metric SHA-256 is
`ab6f4e9da1203eba7a4119a8bd843dcc5ee48ebdd95e8b456c08f6803322533e`.
No candidate values were copied into expectations.

All nine prior fixtures pass current-code rebuilding: Large Office 81.909 seconds,
Small Office 19.548, Ideal Loads 31.635, PTAC 24.412, PTHP 25.548, Fan Coil 9.430,
VRF 80.005, Radiant 87.558 and District 21.703. All eighteen prior expected
header/companion hashes remain unchanged.

The first saved check used the wrong snapshot option and therefore correctly
rejected the preserved, pre-fix capture payload. With the correct explicit
`EPATH_REAL_VERIFY_SNAPSHOT` pointing to candidate 03, saved numerical acceptance
and the ten-fixture catalog guard pass in 220.822 seconds. The original capture
was not rewritten to manufacture a pass.

Full repository verification passes: app 22.408 seconds, CLI 5.700,
frontend/browser 181.959 and simulation 465.731; input, IDF and tabular tests
pass from cache. The production Windows build passes in 7.927 seconds.
Normal commit-hook verification/build and push finish this checkpoint. Nine
other engineering fixtures remain unapproved; this approval is not completion
of checklist section 22 or the goal.
