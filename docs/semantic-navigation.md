# Semantic Navigation Contract

This document defines the product and implementation contract for bidirectional
navigation between Semantic Text and every analysis panel. New panels and new
semantic projections must follow this contract instead of introducing a
panel-specific click or jump model.

## Product model

### Left Semantic Text

- Preserves IDF objects and fields as source anchors.
- Lets users read relationships between IDF objects as zone, profile, service,
  and output flows.
- Acts as a navigation index from a semantic entity to its other views.
- Displays a projection, while every available source anchor continues to map
  to the original IDF.

### Right Analysis Panels

- Summary, Profile, HVAC, Output, Simulation, Diagnose, and Geometry are
  specialized lenses over the same semantic entities.
- A target selected in a panel must be able to return to the most appropriate
  occurrence in Semantic Text.
- Simulation results remain observations about the analyzed model. They link
  to canonical model entities and output sources, but do not become canonical
  Semantic Text entities themselves.

The two sides share one primary selection. A panel may retain local display
context such as a graph scope, active story, or filter, but it must derive its
selected model entity from the global selection or synchronize it through the
common controller.

## Vocabulary

- **entity**: A stable, model-level identity such as a zone, surface, schedule,
  HVAC component, output request, or diagnostic. An entity can be presented in
  more than one place and view.
- **occurrence**: One contextual presentation of an entity in Semantic Text,
  such as a coil in a zone service path and the same coil in a plant loop.
- **source anchor**: The best available stable source-object identity plus an
  optional field identity and object-index fallback that locates original IDF
  text without defining semantic identity.
- **selection**: The single committed global semantic entity, its chosen
  occurrence and source anchor, and the view context from which it originated.
- **hover**: A transient indication of an entity and related occurrences. It
  never scrolls, changes tabs, or enters history.
- **reveal**: Making a target visible inside an already active view without
  changing views. Reveal alone does not enter history.
- **open**: Moving to a target's appropriate view and revealing it as one
  atomic navigation operation.
- **history**: Lightweight snapshots of user-initiated context moves. History
  restores both semantic and panel context, never reports, graph data, caches,
  rendered HTML, or other large derived values.
- **definition**: The canonical source definition of a referenced object.
- **references**: Contextual occurrences that refer to the selected object.
- **edit**: An explicit source or semantic operation that changes the document;
  selection is remapped after the new projection is available.

Semantic identity must be stable whenever source information permits it.
`objectIndex` is only a last-resort identity input and a source-jump fallback.
Navigation never changes model text and never starts analysis merely to move
between already-current views.

## Navigation actions

Action names are part of the public implementation vocabulary. Function names,
data attributes, adapter operations, telemetry, and tests use these terms.

| Action | Meaning | May change tab | May scroll | History |
|---|---|---:|---:|---:|
| `hover` | Temporarily highlight the entity and related occurrences | No | No | Never |
| `select` | Set the global primary selection | No | Only reveal a compatible visible counterpart | Once when the entity changes |
| `reveal` | Make a target visible in the current view | No | Yes | Never by itself |
| `open` | Move to the appropriate view and reveal the target | Yes | Yes | One atomic entry |
| `reveal_source` | Show the original IDF object or field | Input view only | Yes | One entry |
| `definition` | Move to a referenced object's definition | Input view only | Yes | One entry |
| `references` | Cycle occurrences that reference the object | Input view only | Yes | One entry |
| `clear_selection` | Clear the global selection | No | No | Never |
| `edit` | Explicitly change a field or apply a semantic operation | No | Restore after apply | Document history only |

Existing entry points such as `focusInputObject`, `selectHVACGraphKey`,
`navigateHVAC`, and panel-specific focus functions remain compatibility
wrappers while migration is in progress. Their navigation effects must route
through the common selection controller, and one user action must not create
duplicate history entries.

## Pointer and keyboard contract

Every semantic line and selectable panel item follows the same interaction
model unless an explicitly documented accessibility constraint requires a
different physical gesture.

| Input | Result |
|---|---|
| Hover | Weakly highlight the same entity and related occurrences on both sides; do not scroll, switch tabs, or record history. |
| Single click | Commit the global selection. Reveal only if the currently visible counterpart can represent it; never switch to another result tab. |
| Double click or `Enter` | Open the occurrence's preferred view and reveal both sides as one atomic history operation. |
| `Alt+Enter` | Open the available-view target menu. |
| `F12` | Go to definition. |
| `Shift+F12` | Cycle references. |
| `Alt+Left` / `Alt+Right` | Navigate view history backward / forward. |
| `Esc` | Close a transient popover first; otherwise clear global selection while retaining filters, graph scope, and active tabs. |

Single-clicking an editable value only selects it. Editing begins through an
explicit edit affordance or `Enter`/`F2` while the value control has edit
focus. A semantic line click and its edit-button click are distinct actions,
and double-click never begins direct text editing.

Mouse and keyboard paths must expose equivalent operations. Panel-specific
exceptions are not permitted unless they are documented here together with an
equivalent accessible operation.

## Linked selection and follow selection

`Linked selection` defaults to **on**. `Follow selection` is a separate setting
that also defaults to **on**. Both are visible in the workspace link bar and
persist in workspace snapshots.

When Linked selection is on, selection and highlights are shared between
Semantic Text and panels. When Follow selection is also on, a committed panel
selection scrolls Semantic Text to a compatible occurrence, and a Semantic
Text selection scrolls or focuses a compatible target in the active panel. It
still does not automatically switch result tabs.

When Follow selection is off, the counterpart receives the shared highlight
and exposes a **Reveal** action without changing the user's scroll position.
Turning Linked selection off suppresses cross-view propagation while preserving
the committed selection so it can be relinked without losing context.

## Canonical panel targets

The backend projection supplies all applicable view targets and chooses a
preferred target from occurrence context. The frontend consumes those targets;
it must not duplicate this table as a large object-type switch.

| Semantic context | Preferred view | Canonical target |
|---|---|---|
| Building or site summary | Summary | Summary section |
| Zone or space geometry | Geometry | Zone or space ID |
| Surface or fenestration | Geometry | Geometry object ID |
| Zone profile dimension | Profile | Zone plus dimension |
| Profile group | Profile | Group ID |
| Schedule definition or use | Profile | Schedule identity |
| HVAC service path | HVAC | Path ID |
| HVAC loop | HVAC | Loop ID |
| HVAC component | HVAC | Component ID |
| Supporting coupling | HVAC | Coupling ID |
| Output request | Output | Output signature or source object |
| Simulation-purpose output source | Simulation or Output | Result/source ID |
| Diagnostic occurrence | Diagnose | Diagnostic ID |
| Raw or source-only occurrence | Input source | Source anchor |

A zone name is not forced to one panel: it advertises every supported
Geometry, Profile, HVAC, Output, and Diagnose target. Occurrences beneath
`zones/<zone>/profiles`, `zones/<zone>/services`, and
`zones/<zone>/geometry` prefer Profile, HVAC, and Geometry respectively. When
multiple valid targets or occurrences remain, the UI offers a chooser rather
than inventing a relationship or silently choosing an unrelated context.

## Reveal, filtering, and analysis lifecycle

Selection never destroys a semantic mode, facet, search filter, panel filter,
graph scope, or active tab. If a selected occurrence is hidden, reveal
temporarily materializes the containing section and exempts only the target.
Clearing the temporary reveal restores the user's unchanged filters.

If the current text hash matches the report analysis key, navigation uses the
existing report and navigation index and makes zero analyzer calls. If text is
stale, raw source reveal may still locate the source immediately. Semantic or
panel open records a pending target and reports that analysis is pending; the
navigation action itself does not start a full analysis. After the normal
analysis lifecycle produces a current projection, the pending target is
applied once without adding history.

## History contract

History records user context moves: entity changes, preferred-view opens,
explicit tab changes, definition/reference jumps, input-context changes caused
by source reveal, and graph focus-scope changes. It does not record hover,
temporary highlight, scrolling, tooltip/popover state, filter typing,
resize/pan/zoom, render refreshes, or selection restoration.

An `open` operation records exactly one snapshot even though it may update
selection, switch a tab, and reveal both panes. Back and Forward restore the
global selection, semantic occurrence and filters, active result tab, and the
compact local context of each panel.

## Baseline regression gaps

The migration is complete only when automated acceptance coverage prevents all
of these baseline problems from returning:

1. `SemanticYAMLLine` previously exposed object/field indexes and edit metadata
   but no stable semantic entity ID or panel target.
2. Semantic line selection previously updated `semanticSelectedObjectIndex`
   and raw-text synchronization but not right-panel selection.
3. Generic right-panel input jumps previously relied on
   `data-jump-object-index` and could not choose a contextual semantic
   occurrence.
4. View history previously stored input/result tabs and source object/scroll
   but not Profile, HVAC, Geometry, Output, or Simulation context.
5. HVAC, Profile, Geometry, and Simulation previously held rich independent
   selection state without a common global entity selection.
6. Basic Semantic Text previously hard-truncated at 250 lines, so later
   occurrences could be absent from the DOM and unreachable.
7. A target hidden by mode, facet, or filter previously could not be revealed
   reliably without disturbing the user's view state.

Each gap has an acceptance test at the backend, controller, adapter, or end-to-
end layer appropriate to the behavior. Adding a new panel requires only a
registered navigation adapter, canonical target metadata, standard selectable
markup/actions, compact context capture/restore, and the same shared tests; it
must not add an independent selection model or navigation event protocol.
