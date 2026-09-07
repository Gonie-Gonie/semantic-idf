# Energy Path acceptance evidence

This record follows the numbered acceptance checks in the Energy Path refactor
checklist. Existing assertions are reused when they already prove the exact
requirement; a similarly named test or a successful render alone is not proof.
New tests close identified gaps rather than duplicate the production builder.

Backend tests below live in `cmd/semantic-idf/internal/simulation`. Browser
fixtures and actual EnergyPlus model runs are separate forms of evidence:
synthetic graph/SQL fixtures do not claim a successful EnergyPlus simulation.
Actual-model manifests, performance checks, cleanup and end-user acceptance
remain required after these unit and frontend checks.

## Backend checks (EPATH-190–198)

Each item below records the tested numerical or structural contract and the
assertions that enforce it. Frontend and actual-model acceptance remain separate.

### EPATH-190 — graph direction

`TestEPATH190CanonicalPrimaryDirectionsAcrossEveryScopeAndPeriod` loads a real
IDF and synthetic monthly SQLite outputs through `BuildPurposeResultBundle`.
It checks Building and both Zones, their top-level Annual graphs and every
returned period. Each primary relation must connect its exact adjacent stages:
driver → load, load → end use, end use → carrier. Missing/duplicate IDs and the
forbidden carrier → end-use or load → driver directions fail explicitly.
Both HVAC service conversions and all ten non-HVAC Building categories must
actually be present; Zone lighting/equipment must have carrier outputs and no
primary load input. Source correspondence is separately checked as non-flow.

Evidence: `energy_path_direction_epath190_test.go`; focused `^TestEPATH190`
passed independently in 1.991 s. Existing direction/source-correspondence
regression also passed (0.178 s). No production changes were needed.

### EPATH-191 — independent conversion quantities

The existing
`TestEPATH121GeneratedDualValueConversionsAndTracesSurviveTwoReloads` asserts
cooling FromValue=100, ToValue=25, COP=4 and gas heating FromValue=85,
ToValue=100, efficiency=0.85. It checks exact ratio kinds, thermal/site domains,
both units and source identities, then repeats after two JSON reloads.

Evidence: `energy_path_conversion_epath121_test.go`; the exact test passed
after EPATH-190 (0.586 s). Its numerical assertions already cover every
EPATH-191 bullet, so no duplicate calculation or redundant test was added.

### EPATH-192 — monthly-before-annual aggregation

`TestEPATH192WallContributionsAggregateTwelveMonthsBeforeAnnual` parses an IDF
wall and twelve signed SQL observations through the canonical SQL reader and
builder. The original wall's annual net is 0; heating and cooling contributions
are each 38 kWh. Every monthly node and link, each service's twelve-month sum,
Building/Zone projection, source ownership and two JSON reloads are asserted.

This uncovered a real bug: two legacy metadata fallbacks replaced the known
source net 0 with a directional magnitude 38. Prepared driver values now bypass
those fallbacks. A source that solely owns its directional driver contributions
retains their exact sum (76 here), independently of its net; shared-category
evidence cannot be used to invent a complete source attribution. The formula
identifies the summed contributions and does not claim a ratio against net 0.
The v2 writer explicitly retains known source/Zone-detail zeroes while keeping
absent/null stored values unknown and leaving the frozen v1 writer unchanged.

Evidence: `energy_path_monthly_aggregation_epath192_test.go` and
`energy_path_source_accounting_epath192_test.go`. Focused tests passed (1.610 s),
then the complete simulation package passed uncached (20.801 s).

### EPATH-193 — multipliers exactly once

`TestEPATH051EffectiveMultiplierIndexAndOfficialSourceSemantics` resolves Zone
2 × ZoneGroup 5 = 10, distinguishes already-multiplied Zone System Predicted
outputs (factor 1) from unmultiplied Zone Predicted/heat-balance outputs (factor
10), checks source accounting, then reapplies preparation to prove no double
multiplier. The other EPATH-051 tests retain per-source values, surface ownership
without geometric re-multiplication, and conservative unknown semantics.
`TestEPATH071AcceptanceZoneMonthMultipliersApplyOnceAndServicesStaySeparate`
checks the resulting monthly/Zone/Building loads and separate services.

Evidence: `energy_canonical_pipeline_test.go` and
`energy_building_load_aggregation_acceptance_test.go`. These exact tests passed
after EPATH-192 (0.251 s); no new multiplier implementation was needed.

### EPATH-194 — surface categories and actual Topology identity

`TestEPATH194SurfaceSourcesUseActualAnalyzedTopology` parses and analyzes one
IDF, resolves 13 distinct SQL source keys, and reads the canonical result before
and after JSON reload. It checks exterior wall, roof, ground floor, reciprocal
interzone surface, adiabatic surface, InternalMass, OtherSide wall/roof/floor,
Window, Door, GlassDoor and an unresolved source. Door/GlassDoor must retain the
actual opening entity, window, parent surface and unique Topology connection,
not hand-written entity IDs. Each source remains present exactly once with its
raw value, source/normalized units and owning Zone. Only the deliberately missing
surface may produce the unresolved-source warning.

Evidence: `energy_path_surface_categories_epath194_test.go`; focused test passed
independently (1.111 s). Existing category/aggregation tests remain in place.

### EPATH-195 — duplicate prevention

The following existing numerical assertions were inspected and run after 194:

| Required overlap | Assertion |
| --- | --- |
| Surface detail + Zone aggregate | `TestEPATH060ReviewSurfaceAggregateMismatchIsSeparateBalanceTerm`: aggregate 12 = physical detail 10 + separate residual 2; allocated contributions 83.333 + 16.667 = load 100. |
| Internal families + total gain | `TestEPATH061ReviewInternalFamiliesExcludeTotalsAndExposeAggregateGap` and `TestEPATH061ReviewInternalAggregateAliasesAreNotDoubleCounted`: selected components, excluded context aliases and residual close once to the actual load. |
| Infiltration/ventilation + aggregate outdoor air | `TestEnergyDriverOutdoorAirMismatchIsNamedStorageComponent`: 55 = infiltration 30 + ventilation 20 + named residual 5; the aggregate cannot enter either direct driver again. EPATH-063 fallback/component/sign tests also pass. |
| Energy + Rate | `TestEPATH052SelectionIsFixedByPhysicalFamilyAndEnergyPriority` and `TestEPATH070AcceptanceEnergyAndActualSourceAuthority`: Energy wins, including a reported zero month; Rate cannot fill unobserved Energy months. |
| Zone + system/plant loads | `TestEPATH070AcceptanceHierarchyPreventsCrossLevelDoubleCounting`: both services use the highest available physical tier, with exact fallback values and no lower-tier sum. |

Evidence: `energy_driver_acceptance_review_test.go`, `energy_driver_mapping_test.go`,
`energy_canonical_pipeline_test.go`, `energy_load_selection_acceptance_test.go`.
The focused duplicate-prevention group passed in 0.953 s.

### EPATH-196 — driver allocation closure

Existing EPATH-080/081/120 tests prove cooling and heating closure in each
Zone-month before Building/Annual aggregation. Zero pressure with nonzero load
creates Other/storage, while real raw pressure with zero matching load remains
explicitly unallocated. Simultaneous service loads remain separate (Building
110 cooling / 90 heating in the EPATH-081 fixture), with correct Zone-local
overlap. The EPATH-120 fixture independently retains People raw/effective/
allocated 10/20/60 and zero-pressure Other/storage contribution 7 across reloads.

Evidence: `energy_driver_contribution_allocation_acceptance_test.go`,
`energy_offset_simultaneous_acceptance_test.go`, `energy_path_driver_links_epath120_test.go`,
plus their review tests. `^TestEPATH(080|081|120)` passed in 2.025 s.

### EPATH-197 — end-use and carrier splits

`TestEPATH090CarrierNeutralHeatingPreservesExactCarrierSplitsAndLocalSources`
checks one Heating node 120 with exactly two branches, electricity 70 / gas 50.
The monthly/annual reconciliation test checks exact facility totals and signed
residuals without changing those branches. EPATH-091 checks arbitrary unknown
end uses collapse to Other while retaining values and per-carrier sources.
`TestEPATH111FacilityCarrierReconciliationExcludesSupplyActivities` proves
facility 100 = consumption 90 + residual 10, excluding generation 25 and storage
discharge 8. The SQL supply/storage test also proves charge is counted once,
discharge is not consumption, and support context cannot create a second flow.

Evidence: `energy_end_use_carrier_split_review_test.go`,
`energy_end_use_taxonomy_audit_test.go`, `energy_carrier_reconciliation_audit_epath111_test.go`.
`^TestEPATH(090|091|111)` passed in 0.457 s.

### EPATH-198 — Zone and auxiliary allocation

| Required path | Existing numerical/provenance assertion |
| --- | --- |
| Direct Zone HVAC | EPATH-094 observed-source tests and `TestEPATH100AuditDirectPriorityIsCarrierQualifiedAndDoesNotRenormalizeRemainder`: Office electricity 30 remains direct; gas 10 remains separately allocated. |
| Related service-path share | `TestEPATH100AuditMonthlyExactServicePathsAreBoundedAndOrderInvariant`: Office/Lab monthly 10/30 and 45/15 sum to annual 55/45; unrelated/cross-service paths cannot dilute the denominator. |
| Zone-load fallback | `TestEPATH100AuditFallsBackOnlyToMatchingZoneServiceLoad`: heating 50 splits 10/40; an unrelated cooling load 950 is excluded. |
| Unassigned HVAC | `TestEPATH100AuditTinyUnassignedRemainderStaysBuildingOnly`: direct 24.999 + unassigned 0.001 closes to Building 25; the remainder is not added to the selected Zone. |
| Fan AirLoop | `TestEPATH101AuditFanUsesEachRelatedAirLoopZoneOnce`: 60/40 split, duplicate service paths counted once, unrelated load 1000 excluded. |
| Pump PlantLoop | `TestEPATH101AuditPumpPlantLoopServiceSeparation`: cooling 75/25 and heating 20/80 use only the matching plant/service. |
| Zone sum and reconciliation | The same EPATH-100 direct/path/fallback tests assert per-carrier Zone sums, immutable Building totals, selected-Zone source accounting and explicit reconciliation. |

Evidence: `energy_zone_direct_use_audit_epath094_test.go`,
`energy_service_path_allocation_audit_epath100_test.go`,
`energy_auxiliary_zone_allocation_audit_epath101_test.go`.
`^TestEPATH(094|100|101)` passed in 0.893 s. No new allocation implementation was
needed for these already-asserted requirements.
