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

## Policy clarification

EPATH-030's earlier 11-node cap conflicts with EPATH-123's stricter <1% grouping
rule when a Zone has twelve material categories. The current implementation
preserves the material categories rather than forcing one into Other. A user
preference question was sent for this conflict; the default follows EPATH-123.
The normal Building interzone/category projection remains intact.

## Next in sequence

EPATH-150–152 → 160–161 →
170–171 → 180–182 → 190–198 → 200–204 → 210–211 → 220–222 → 230–235.

Checklist section 22's actual-model fixtures and expected manifests also remain
required between EPATH-204 and EPATH-210; they have no individual EPATH numbers.
The final completion definition in section 26 is part of acceptance as well.

Existing code may already satisfy portions of later items. They remain pending
until their exact checklist requirements and regression/acceptance evidence
are checked in order. In particular, old renderers/state are removed only after
the new view's acceptance tests pass.
