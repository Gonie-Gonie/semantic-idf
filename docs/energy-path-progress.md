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

Checklist section 22's actual-model fixtures and expected manifests also remain
required between EPATH-204 and EPATH-210; they have no individual EPATH numbers.
The final completion definition in section 26 is part of acceptance as well.

Existing code may already satisfy portions of later items. They remain pending
until their exact checklist requirements and regression/acceptance evidence
are checked in order. In particular, old renderers/state are removed only after
the new view's acceptance tests pass.
