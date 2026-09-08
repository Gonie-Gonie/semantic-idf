# Energy Path implementation progress

Execution order follows `semantic_idf_energy_path_refactor_checklist.md` supplied
by the user. This ledger records implementation checkpoints; it does not replace
the checklist or its final acceptance scenarios.

## Checkpoints

- EPATH-001–110: prior committed implementation, through `c19fcc5` (canonical
  carrier taxonomy). Final end-to-end acceptance is still pending.
- EPATH-111: facility reconciliation and supply context implemented. Carrier
  accounting is regenerated from consumption splits, including stored v2 and
  monthly-first annual results. Positive gaps exceeding either 2% or 0.01 kWh
  gain an optional Unclassified energy branch; all signed gaps remain in the
  inspector. Charge stays consumption while generation/discharge/purchases/
  sales stay supply context. Supply-only source IDs do not contaminate closure.
  Verification: `TestEPATH111*` backend, SQL/output-plan, round-trip and browser
  tests; full repository verification is required by the commit hook.
- EPATH-120: allocated thermal links verified against monthly-first load
  closure, including Other/storage. Stored results retain independent raw,
  effective and allocated zeros, refresh known driver contributions, and only
  reconnect missing links when the thermal service/scope target is unambiguous.
  Conversion temporal overlap is unchanged. Verification: `TestEPATH120*`,
  existing `TestEPATH080*`/`TestEPATH081*`, and driver-allocation browser checks.
- EPATH-121: matching cooling/heating conversion links retain their measured
  dual values and exact temporal traces across reloads. Ratio classifications
  are regenerated from retained carrier splits; invalid service/domain/unit
  crossings are rejected. Verification: `TestEPATH121*`, the full simulation
  package, and conversion browser checks.
- EPATH-122: all exact legacy contributors are retained when their taxonomy
  endpoints merge. Stored carrier splits use the period-local carrier value at
  both ends, keep branch-local provenance, and rebuild end-use display totals.
  Invalid carrier removal is explicit partial context, not a new conversion
  allocation. Verification: `TestEPATH122*`, the full simulation package, and
  end-use/carrier browser checks.
- EPATH-123: presentation-only Other grouping is implemented with strict <1%
  thresholds, protected semantic endpoints, original node/member/source IDs and
  inspector-only Expand. Verification includes threshold/context/closure browser
  tests and the real Tools CSV export path (identical bytes before/after grouping).
  Full repository verification is required at the commit checkpoint.
- EPATH-130: v2 quality now separates run-level Drivers, Loads, End uses and
  Carriers availability from selected-period closure and conversion-ratio
  availability. Endpoint-weighted absolute discrepancies cannot cancel, and
  building-wide zone coverage retains direct/allocated and explicit unassigned
  shares. Stored quality and sibling summaries are rebuilt, including empty
  not-requested results; v1 serialization is unchanged. Verification includes
  `TestEPATH130*` stage/sentinel, period, allocation and round-trip regressions.
- EPATH-131: compact stage quality sits below the graph, with a normally closed
  Data details drawer for availability, reconciliation and diagnostics. Its
  internal Output tab resolves exact run-plan requests by type/name/key/frequency
  and never trusts a stale object index alone. Source filters preserve explicit
  stage and Zone scope while retaining required derivation traces. Verification:
  `TestEPATH131*`, full frontend checks, and interactive browser review of stage
  filtering, All stages, exact Monthly request selection and Escape focus return.
- EPATH-140: Energy now has one primary view, without the old subnav or duplicate
  summary tables. Selected-node inspectors retain exact Series and HVAC jumps;
  multiple targets require an explicit choice. SQL Series retain dictionary,
  frequency and type metadata, while old ambiguous identities remain unresolved.
  Month jumps use actual timestamps, reset the visible panel range, and display
  single-point values. Scope/period/service/selection/drawer and Series panels
  survive navigation history; old Zone/Sources snapshots migrate to current
  state. Acceptance passed before removing old subnav/active handlers/state.
  Disconnected old renderer functions remain until the planned EPATH-220 cleanup.
  Verification: real SQL duplicate-frequency fixtures, strict identity/calendar
  browser tests, actual dashboard navigation/history/keyboard acceptance, unchanged
  raw/export inputs, and interactive browser review of February Series and return.
- EPATH-141: four compact, clickable KPI cards retain scope/period totals across
  Service changes. Exact graph targets are selected or explicitly chosen; hidden
  targets reveal their service without changing scope or period. Selected-service
  load/site ratios use paired conversion-link values, with partial-overlap labels.
  Coverage shows two separate closure boundaries, not a legacy scalar score. As
  coverage has no energy node, it opens accounting details and returns focus to
  its own card on Escape. Missing values stay unknown; explicitly reported zeros
  survive zero-node pruning without invented navigation targets. Verification:
  pure KPI-value tests, actual dashboard scope/service/month/click/focus tests,
  light/dark contrast and compactness checks, and interactive browser review.
- EPATH-142: one bounded four-column canvas now replaces the separate stage,
  conversion and auxiliary card stacks. Direct uses begin in the end-use column;
  carriers are shared once, and thermal counterparts remain highlight-only.
  Native node buttons retain two-line labels, full accessible titles and quality
  badges. Quantitative ribbons are intentionally the next EPATH-143 step.
  Completed-run setup collapses once but remains reopenable; failed/running
  setup stays available, and manual disclosure choices survive result browsing.
  Compact setup/status, KPI widths and whitespace keep the whole default view
  within the actual 1600x900 application viewport with the normal 50% editor.
  Verification: immutable pure geometry, twelve material drivers and nine direct
  or residual rows, actual-index 24-node hit-testing, matching Building/Zone
  geometry, full-view scroll checks, setup/focus lifecycle and screenshot review.
  Existing 131/140/141 navigation, drawer and KPI acceptance also passed.
- EPATH-143: thermal and site values now use separate linear quantitative scales,
  with same-domain ribbons and dual-value tapered equipment bridges. Reported
  node values are not inflated to fit mapped ports; actual From/To values and
  source identities are retained. Accessible ratio labels explain the numeric
  load/site ratio and independent scales. The legend has exactly four entries.
  Invalid, contradictory duplicate and overflowing links are excluded explicitly;
  tiny values remain linear without a fabricated minimum ribbon width.
  Verification: immutable pure geometry, COP/efficiency taper direction, partial
  overlaps, dense layouts, actual SVG hit geometry, keyboard tooltips, explicit
  versus absent ratio-quality markers, Building/Zone viewport fit and screenshot
  review. Existing navigation, KPI and drawer acceptance also passed.
- EPATH-144: a fixed, theme-aware semantic palette distinguishes cooling blue,
  heating red, restrained driver/end-use categories and stable carrier colors.
  Allocated values retain their hue with diagonal hatching and accessible corner
  markers; positive carrier residuals are neutral and crosshatched. Mixed member
  provenance reads Includes allocated, including after automatic Other grouping.
  No color customization controls were added. Labels, values and small badges
  use existing theme variables with readable contrast; geometry is unchanged.
  Verification: pure appearance and provenance tests, actual light/dark contrast,
  non-color markers, unchanged raw/quantitative data, stable scope/period/service
  colors, Building/Zone fit, screenshot review and related frontend regressions.
- EPATH-145: node taxonomy and both ends of ribbon port stacks now have fixed
  ordering independent of values or input order. Selection follows ancestors and
  descendants separately, so shared drivers/carriers do not activate unrelated
  branches. Non-flow counterpart cards keep their outline without activating
  their bars or links. Unrelated paths dim to 25%; focused labels remain readable.
  Separate transparent link hit areas and native conversion buttons expose exact
  selected-link values, units, ratio and basis, with source details closed.
  Enter/Space, Escape, blank-canvas clearing and monthly history preserve focus
  and original data. Verification: pure topology/port geometry, actual 1.37px
  ribbon hit-testing outside its fill, directed path/counterpart selection,
  monthly link details/history, fixed viewport and default/selected screenshots.
- EPATH-150: every node/link inspector now shares seven ordered sections:
  representation, value, breakdown, calculation basis, related model entities,
  closed Source data and actions. Source/rule IDs and raw formulas stay in Source
  data; grouped contribution values remain expandable. Friendly exact model
  labels and existing Series/HVAC/counterpart actions are retained, including
  source-backed monthly Series navigation from selected links.
  Independent raw/effective/allocated values never borrow annual source scalars.
  Original driver signs are combined individually; ambiguous directions remain
  unavailable. Missing multipliers may be derived only from consistent, known
  same-period original values. Explicit nulls remain unknown and real allocated
  driver zeros survive new and stored JSON without inventing sparse values.
  Breakdowns use typed period-local components, exact matching Zone graphs,
  carrier splits, allocation evidence and supply/reconciliation context.
  Verification: pure model/metadata, Go zero-presence round trips, actual
  25-node/four-link-type seven-section acceptance, source visibility, monthly
  Series return, unchanged raw data, native pane fit and screenshot review.
- EPATH-151: driver actions now provide category-specific Topology, Profile and
  verified outdoor-air HVAC destinations through the global selection controller.
  Building aggregates expose explicit Zone/source-group choices, never an
  arbitrary first surface. Matching physical sources and unquantified model
  context are distinguished; mixed connections and shared interzone air links
  do not acquire fabricated per-target energy contributions. Exact report
  targets, Zone ownership and Profile source anchors are checked against the
  current semantic navigation index, again when a destination is activated.
  Physical entity targets avoid duplicate semantic field-context choices; native
  disclosures keep multiple choices compact. Air-coupling navigation exposes the
  selected edge in the Topology air metric. Each jump records one Energy return
  point, preserving scope, month, service, selected driver and detail state.
  Verification: pure category/provenance/identity tests, real analyzed-IDF schema
  bridge, actual Topology/Profile/HVAC adapter round trips and air-edge visibility,
  stale-target rejection, no Analyze/Run calls, unchanged raw data and screenshots.
- EPATH-152: load and equipment actions now resolve verified HVAC service paths,
  connected systems and AirLoop/PlantLoop targets; carriers open their exact
  facility-meter Output request. Building aggregates offer explicit, ranked Zone
  choices and unquantified shared-system context. Dedicated physical owners
  remove duplicate Zone aliases without guessing among ambiguous targets.
  Exact service, Zone, period, semantic targets and Output request identity are
  revalidated on activation. Request frequency and HVAC type/service distinguish
  same-name choices. Zone-only source context does not fabricate path evidence.
  Load actions open a matching Zone Heat-Flow Ledger with actual calendar bounds
  and category-major frames; the ledger is labelled context, not load provenance.
  HVAC/Ledger jumps record one return point; the local Output drawer restores
  focus to its chosen action on close. Ledger controls and charts now fit the
  actual result-pane width, including the focused inspector.
  Verification: immutable pure evidence/identity tests, vendored Large Office
  analysis-to-semantic schema bridge, 19 actual destination round trips, monthly
  and hourly request disambiguation, stale-target rejection, no Analyze/Run calls,
  exact history restoration, actual pane overflow checks and three screenshots.
  Existing 140/150/151 navigation and inspector regressions also passed.
- EPATH-160: Energy now has exactly six primary state fields. Drawer tab,
  stage and source belong to local panel context; related model focus uses
  global navigation rather than parallel Energy focus settings. Strict pure
  migration reads old workspace aliases, preserves pending selections without
  a result, and writes only the new primary fields and nested drawer context.
  Unused legacy controls and their state writes are removed; old renderers remain
  isolated with fixed defaults until EPATH-220. Loaded results validate stale or
  ambiguous selections/sources, including dormant Energy restored behind HVAC.
  Schema-4 workspace snapshots hold an exact input-hash/run-ID reference, not
  the result payload. A bounded desktop-process cache returns original wire JSON
  without SQL reads, migration or reaggregation. Cold Settings/Batch returns
  restore both result and context; misses remain empty without automatic runs.
  Input/save/cache-response races, older run completions and same-text new files
  cannot attach or evict the wrong result. Diagnose starts work only when opened,
  and edits/file replacement invalidate its saved simulation reference.
  Verification: pure typed/legacy migration, exact-wire and concurrent Go cache
  tests, HTTP and real run-entry integration, eight actual cold main documents
  through Settings/Tools, dormant HVAC/Energy and Output Escape focus restoration,
  auto-run suppression with a positive control, edited/newer-run response guards,
  compact snapshots, new-file reset, lazy Diagnose and existing Energy regressions.
- EPATH-161: Header, KPI, Graph, Inspector, Quality and Details now have separate
  render stages. A single current scene owns the scoped projection, all-service
  KPI targets, layout, ribbons and controls. Selection updates only existing
  focus attributes and the inspector; the drawer has its own update boundary.
  Cards, bars, ribbons, ports, ratio tooltips and hit targets retain DOM identity
  and exact geometry. Re-selecting a node preserves its open source/chooser
  disclosures; changing model metadata refreshes the inspector without layout.
  The scene excludes selection/drawer keys, invalidates actual result/context
  and payload replacements, and deliberately does not cache older scopes yet.
  Model navigation indexes are prepared at activation, not on the first click.
  Opaque driver indexes preserve duplicate, source-anchor and target validation;
  selected evidence and destination authority are still checked on activation.
  Verification: pure scene lifetime/invalidation and prepared destination
  equivalence tests; actual native-clock 1600x900 app with untrimmed analyzed
  Large Office metadata (19 Zones, 2,020 semantic entities, 64 service paths).
  All 19 measured selections eagerly rendered all seven inspector sections and
  actions within 50ms (max 43.5ms, p95 35.6ms); first wall selection was 31.7ms.
  No projection/layout/ribbon computation occurs on selection, drawer changes
  or a same-context workspace repaint. Scope/month/service/result changes rebuild.
  Existing navigation, cold workspace, layout and inspector regressions pass.
  The older virtual-clock focus test now deterministically finishes native
  opacity animations before final-style assertions; production transitions and
  the new native-clock response-time acceptance remain unchanged.
- EPATH-170: Batch Energy comparison is fixed to Building / Annual and shows
  Drivers, Loads, End uses, Carriers, Ratios and Residual / coverage in one
  six-column baseline/target table. Rows match typed semantic categories, never
  run-local node/source IDs or labels. Missing, invalid and duplicate categories
  remain unknown; an explicit zero is retained. Units and scale domains guard
  subtraction, and basis differences and source-coverage changes are visible
  beside the comparison. Coverage deltas use percentage points.
  A selected model opens one separate, locally owned Energy Path detail, with
  its own selection, inspector, KPI and Data / Output drawer. It does not replace
  the main workspace result or expose model actions against unrelated metadata.
  Duplicate run IDs cannot silently resolve to a different model. Non-Energy
  purpose comparisons remain available when their Energy payloads are empty.
  Verification: pure immutable/nullability/category/unit/coverage boundary tests,
  actual Go v2 summary-to-browser schema tests, and the actual Tools bootstrap
  with 100 result rows, single-model detail, keyboard/focus and Output navigation.
  A separate cold Comfort-only document preserves its purpose chart and zeros.
  Compare/detail navigation makes no additional analysis or simulation calls.
  Native 1600x900 screenshots of both the comparison and selected-wall detail
  were inspected. The capture helper fixes the viewport before navigation and
  verifies visible panel bounds rather than accepting a blank screenshot.
- EPATH-171: the disconnected Batch edge/ranking renderers and their styles are
  removed. Current v2 Excel exports default to exactly Energy Path Summary,
  Energy Path Delta, Data Quality and Runs. The unchecked, Excel-only Include
  trace sheets option adds canonical node/source/link evidence, link deltas,
  availability/reconciliation/warnings and the original submitted Energy JSON.
  Default sheets contain readable categories, not raw node/source/link IDs.
  Workbook rows snapshot the same pure comparison used by the UI; Go validates
  the submitted run indexes, summary context, selected pair and row bindings
  without implementing another semantic matcher or delta calculation. Nullable
  numbers retain unknown versus explicit zero; basis/coverage warnings and
  each side's units survive the request and workbook boundaries. Stale or
  ambiguous comparison IDs cannot silently select another pair. Missing v2
  export projections fail before the save dialog rather than falling back to
  the old numeric exporter. Existing non-Energy exports and the established
  genuine-v1 read/export compatibility path remain until EPATH-222.
  Original Energy JSON trace chunks retain unknown fields, nulls and Unicode
  within Excel's cell limit. Carriage returns are XML-escaped so opening the
  workbook cannot normalize away original JSON line endings.
  Verification: 100-run immutable pure projection parity, actual Tools
  checkbox-to-Save JSON-to-Go-to-XLSX round trips, default/trace sheet isolation,
  raw JSON reconstruction, nullable values and comparison warnings, explicit
  unit mismatch and backend invalid-request/legacy-read regression tests.
- EPATH-180: `docs/energy-path-schema.md` documents the actual v2 Go wire
  contract, four stages/two scales, independent link quantities and ratio
  direction, monthly-first signed driver allocation, multiplier handling,
  policy-specific Zone allocation, source correspondence and static-factor
  limitations. Requested-source availability, period-local ratio availability
  and accounting closure have separate definitions. Legacy thermal residuals,
  additional retained periods, sparse values and source-period limitations are
  explicit rather than silently simplified away.
  A complete JSON example and standard-library Python reconstruction preserve
  canonical graph identity, quantities, units and provenance without claiming
  identical browser pixel layout or implementing another v1 migration.
  Verification: JSON keys checked against real Go wire tags; the exact Python
  fence executed on the example, actual 100/25 cooling and 85/100 heating
  conversions, source correspondence, thermal residual compatibility and the
  frozen v1 golden after the real Go reader. Wrapper, monthly, missing-month,
  Zone/source ownership and forbidden cross-scope fallback cases pass.

- EPATH-181: a read-only `energy-path` command, desktop `LoadEnergyPath` method,
  local `/api/energy-path` endpoint and standard-library Python HTTP/stdio
  methods share the existing v2 builder and one projection/CSV formatter.
  Building/Zone, Annual/M1-M12 and All/Cooling/Heating selection is explicit.
  The original GUI `purposeResults` stays unchanged beside the selected view;
  service filtering retains full carrier and scope-period quality context.
  CSV defaults to summary/quality, with source/link rows only on trace opt-in.
  Unknown counts/percentages are blank, explicit zeroes survive, and source
  units plus independent conversion quantities are preserved.
  The loader binds exact SQL/input identities to genuine run metadata, rejects
  hash mismatches and ambiguous/unreadable outputs, and never substitutes a
  sibling SQL. Missing historical plans expose unknown requested coverage in
  the shared builder rather than inventing output expectations. Provenance is
  explicit outside the canonical graph. SQLite is read-only; unsafe WAL/journal
  states are refused without writes. CLI output cannot truncate the original
  SQL/model or run metadata, including hardlink/symlink aliases.
  Canonical reconciliation/warning ordering is stable across repeated reads.
  Verification includes actual IDF/epJSON and SQLite loads, strict GUI/App/HTTP/
  CLI/Python JSON equality, exact default/trace CSV, selected scope-period
  quality, invalid requests, unchanged source files and unchanged desktop cache.
  The Python clients execute in real subprocesses; model text is not parsed or
  energy data aggregated in Python. See `docs/energy-path-cli.md`.

- EPATH-182: the main Energy Path Data drawer now exports HTML, XLSX and the
  original full-run JSON from one immutable snapshot of the existing UI summary
  and quality helpers. The first HTML section / XLSX sheet is Energy Path:
  scope/period, Drivers, Loads, End uses, Carriers, Ratios and Quality. The
  all-service summary is explicitly distinguished from the graph's service
  emphasis; saved-result context does not claim to include current editor edits.
  Source/link identifiers and detailed rows are behind closed HTML Trace or
  five opt-in XLSX trace sheets. Source and normalized units, independent link
  quantities, unknown values, explicit zeroes and original Unicode/CRLF JSON
  survive export. Zone direct/allocated basis and building-wide unassigned
  accounting remain explicit. Batch v2 starts with its scope/period context;
  genuine-v1 compatibility remains isolated until EPATH-222.
  Go validates the captured raw graph and formats the supplied presentation
  DTO without normalizing or aggregating the saved result. Workbook output
  cannot overwrite input, SQL or run metadata, including aliases. Exporting
  does not analyze, run, reread SQL, change selection or relayout the graph.
  Verification: actual Data buttons to captured HTML/JSON and real XLSX ZIP
  cells, every summary/quality cell, Zone accounting, trace quantities/units,
  default/trace isolation, async run changes, malformed requests, nullable
  values, injection escaping and raw JSON reconstruction. Focused 171/182/Batch
  backend regression and 182/141/142 frontend/cross-jump regression pass.
  Fresh-profile Building/Annual and Zone/M1 HTML screenshots were inspected
  for readable hierarchy and long-label layout (synthetic browser fixtures,
  not an actual EnergyPlus simulation claim).

- EPATH-190–191: a new real-IDF/synthetic-SQL acceptance fixture checks exact
  forward primary relation endpoints in every returned Building/Zone/period
  graph, both HVAC services and all ten non-HVAC categories. Direct end uses
  have no primary load input; correspondence stays non-flow. The existing
  exact 100/25 COP and 85/100 efficiency test was verified after that test,
  including two JSON reloads. See `docs/energy-path-acceptance.md`.
- EPATH-192: twelve signed wall-output months exposed two zero-as-missing
  metadata fallbacks. Prepared source raw/effective net 0 now survives beside
  distinct cooling/heating contributions 38+38. Unambiguously source-owned
  directional allocations sum independently of the signed net; shared-source
  evidence cannot invent complete attribution. V2 source/Zone-detail JSON
  retains known zeroes, stored sparse/null values remain unknown, and the v1
  writer is unchanged. All twelve monthly graphs, annual sums, two scopes,
  source accounting and repeated JSON reloads pass; the full simulation
  package and existing API/export regressions also pass.
- EPATH-193: existing Zone/ZoneGroup multiplier source-semantics and monthly
  aggregation acceptance tests were checked and executed after EPATH-192.
  They cover native versus already-multiplied outputs and repeated preparation
  without applying the multiplier twice. No new multiplier logic was needed.
- EPATH-194: a new parsed-IDF/analyzed-Topology plus synthetic-SQL acceptance
  fixture covers 13 surface sources, including Door/GlassDoor, InternalMass,
  OtherSide variants and an unresolved key. Exact real entity/connection
  ownership, category, units and raw values survive canonical build and reload.
- EPATH-195–198: existing concrete numerical assertions were inspected and
  executed in order for all five duplicate-prevention families, cooling/heating
  driver closure (including zero pressure and simultaneous loads), mixed-fuel
  end-use/carrier splits, generation/storage non-double-counting, and direct/
  path/fallback/unassigned Zone plus AirLoop/PlantLoop auxiliary allocations.
  Each checklist bullet and its test evidence is mapped in
  `docs/energy-path-acceptance.md`; no duplicate aggregators were introduced.
- EPATH-200: a new actual-app browser acceptance test captures fresh defaults
  before fixture setup, exercises the conditional Zone search input/datalist,
  Annual plus all twelve months, and Cooling/Heating/All in both scopes. It
  checks absence of legacy controls, retained control focus, immutable results
  and zero Analyze/Run calls. Independently passed in 3.855 s; no production
  control change was needed.
- EPATH-201: existing actual-app geometry/ribbon/taxonomy/Other/dimming
  assertions pass. A new trusted-keyboard test traverses all 47 graph stops
  forward and backward and activates node/ratio/SVG targets with Enter and
  Space, clearing with Escape. It checks focus-only readability, complete
  ratio tooltips, unchanged graph DOM/geometry/calculation counters, immutable
  results and zero analysis/simulation calls. Independent and three-repeat
  runs pass; no production layout change was required.
- EPATH-202: exact actual-DOM carrier/end-use split tests now cover Building
  and Zone, Annual and January, with independent expected quantities, friendly
  names and site units. Facility reconciliation and supply/storage context
  cannot enter consumption rows; Sources remains closed and selection retains
  graph DOM. Existing common inspector and actual destination/chooser tests
  also pass. The new discharge fixture uses an explicit canonical identity.
- EPATH-203: quality acceptance now compares missing versus not-requested on
  the same Drivers stage and checks all three direct/allocated/unassigned
  visible percentages. Existing four-stage, filtered Data/Output, Zone-period
  coverage, unknown-value and exact source-navigation assertions pass. No
  production quality logic was changed.
- EPATH-204: control changes now enter the existing Back/Forward history once,
  before mutation. Invalid/disabled, same-value and typing events preserve
  selection, drawer, redo and graph DOM; exact control/node/ratio/edge focus
  reuses the existing snapshot target ID. Tests cover all six primary fields
  and the complete drawer, including the exact Output source/request. A real
  retained-semantic-Zone regression exposed final global reveal overwriting a
  restored Building context; a narrow history-restore guard fixes that without
  changing explicit destination navigation. Both actual-app tests, a parsed-IDF
  semantic test and existing destination/Settings/Batch/cache regressions pass.
  A response-only removal of the guard reproduced the exact old failure.

## Policy clarification

EPATH-030's earlier 11-node cap conflicts with EPATH-123's stricter <1% grouping
rule when a Zone has twelve material categories. The current implementation
preserves the material categories rather than forcing one into Other. A user
preference question was sent for this conflict; the default follows EPATH-123.
The normal Building interzone/category projection remains intact.

## Next in sequence

Section 22 actual-model fixtures → 210–211 → 220–222 → 230–235.

Section 22 is underway, not complete: 19 byte-preserved official model inputs
cover the required equipment/model types and 22.1/23.2/24.2/25.1 Large Office
versions. Structural assertions and independent SQL-oracle unit tests pass.
The first actual Large Office capture exposed and reproduced unnecessary
hourly graph construction and cross-Zone output-key fallback; focused fixes
and regressions pass. A fresh annual run completes through the shared builder
with all four stages and 19 Zone projections. All 19 selected engine executions
have now finished: 18 normal results and one no-heating model rejected for two
Severe errors. Saved SQL calendar/annual-scalar compatibility corrections pass
focused tests and four independent read-only rechecks. Eight-group expected
manifests and the no-heating convergence resolution remain required. Fresh
Large Office saved replay now verifies the corrected 12/18, 9/14, 9/10 and 2/2
stage counts and actual exact-key discovery. Real annual parallel-link ID
collisions are fixed without losing branches or relaxing projection guards.
The simultaneous fixture's six exact time-aligned SQL measurements also pass
an automated positive/negative contract. The isolated no-heating hybrid-solver
trial retained its two Severe errors and was not accepted. Evidence,
isolation, engine provenance and explicit capture-versus-acceptance distinctions are in
`docs/energy-path-real-models.md`.

The next real-result pass also preserves tiny positive conversion ratios and
uses explicit locale-aware `<0.01` presentation instead of zero. Actual replay
and browser/regression checks pass. Independent Large/Small Office source
recipes are present, but the first 17,469-metric Large Office diagnostic is
rejected: monthly sensible/latent reconciliation rows have colliding IDs, and
the interval/presence/complete-coverage acceptance guards remain unfinished.
These are explicit next tasks within section 22, not deferred later items or
approved fixture expectations. No numeric expected manifest is approved.

Follow-up raw-wire guards now reject missing/null required quantities before
compatibility decoding. The initial preserved-candidate presence check passed;
the later `found <= total` invariant correctly rejects that older candidate's
Zone 9/0 and 2/0 counts. An independent topology audit also identified false July/August
heating allocation to unserved plenums and discarded, already-reported per-loop
fan energy pools. These must be fixed and checked against their exact connected
recipients; the unreviewed fallback numbers are not approved expectations.

The reconciliation-fixed real candidate eliminates the shared structural
blocker: a second 17,469-check diagnostic fails with 218 remaining discrepancies,
down from 13,150. Loads, carriers, completeness and carrier residual comparisons
have no mismatches in that run. Explicit bounded presentation intervals retain
tiny positive source centers and unknown/zero distinctions. Known-topology
zero-load allocation and derived native-Zone raw metadata corrections now pass
focused regressions; actual post-fix verification and the eight-group manifest
remain required. The complete chronological evidence is in the real-model doc.

A third actual diagnostic includes 255 independent per-loop fan checks and
still rejects 262 of 17,724 checks: 46 scoped reported-zero fields, 208 fan
carrier-selector mismatches and eight wholly pruned zero/tiny allocation-row
fields. Derived raw quantities, loads, end uses, carriers, all 52 ratios,
completeness counts and carrier residual comparisons have no mismatches in
that candidate. This is not acceptance. Scoped proof, canonical fan selectors
and bounded whole-row presence corrections are undergoing regression tests;
full verification also caught and prompted a fix for omitted IdealLoads mixed
service recipients. Exact original-wire decoding now prevents compatibility
repair from concealing runtime defects. A fresh complete diagnostic and full
repository/Wails verification remain required before the next checkpoint.

The next original-wire diagnostic reduces these to four fan Zone-month
rounding failures; all other 17,720 checks pass. Largest-remainder apportionment
now bounds each Zone's own rounding error while preserving the full pool,
instead of accumulating earlier rounding errors in the final Zone. Dedicated
ordering/zero-weight tests and existing allocation/v1 regressions pass. The
independent expected centers and intervals were not changed. The new original-wire
candidate diagnostic now passes all 17,724 checks in 58.151 seconds; the separate
report retains `acceptance: false`. Required complete coverage and approved
eight-group expected manifests across all model types remain unfinished.
A full repository test and Wails build passed immediately
before this final bounded rounding fix, and verification is repeated for the
checkpoint.

The post-rounding-fix full `scripts/verify.ps1` now also passes, including all
actual frontend tests, the simulation suite and Wails production build
(10.101 seconds). The first passing Large Office numeric diagnostic and these
regressions form an intermediate checkpoint; section 22 remains active.

The user's subsequent 89% report is reproduced from the exact combined
Basic Energy + Surface Zone Heat Flow capture, with the original files
preserved and no engine rerun. The extended silent post-processing interval
is reduced from 168.525 to 118.878 seconds; complete numeric/metadata/source
membership equality passes. Actual phase and elapsed-time feedback no longer
rerender existing results, and the 132 MB result displays in the real frontend.
See `simulation-postprocessing-performance.md` for the bounded evidence and
the remaining profile-guided work. This correction is not section 22 acceptance.

The immediate profile-guided alias lookup follow-up reduces that same combined
bundle to 60.826 seconds (from 168.525 seconds), without changing any exact
numeric token, metadata or provenance membership. Fixed catalog indexes retain
the original first-match and caller-owned slice contracts. Focused regressions
and the full actual-file comparison pass. Fresh full repository verification
and the Wails build also pass after correcting a native-clock browser test;
the SQL/feedback and alias improvements are combined into one checkpoint.

The saved run's initial SQL reader also exceeded its existing 20-second limit
and discarded completed SQL sections before CSV/ESO fallback. Extending the
verified compact walker to the initial readers reduces the original complete
SQL parse from 30.023 to 12.101 seconds; the default-limit path completes in
12.146 seconds. The full 1,797,121-byte SQL result is byte-identical to the
preserved original unlimited parser. The actual source-selection replay takes
11.818 seconds, retains SQL Series/Heat Flow exactly and does not invoke ESO
fallback. The capture remains unchanged. This bounded optimization preserves
the existing timeout/error policy; it is not a general partial-result policy
change or section 22 numerical acceptance.

The next section 22 checkpoint adds independent Driver-to-Load, per-Zone HVAC
and direct lighting/equipment proofs, including exact SQL ownership, monthly-
first allocation, source provenance and multiplier checks. Real-model
compilation now requires a full-record coverage ledger: unchecked fields or
missing obligations prevent group approval even when compiled numbers pass.
The fresh `large-office-25-1-candidate-coverage-01.json` and original-wire
`large-office-25-1-diagnostic-coverage-05.json` pass all 31,928 numeric/contract
checks in 29.930 seconds, but the diagnostic intentionally fails for 12,127
remaining coverage gaps across 13,996 records. Drivers and Loads have no gaps;
the remaining obligations concern reconciliation, Zone subtotals and per-context
quality/status evidence. This is not acceptance or an approved expected
manifest. The exact group counts and preserved failed-attempt distinctions are
documented in `energy-path-real-models.md`. No engine rerun, source capture
change or production change was needed for this test-only checkpoint.
Cross-review regression tests reject sum-preserving duplicate primary nodes,
misplaced/missing contributor sources and cosmetic duplicate branches. Annual
endpoint completeness remains distinct from each branch's completed-month
provenance, avoiding invented temporal overlap while retaining active-month
source checks.

Checklist section 22's actual-model fixtures and expected manifests also remain
required between EPATH-204 and EPATH-210; they have no individual EPATH numbers.
The final completion definition in section 26 is part of acceptance as well.

The following checkpoint closes the Large Office full-record coverage gaps.
Independent thermal/site reconciliation, carrier-specific consumption and fan
branches, Zone carrier subtotals and all nine scope-period quality fields now
pass together. This catches and fixes two production defects: Zone allocation
coverage was calculated from a fan/pump-only subset, and allocated fan carrier
branches lost their exact AirLoop pool source. The oracle separately corrects
legitimate selected-Zone load-context provenance and case-insensitive EnergyPlus
Zone identity, without changing numeric centers or weakening coverage gates.
The new `large-office-25-1-candidate-coverage-02.json` is generated from preserved
SQL in 20.110 seconds; `large-office-25-1-diagnostic-coverage-07.json` passes
46,224 numeric/contract checks and all required fields in 13,996 ledger records
in 40.480 seconds, with zero failures and zero gaps across all eight groups.
It remains a diagnostic, not approved expected-manifest acceptance; section 22
continues with explicit manifest review and the remaining actual model types.
The subsequent complete replay also passes and creates a separately marked
pending-review artifact from independent metrics, never from copied candidate
expectations. A test-only saved acceptance option now reuses a SHA-bound
original-wire snapshot, but still requires a separately approved catalog
manifest; normal tests and passing diagnostics cannot create that approval.

Large Office 25.1 now has its first explicitly approved expected manifest.
Its 394,508-byte lossless companion preserves all 46,224 independent metrics,
including exact zeros and explicit unknowns; the review header binds the pending
artifact, original SQL, reviewed recipe, compared candidate and production code.
The separate saved-original-wire acceptance test passes in 72.320 seconds with
all eight groups, full metric/key/count/status comparisons and complete record
coverage. No simulation rerun or source capture changes were needed. Ordinary
offline tests now also require this approved artifact's integrity. The other
18 catalog entries remain unapproved; Small Office is next, with independently
reviewed exterior-floor classification and exact fan/direct-use source work
still to apply. Later checklist sections are not marked complete by this pass.

Existing code may already satisfy portions of later items. They remain pending
until their exact checklist requirements and regression/acceptance evidence
are checked in order. In particular, old renderers/state are removed only after
the new view's acceptance tests pass.

The next section 22 checkpoint approves Small Office 25.1 after correcting the
actual DX/fuel AirLoop service path omission and the `NoReheat` name-based
misclassification. Unique continuous supply branches and exact terminal demand
membership now provide the conditioning evidence; a two-loop/two-terminal Zone
regression prevents cross-assignment. Independent SQL checks cover exterior
Floors, explicit nonmember Zones, seasonal Load / fuel ratios, counted fan-pool
rounding stages and exact whole-month driver-owner/source choices. Original
inputs, engine output, SQL and failed diagnostics remain intact.

All 17,675 independent metrics and required fields across 5,074 coverage records
pass repeatedly. Separate reviews verify the immutable pending artifact,
42 monthly-to-annual family sums, 156 carrier sums and 91 original-SQL Core
calculations. Explicit root approval creates the second expected header and its
146,896-byte lossless companion, retaining actual zeros, explicit unknowns and
tiny positive unassigned pump energy. Saved-original-wire acceptance passes in
17.040 seconds. The unchanged Large Office approved expectation also passes
46,224 metrics in 76.340 seconds after exact redundant-owner search pruning;
the original visit bound and rejection of fractional-month assignments remain.
The offline integrity guard now requires both approved fixtures. Ideal Loads
is next; 17 catalog entries and all later checklist acceptance remain pending.

Ideal Loads preparation now exposes and fixes native Building/HVAC/Plant meter
subtotals being counted as additional Other consumption. Their original source
values remain in inspector context. The actual original-SQL rebuild restores
site closure from 42.383% overmapped to 100%, retaining reported annual district
consumption and its absence from Monthly graphs. A dictionary-only inference
that Ideal Loads had no site consumption was corrected against the original
utility tables and official EnergyPlus 25.1 source before implementation.

The independent source recipe and strict annual Tabular reader preserve source
display precision, unique report/cell provenance, actual zero versus missing/NULL
and full-year Weather checks. Annual frames, original-source proofs, site flows/
residuals, Building/Zone services, carrier subtotals, quality and required-field
coverage are now integrated. Tabular sources never acquire fake dictionary IDs
or Monthly quantities; strict absence proofs distinguish unavailable Monthly
district consumption from the independently reported Monthly electricity.
Monthly-source annual sums remain unchanged.

Twenty exact original Monthly Ideal Loads Supply Air Latent Energy/Rate sources
are independently bound to their five actual equipment owners. Existing
EPATH-071/092 non-additive inspector context is retained without changing the
selected sensible load or allowing context to replace its numeric source proof.
Preserved Ideal Loads diagnostic 01 reports 6,504 failures over 16,993 checks
(2,276 numeric/contract and 4,228 coverage); diagnostic 02 reports 1,246 over
17,073 checks (480 numeric/contract and 766 coverage). Twelve explicitly reviewed
Hourly reconciliation aliases now have independent complete 8,760-hour calendar
and Monthly-equivalence proofs. Their source-only per-point normalization bound
does not alter Monthly numeric authority or allocation precision. The resulting
diagnostic 03 passes all 17,121 checks and required fields across 4,047 coverage
records in 15.252 seconds, with zero failures or gaps in all eight groups.

Diagnostic 04 repeats the complete pass in 16.396 seconds and writes only a
pending-review artifact. Independent reviews verify its exact provenance,
17,121 unique keys, 5,809 actual zeros, 2,328 explicit nulls, 455 count pairs and
all 8,594 required coverage fields. Separate direct original-SQL arithmetic
compares 3,135 core expectations across all 78 Zone-period contexts, retaining
the actual plenum load, five equipment owners and annual-only district energy.
Root explicitly approves the reviewed expectation header and its lossless
131,365-byte companion; saved-original-wire acceptance passes all 17,121 metrics
in 28.405 seconds. The offline integrity guard now requires this third approved
fixture. The other 16 fixtures and later checklist acceptance remain pending.
No production code or original simulation output changed in this oracle pass.
Normal full verification and Wails build remain the required commit gate.

The final normal `scripts/verify.ps1` passes: app 17.388 seconds, frontend/browser
checks 148.242 seconds, simulation 164.133 seconds and Wails production build
6.333 seconds. No test is excluded and no hook or latency threshold is bypassed.
Windows Go package/cache finalization is slow between child processes, but the
same verification invocation reaches a successful exit. The resulting executable
is `build/bin/semantic-idf-v0.4.4.exe`. PTAC (`DOAToPTAC.idf`) is next in section 22;
its existing capture remains unapproved and its source recipe is not yet authored.

The preceding checkpoint's first full verification stopped at the unchanged EPATH-161
50 ms selection limit: refreshing metadata for the same selection took 51.6 ms.
All backend packages passed, but the hook did not build or create a commit.
Response-only timing instrumentation then identified unnecessary whole-model
driver navigation preparation after a shallow report-wrapper replacement:
23.8 ms of a 32.9 ms handler in the isolated pre-fix measurement. A deterministic
no-rebuild assertion fails before the correction even when timing is under the
limit. The minimal correction excludes only that wrapper from the navigation
cache context; inspector refresh and actual model/projection/hash invalidation
remain required. No latency threshold, native clock or DOM-retention check is
relaxed.

The isolated post-fix run passes in 8.339 seconds: that metadata handler takes
8.7 ms with zero navigation rebuilds, and all 19 original timed selections remain
under 50 ms (maximum 44.5 ms). Nine actual model/projection/navigation/analysis-key
replacement cases also require exactly one fresh preparation and reuse on the
following same-node click. Independent diff review confirms the original
synchronous seven-section visibility, graph DOM retention and no-backend-work
assertions remain intact. The subsequent full verification and Wails build passed,
and checkpoint `cd50293` was committed and pushed: frontend 148.960 seconds, app
19.563 seconds, simulation cached and Wails 7.462 seconds. That completed gate
does not replace the pending full verification of the newer annual integration.

The later EPATH-211 cache audit must also exercise same-node selection after
in-place replacement of nested model collections. The pure destination resolver
already invalidates replaced collections, but the mounted inspector currently
observes their parent model references. Normal analysis replaces those parents;
this narrower wrapper correction does not claim broader in-place mutation
support or mark that later audit complete.

Section 22 PTAC preparation first passed 16,765 independent checks against the
preserved September 7 capture, but that fallback-only result was not approved:
EPATH-094/100 requires exact directly owned HVAC consumption where it exists.
The original `DOAToPTAC.idf` owns five distinct DX/fuel PTAC pairs; the source RDD
and MTD expose their main, crankcase and ancillary consumption separately.

The normal output plan and SQL reader now request and bind all 25 reviewed
Monthly/J coil inputs through the original whole-model PTAC/EquipmentList/Zone
ownership. Shared, central, duplicate, broken or ambiguous ownership is rejected.
Native DOAS mixer references are allowed only with exact connected air nodes.
Constituents are additive and already model total, without a second Zone/List
multiplier. Missing, NULL, negative or duplicate Monthly data cannot leave a
partial main/ancillary cohort claiming complete direct energy. Actual source
zeros remain observed; independent annual building meters remain authoritative.
The mixed local/DOAS broad fan pool remains explicitly unassigned because no
individual fan or measured fan-pool observations support a defensible split.

The new annual run `real-ptac-25-1-20260908T081443.664544900` completed engine and
result generation, preserving the original and annual-input hashes. Its SQL hash
is `37372221dba622dc0e71b5574edba740f316fd973ea6fbcb9655311053674090`.
Separate read-only arithmetic confirms all 300 component-month observations,
25 exact MTD memberships, cooling/fuel meter closure, 65 owned Zone-period
contexts and 130 paired conversion ratios. Original warnings remain visible.
The obsolete fallback-only oracle correctly rejected 4,312 checks; that failure
is preserved, not described as a passing capture test or fixture acceptance.
The new independent component-source, direct-first service and exact provenance
proofs are being integrated before any PTAC expected values may be approved.

Prior-fixture rebuild checks also exposed a test-only native/wire boundary:
the strict independent reader correctly requires explicit scalar presence, but
the rebuild path had skipped the production v2 writer that emits observed zeros.
The saved rebuild seam now serializes the actual v2 bundle, performs the strict
wire preflight, and uses the original-wire decoder. It does not infer zero for
absent/NULL data, repair a candidate through the production compatibility reader,
or change the frozen v1 writer. Focused and full verification remain pending for
this current integration checkpoint. Approved fixtures remain 3 of 19; PTAC and
the other 15 entries, plus later checklist acceptance, remain unapproved.

The new independent PTAC proof now passes all 17,021 metrics and required fields
in all eight groups (diagnostic 03, 12.802 seconds). Diagnostic 02 is retained:
its 1,230 failures comprise 30 exact-zero source-scope observations, 390 carrier
reconciliation identities and their 810 coverage consequences. Original global
source zeros were not missing; only the proof for an independently owned,
exact-zero, explicitly serialized coil may reuse that observation when pruning
left no scope detail. General missing/NULL/foreign-Zone source behavior is not
relaxed. Fully directly observed Zone carriers use their independently proved
plain monthly/annual reconciliation identities, without accepting arbitrary IDs.

The direct-first compiler preserves carrier-qualified remainders, known-zero
owner exclusion and exact main/ancillary source cohorts. Positive consumption
with no load has separate relation-specific monthly/annual proofs. A narrow
original-SQL 3-decimal presentation proof resolves the actual 0.110/0.110 heating
classification boundary; raw numerical centers/bounds stay unchanged, and exact
positive displayed pairs cannot be pruned by broader raw intervals. The combined
new source/service/consumer/presentation/wire regressions pass in 18.446 seconds.
All three earlier approvals also pass current-code saved-original rebuilding:
Small Office 17,675 metrics in 22.199 seconds, Ideal Loads 17,121 in 35.815 seconds,
and Large Office 46,224 in 90.177 seconds. No original capture, expected metric or
production source was changed to achieve these oracle-boundary corrections.
PTAC approval and the normal full verification/build gate are still pending;
the tiny displayed allocation overlap warning is being reviewed separately before
this fixture is marked complete.

The allocation wording now explicitly describes rounded displayed totals and
positive constituent-period overlaps, which need not equal the net annual
residual. It does not diagnose physical excess from the displayed discrepancy
alone or dismiss genuinely excessive observations. Overlap/gap arithmetic,
severity, status and the actual unassigned fan energy are unchanged. The Zone
quality line uses the allocation-specific English/Korean label without renaming
other overmapped statuses or adding primary controls. The ledger/message tests
pass in 0.625 seconds and the actual EPATH-131 drawer/navigation browser regression
passes in 2.918 seconds, including unchanged 87.767/12.233 coverage values.

The final wording-only production rebuild produces a distinct SHA-bound PTAC
candidate 03 without modifying the original capture. Diagnostic 04 passes all
17,021 independent metrics and mandatory fields in all eight groups (12.229
seconds), then exports pending-review artifact 01. That export is deliberately
unapproved and not acceptance; explicit review and saved expected-manifest
acceptance remain separate gates. Earlier failed and passing diagnostics are
preserved rather than overwritten.

Root explicitly approves the separately reviewed PTAC pending artifact
`6a29ae0c042f16c34b6e4d8604f501d59662a8bdeac376f0cade075728e8837d`.
The hand-authored expected header and 140,629-byte lossless companion retain
all 17,021 independent expectations. Saved-original-wire acceptance passes
all eight groups in 22.239 seconds; the offline integrity guard now requires
this fourth approved fixture. Actual original SQL/MTD review confirms all
25 coil identities, 300 monthly observations and meter membership, complete
known-zero constituents, carrier closure below 2e-12 kWh and unchanged original
model/annual-run controls. Approval is limited to result transformation for this
controlled fixture: its original 5,278 warnings remain, including cold-condenser
and freeze-risk warnings. It does not approve the model's climate suitability.
The previous three approved headers and companions remain unchanged.

Section 22 now has 4 of 19 approved catalog fixtures. PTHP is next; the other
15 entries and later checklist sections are not complete. The full repository
verification and Wails build for this implementation checkpoint are next.

The first full verification completed with three failures: the existing raw-text
missing-value scanner misread a new comment's `main/ancillary` substring as the
forbidden sentinel, and two EPATH-100 assertions still expected the superseded
allocation label. The comment now says `main and ancillary`; the two label
assertions and matching negative-scope label guard use the reviewed new wording.
No numerical expectation, threshold, status, warning severity or scanner rule was
relaxed. The sentinel regression passes in 0.208 seconds and all EPATH-100 audit
plus overlap regressions pass in 0.670 seconds. A distinct candidate 04 binds the
comment-only new production digest before replaying the unchanged approval;
candidate 03, its pending review and approved payload remain preserved.
Candidate 04 also passes all 17,021 saved acceptance metrics in 21.041 seconds
against the unchanged approved expectations. Full verification is rerun next.

The corrected full `scripts/verify.ps1` passes: actual frontend checks in 162.757
seconds, simulation tests in 252.096 seconds, all other packages successful,
and the Windows production Wails build in 10.283 seconds. This is the completed
PTAC implementation/acceptance checkpoint; normal commit-hook verification and
push follow. The next PTHP source audit is read-only so far and confirms that
its old capture lacks the required direct coil-consumption outputs. It needs a
typed DX/Fuel ownership extension and a new normal output capture, not copied
PTAC observations or fallback-only approval.

The PTAC checkpoint was committed and pushed as `fab1650`; its normal commit hook
also passed full verification and Wails build. PTHP is now the fifth separately
approved fixture. It requests 35 typed native-coil consumption identities and
independently verifies all 420 monthly observations, including real heating-DX,
defrost and crankcase electricity plus supplemental fuel and ancillary energy.
Shared output names are resolved against exact original typed ownership;
ambiguous reporting identities and incomplete carrier cohorts cannot become
direct observations. Component energy is already model total, and mixed-fuel
heating uses Load / site energy with separate carrier allocation ledgers.

The new engine succeeds with 24,523 retained warnings and no Severe/Fatal, while
its initial capture test's 103 native/wire mismatches remain preserved as failures.
A test-only original-v2 boundary copy fixes observed-zero source verification
without rewriting captures or treating absence/NULL as zero. Saved diagnostic 01
then isolates three actual-zero July gas reconciliation differences. An exact
independent whole-row proof now follows existing production zero-node/orphan-row
pruning, while rejecting positive balanced totals, partial fields and invented
Monthly zeros. Existing metric identities, raw centers and precision are unchanged.
No production reader/writer/pruning behavior was altered for these test boundaries.

Diagnostic 02 passes all 17,113 checks and required fields in eight groups.
Separate original-SQL-to-pending arithmetic reviews 2,870 assertions; the explicit
approved header and lossless companion retain 5,819 known zeros and 806 typed
nulls, with 4,927 coverage records in 91 contexts. Saved acceptance plus the
five-fixture integrity guard pass in 23.915 seconds. All four prior approvals
also pass current-code rebuilding unchanged: PTAC 23.473 seconds, Small Office
19.073, Ideal Loads 30.827 and Large Office 82.972.
Detailed source/provenance and failed-versus-passing evidence are recorded in
[PTHP acceptance](energy-path-pthp-acceptance.md). Section 22 is 5/19, not complete;
fan coil with plant loops is next, followed by the remaining catalog and later
checklist sections. The normal full verification/build gate is next.

The PTHP checkpoint's full `scripts/verify.ps1` gate passes without exclusions:
app 21.138 seconds, CLI 6.247 seconds, frontend/browser checks 166.044 seconds,
simulation 288.174 seconds and all other packages successful. Windows production
Wails build passes in 7.474 seconds. Normal commit-hook verification and push
follow. The next Fan Coil preparation is read-only: it has three terminal fans,
two shared district-energy plants and no individually requested fan/pump energy.
Water-coil heat transfer is not direct purchased consumption; the new source
recipe and measured auxiliary ownership require their own next-fixture work.

The PTHP checkpoint is committed and pushed as `efe5540`; its normal commit hook
also passed the full verification/build gate. Fan Coil is now in progress, **not
approved**. Original SQL/IDF review binds separate cooling/heating water-coil
plants and preserves unassigned local fan energy. Native typed path binding fixes
the former PlantLoop-by-service Cartesian product without requesting heavy
auxiliary outputs. Original-value precision regressions also expose and remove
an artificial -0.001 kWh internal pressure at NORTH ZONE in August, retaining its
real 0.033 kWh delivered heating load as load-only Other/storage. Missing, NULL,
small-real-signal and multiplier guards pass without relaxing SQL expectations.

Independent zero-pressure and allocated-pump proofs are now covered by focused
tests. The first complete 8,966-metric saved diagnostic remains a failure because
the fallback's Monthly period metadata is incorrect; graph context/field checks
are not bypassed. [Fan Coil work evidence](energy-path-fan-coil-acceptance.md)
distinguishes every failed attempt from later required acceptance. Section 22
remains 5/19 until the actual result, all prior approvals and the separate reviewed
expected/saved-acceptance gates pass. Later checklist sections remain pending.

Fan Coil diagnostic 04 now passes all 8,966 independent metrics and required
fields in eight groups (4.65 seconds). A strengthened native regression first
reproduces the missing Monthly fallback period; the bounded correction preserves
the actual load period without changing quantities, allocation rules or source
identities. Separate original-SQL arithmetic verifies 1,934 pending keys and 124
annual/monthly sums. Independent registry/provenance review retains 3,087 known
zeros, 373 explicit nulls and 260 count pairs, with 2,728 records in 52 contexts.

Root explicitly approves pending SHA256
`6324f440b8b608cd1d4c32fb2a5e7eff0352d82c1848cb083ece6db2f611e55d`.
The manual expected header and 74,133-byte lossless companion preserve all values
and statuses. Saved-original-wire acceptance passes all 8,966 metrics in 8.07
seconds, and the offline guard now requires six approved fixtures. All five prior
approvals pass current-code rebuilding with unchanged expected artifacts: PTHP
25.215 seconds, PTAC 23.331, Small Office 18.877, Ideal Loads 30.675 and Large
Office 82.207. Section 22 is now **6/19**, not complete; VRF is next, followed by
the remaining fixtures and sections 23–26. Full repository verification/build
and normal commit/push are the next checkpoint gates.

Fan Coil's full `scripts/verify.ps1` passes without exclusions: app 21.187 seconds,
CLI 5.765, input 1.580, frontend/browser 167.646, IDF 7.098 and simulation 307.114;
tabular succeeds from cache. Windows production Wails build passes in 7.706
seconds. Normal commit-hook verification and push follow. Next-fixture VRF
preparation remains read-only: the original MTD places VRF crankcase electricity
in Cooling and defrost in Heating, so PTHP's heating-crankcase membership cannot
be copied. Its own source/output/ownership review is required before a recipe.
