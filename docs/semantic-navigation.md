# Semantic navigation contract

Metrics, Profile, and Topology share model identity and selection with visible
Text, JSON, and Table input views. Use this contract when adding selection,
source reveal, panel adapters, or view history.

The Semantic structure view is feature-gated by `SHOW_SEMANTIC_STRUCTURE = false`.
It is not an input tab, shortcut, setting, or restored saved view. Its internal
projection and occurrence index still provide navigation identity. User-visible
source reveal resolves through Text, JSON, or Table.

HVAC and Simulation have local selection. They do not reveal input objects,
follow global selection, or appear in semantic destination menus. Simulation
observations do not become canonical model entities.

## Identity and shared state

| Term | Contract |
| --- | --- |
| Entity | Stable model identity: zone, surface, schedule, component, output, diagnostic, etc. |
| Occurrence | A contextual presentation of an entity, such as the same coil in zone service and plant-loop contexts. |
| Source anchor | Stable source object identity plus optional field and object-index fallback. |
| Selection | One committed global entity, chosen occurrence, source anchor, and origin context. |
| Hover | Transient highlighting only. |
| Reveal | Make a target visible within the active view. |
| Open | Move to the preferred view and reveal as one operation. |
| Definition / references | Canonical source definition / contextual uses of an object. |

Semantic identity uses stable source information whenever available;
`objectIndex` is a last-resort identity input and source fallback. A participating
panel can retain filters/scope/layout, but selected model entities go through
the common controller. Navigation changes no model text and starts no analysis.

## Actions and gestures

Action names are shared vocabulary for controller APIs, adapters, markup, and
tests. Compatibility wrappers such as `focusInputObject` route through the
common controller and suppress nested history. `selectHVACGraphKey` and
`navigateHVAC` affect local HVAC context only.

| Action | Effect | View change | History |
| --- | --- | --- | --- |
| `hover` | Highlight entity/related occurrences, without scrolling. | None | Never |
| `select` | Commit selection; reveal a compatible visible counterpart. | None | Once if entity changes |
| `reveal` | Make the target visible in the current view. | None | Never alone |
| `open` | Select, switch to preferred view, and reveal. | Allowed | One atomic entry |
| `reveal_source` | Reveal source object/field. | Input only | One entry |
| `definition` | Reveal referenced definition. | Input only | One entry |
| `references` | Cycle referring occurrences. | Input only | One entry |
| `clear_selection` | Clear primary selection, retaining local filters/scope/tabs. | None | Never |
| `edit` | Explicit document edit; remap selection afterward. | None | Document history |

| Gesture | Operation |
| --- | --- |
| Hover | Weak highlight, no scroll/tab/history. |
| Single click | Select without switching result tabs. |
| Double click / Enter | Open preferred occurrence/view atomically. |
| Alt+Enter | Available-view target menu. |
| F12 / Shift+F12 | Definition / reference cycling. |
| Alt+Left / Alt+Right | Back / Forward. |
| Escape | Close a transient popover first; otherwise clear selection. |

These gestures apply to selectable navigation targets. Editable values require
an explicit edit affordance or Enter/F2 while the value control has edit focus;
double-click navigation does not begin direct text editing. Accessible
keyboard actions must provide equivalent operations.

Link and follow are always enabled for participating views, without a persisted
user toggle. Committed selection reveals a compatible occurrence/input source
or active-panel counterpart. Following selection never switches result tabs.
Restore/remap transactions can suppress redundant follow work.

## Canonical targets and ambiguity

Backend projection metadata supplies applicable targets and chooses a preferred
target from occurrence context. Frontend adapters consume it instead of
duplicating object-type dispatch rules.

| Context | Preferred destination |
| --- | --- |
| Building/site metric | Metrics section |
| Zone/space geometry, surface, opening | Topology stable entity |
| Zone profile dimension, profile group, schedule | Profile target |
| Output request, simulation-purpose source | Visible input source anchor |
| Diagnostic | Tools / Diagnose diagnostic ID |
| Raw/source-only occurrence | Visible input source anchor |

Zones can advertise both Topology and Profile. Occurrences under
`zones/<zone>/profiles` prefer Profile; `zones/<zone>/geometry` prefer Topology.
Several valid targets/occurrences require a chooser rather than an invented
relationship. Diagnose resolves its current snapshot independently.

Backend metadata may retain HVAC/Simulation targets for identity compatibility;
frontend policy excludes them from reveal/open/follow/destination menus.
Profile and Topology also exclude each other as direct result-panel destinations;
both are independently reachable through input navigation and top-level tabs.
Topology's local projection and emphasis are defined in
[Views and interaction](topology.md#views-and-interaction).

## Filtering and analysis lifecycle

Selection preserves mode, facet, search/panel filters, graph scope, and active
tabs. A hidden target is temporarily materialized within its section and
exempted from filtering; clearing temporary reveal restores the unchanged
user filters.

When text hash matches the report analysis key, navigation uses cached reports
and indexes with zero analyzer calls. Stale navigation queues a pending target
and signals pending analysis; it does not launch analysis itself. After normal
analysis produces the current projection, the pending target is applied once
without another history entry. Document edits remap stable entity/occurrence
and panel context against the new projection.

## History

History records user context moves: entity changes, opens, explicit tab changes,
definition/reference jumps, source reveals, and graph focus-scope changes.
Hover, scrolling, popovers, filter typing, resize/pan/zoom, rendering, and
selection restoration do not create entries.

One `open` creates one snapshot even when it changes selection, tab, and both
panes. Back/Forward restore global selection, occurrence/filter context,
active result tab, and compact panel context. Snapshots must not contain
reports, graphs, caches, rendered HTML, or other large derived values.

## Implementation and regression references

- [semantic_navigation.go](../cmd/semantic-idf/internal/idf/semantic_navigation.go): canonical entities, occurrences, source anchors, and target metadata.
- [selection-controller.js](../cmd/semantic-idf/frontend/src/js/selection-controller.js): shared state and atomic actions.
- [panel-navigation-registry.js](../cmd/semantic-idf/frontend/src/js/panel-navigation-registry.js) and [panel-navigation-policy.js](../cmd/semantic-idf/frontend/src/js/panel-navigation-policy.js): adapter registration and participation rules.
- [semantic-navigation-cache.js](../cmd/semantic-idf/frontend/src/js/semantic-navigation-cache.js): snapshot-keyed lookup maps.
- [view-history.js](../cmd/semantic-idf/frontend/src/js/view-history.js): compact history capture/restore.

Use `dev.bat test -Area navigation`; see [testing.md](testing.md) for plan/full
commands. Focused invariants include `TestSemanticNavigationDoesNotTriggerAnalysis`,
`TestTextJSONAndTableClicksCommitTheSameSemanticSelection`,
`TestSemanticLineClickAndPrimaryOpenHaveSingleHistoryBoundary`,
`TestSemanticTemporaryRevealPreservesUserFiltersAndMode`,
`TestSemanticEditReanalysisRestoresIdentityOccurrenceAndPanelContext`, and
`TestNavigationSelectionUXBrowserHarness`.

New participating panels require registered adapters, canonical targets,
standard selectable actions/markup, compact context capture/restore, and
coverage of these same invariants.
