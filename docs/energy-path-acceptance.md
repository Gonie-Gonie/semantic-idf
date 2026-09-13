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

## Frontend checks (EPATH-200–204)

Browser tests below live in `cmd/semantic-idf/internal/frontendchecks` and use
isolated, fresh headless browser profiles. Their completed-run fixtures are
synthetic; actual-model acceptance is recorded separately. In-app Browser was
unavailable in this environment, so the repository's browser test harness is
used without attaching to the user's browser or signed-in session.

### EPATH-200 — primary controls

`TestEPATH200ActualAppEnergyPathControlsBrowser` uses the actual application
index, CSS, rendering modules and delegated change handlers. It captures the
fresh module defaults before the fixture sets up a completed result, proving
Building / Annual / All independently of fixture initialization. The Zone search
input and its datalist must be absent in Building, present in Zone, and absent
again on return. It selects Annual and all twelve months, verifies January's
20 kWh cooling load rather than the annual 80, and checks that unreported months
cannot reuse an annual graph. Cooling, Heating and All are exercised in both
scopes. Every rerender must retain focus on its control. Old subviews, node-limit,
sign-mode and allocation controls must be absent. The frozen completed result
is unchanged, with zero Analyze/Run calls.

Evidence: `energy_path_controls_epath200_browser_test.go`; focused test passed
independently in 3.855 s. No production control change was required.

### EPATH-201 — graph layout and interaction

| Required presentation | Numerical or actual-DOM assertion |
| --- | --- |
| Four fixed columns | EPATH-142 uses the actual 1600 × 900 app, editor/splitter and balanced analysis pane. The four stage bounds must increase left-to-right; all 24 material nodes remain visible and hit-testable. |
| Lower direct-use lane | EPATH-142 checks the band begins at End uses, direct nodes lie inside it, and HVAC conversion nodes remain above it. |
| Thermal/site divider | EPATH-142 checks the equipment-conversion boundary lies between Load and End uses columns. |
| Tapered conversion | EPATH-143 checks actual SVG fills at both endpoints against the independent thermal/site scales, cooling narrowing, heating widening and unchanged same-domain width. |
| Fixed taxonomy | EPATH-145 asserts exact ordered driver, load, end-use and carrier lists, including cooling before heating and residual last, independent of magnitude. |
| No overlapping center labels | EPATH-143 intersects actual ratio/node and ratio/ratio bounds, verifies tooltip containment, and retains full accessible labels. |
| Automatic Other | EPATH-123 asserts strict <1% membership, exact-1% preservation, protected categories, scope/month/service isolation, exact carrier branches and immutable source/member identities. Expanding native details cannot increase graph nodes. |
| Selection dimming | EPATH-145 checks directed path opacity 1 versus unrelated 0.25. Source-correspondence cards remain readable while their physical bars stay dimmed; focus alone cannot select a path. |

Evidence: `energy_path_layout_epath142_browser_test.go`,
`energy_path_ribbons_epath143_browser_test.go`,
`energy_path_interaction_epath145_browser_test.go`,
`energy_path_other_grouping_epath123_browser_test.go` and their pure-geometry
regressions. The exact `^TestEPATH(123|142|143|145)` group passed in 16.258 s.
Native keyboard traversal is verified separately from the existing DOM-order
and synthetic-key assertions.

`TestEPATH201ActualGraphTrustedKeyboardBrowser` drives a fresh test-owned browser
with trusted native Tab/Shift+Tab, Enter, Space and Escape input. All 47 graph
stops are traversed in exact forward and reverse order: four node columns,
two native conversion controls, then same-domain SVG edge targets. Conversion
SVG hits cannot introduce duplicate stops. Native activation selects the exact
node/link and opens all five inspector sections; Escape clears without losing
the focused control. Focusing an unrelated dimmed card restores its readability
but not its quantitative bars or selected path. Ratio focus exposes the complete
tooltip without selecting. Event trust, retained graph DOM/geometry, immutable
run/context and unchanged actual projection/layout/ribbon counters are checked.

Evidence: `energy_path_keyboard_epath201_browser_test.go`; independent focused
test passed in 3.102 s, plus three consecutive acceptance runs (7.100 s total).

### EPATH-202 — inspector values and destinations

The existing EPATH-150 actual-app test checks each primary node and link against
the five common inspector sections: representation, value, breakdown,
calculation basis, and actions. Related model entities and Source data are
omitted from node and link details. Driver raw/effective/
allocated values stay independent (including signed heating pressure), cooling
load retains sensible 32 / latent 8, and the cooling conversion shows thermal
40 / site 10 / COP 4. Monthly missing accounting remains unknown, explicit zeroes
remain zero, and annual source scalars cannot substitute for monthly values.

The actual EPATH-151/152 tests activate verified destinations through real
Topology, Profile, HVAC and Output adapters. They check exact selected entities,
visible destination content, one history point and return to the original
Energy context. Aggregate sources require a closed, Zone-grouped chooser, never
an arbitrary first destination. Output selection binds the actual request type,
key and frequency; ambiguous, derived or tabular evidence cannot fabricate an
exact request. Real parsed-IDF destination tests provide separate identity
evidence rather than relying exclusively on hand-built navigation metadata.

Evidence: `energy_path_inspector_epath150_browser_test.go`,
`energy_path_driver_navigation_epath151_browser_test.go`,
`energy_path_service_navigation_epath152_browser_test.go` and their pure-model
and parsed-IDF regressions. `^TestEPATH(150|151|152)` passed in 25.141 s.
Exact rendered carrier/end-use split rows are verified separately below.

`TestEPATH202ActualAppInspectorCarrierAndEndUseSplitsBrowser` checks exact
rendered row keys, friendly labels and site-energy values in Building/Zone ×
Annual/January. Water systems splits into electricity 5 / gas 5 annually;
electricity has exactly seven end-use rows totalling 65, and gas two totalling
105. The other three contexts use independently specified fixture factors
0.25, 0.5 and 0.125, not production split helpers. Facility 68, classified 65
and residual 3 remain separate from consumption rows. Purchased, produced,
storage-discharge and charge context is checked separately in both periods.
This test gives discharge an explicit canonical `storage_discharge` identity;
the old fixture's generic `storage` label did not prove actual discharge.
The five common sections remain, selected-node focus and
graph DOM/paths are retained, inputs remain immutable and Analyze/Run calls are
zero. Independent focused test passed in 4.107 s; no production change needed.

### EPATH-203 — quality UI

The actual EPATH-131 quality drawer test checks the four-stage line below the
graph, partial Drivers versus complete Carriers, and a closed initial Data
drawer. The EPATH-203 extension compares the same Drivers stage with
requested-but-missing output and explicit not-requested output: their statuses
and visible labels must differ, neither may fabricate a percentage, and only
the former exposes its two exact missing-source rows. Missing quality remains
unknown rather than inheriting a global mapped percentage.

Clicking a quality stage opens the filtered Data drawer; source actions select
the exact Output request, with frequency/key/index validation. Keyboard tab
navigation and Escape retain the exact opener. Zone coverage uses the selected
period (annual 82/18 versus January 60/40, absent month unknown), preserves
Building-wide denominator labels, and cannot leak unrelated Zone sources.
The auxiliary allocation test separately asserts direct / allocated /
unassigned totals 50/80/20 over 150 and visible 33.3% / 53.3% / 13.3%, with
unassigned energy kept outside the selected Zone. January's denominator and
overmapped truth are independently checked.

Evidence: `energy_path_quality_epath131_browser_test.go`,
`energy_path_output_requests_epath131_browser_test.go`,
`energy_path_auxiliary_allocation_browser_test.go` and its static contract.
The focused quality/auxiliary group passed after EPATH-202 (4.499 s). Only test
assertions were strengthened; production quality calculations were unchanged.

### EPATH-204 — history and cached restoration

Existing EPATH-151/152 actual destination tests verify Energy → Topology/Profile/
HVAC → Back with the original scope, period, service, selection and drawer.
EPATH-160 traverses actual cold main/Settings/Tools-Batch pages with browser
back-forward caching disabled, restores the exact saved run and six-field
Energy context, and counts forbidden analysis/simulation calls. Cache misses,
stale selections and a changed input cannot invent a current result or rerun
the model. EPATH-161 independently proves selected-node/inspector restoration
in an unchanged cached context without recomputing layout/ribbons; ordinary
node and drawer actions retain quantitative graph DOM.

These existing tests did not prove that scope and period control changes create
history entries. The missing behavior is now covered by
`TestEPATH204ActualAppControlHistoryAndExactFocusBrowser`: actual controls drive
Building → Zone → Back/Forward and Annual → January → Back/Forward, restoring
all six primary fields, the selected inspector and all three drawer fields,
including the exact nonempty Output source/request. The input-first select
transaction records once; a following detached or live change event cannot
erase a new selection, open Sources disclosure or Redo history. Invalid and
disabled values are tested from Zone/January/Cooling, not just default values.
Typing, case-normalized same-Zone commits and no-ops retain DOM, computation
counters, focus and both history stacks. A valid branch after Undo clears Redo
once. Selecting a heating-only month normalizes Cooling → All in that same
transaction, and Back restores Cooling. Exact node/ratio/SVG focus is carried
through the existing snapshot target ID; absent targets cannot select an
arbitrary first node, and inspector actions do not become graph focus targets.

The production handler now validates before applying the existing presentation
helper, compares normalized controls using a temporary six-field state, and
records the shared history snapshot before mutation. No new persistent Energy
state, parallel history stack or aggregation logic was added.

Independent review also found and reproduced a second defect: the final global
semantic reveal could follow a retained Zone after the Simulation adapter had
restored Building, replacing the saved node and focus. The narrowly scoped
restore/preserve-filters guard now keeps the restored v2 Energy context and
refreshes selection styles without navigating again. Explicit user destination
navigation remains unchanged.
`TestEPATH204HistoryPreservesEnergyWithRetainedAnalyzedZoneBrowser` constructs
the real Office semantic entity from a parsed/analyzed IDF and exercises full
Back/Redo for both scope and period with that retained selection. It checks
exact Energy state/focus, unchanged semantic identity and cached result, and
zero Analyze/Run, queued analysis or opened-view side effects.

Both new focused tests passed independently (6.086 s); the exact Output-source
extension also passed (4.611 s). Baseline `^TestEPATH(160|161)` passed in 17.859 s,
and final `^TestEPATH(140|145|151|152|160|161|200|201|202|204)` integration
regressions passed in 57.463 s. A response-only mutation check removed the narrow
restore guard without editing production: the actual-IDF test failed because
Building's selected exterior-wall driver became Office's electricity carrier.
The temporary mutation was removed; the checked-in test uses production code.
The full repository check identified two older static assertions that required
the helper to mutate live state directly. They now require the same local
adapter on temporary state, followed in order by history capture, live assignment
and rendering, and explicitly reject the old pre-snapshot mutation. Both
updated static tests pass; their backend-call prohibitions remain intact.
Multi-context Annual ↔ month layout reuse remains the separate EPATH-211
requirement; these tests do not claim that later cache work is complete.
