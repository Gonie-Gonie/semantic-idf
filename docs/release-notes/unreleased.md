# Unreleased Release Notes

<!--
Add release-note entries under the section that best describes the change.
The release script infers bump size from these sections:
- Breaking Changes: major
- Added or Features: minor
- Fixed, Changed, Performance, Security, Documentation, or internal-only notes: patch
-->

## Breaking Changes

- _None._

## Added

- _None._

## Changed

- Made Topology-to-input location synchronization permanent, removed its UI
  and saved setting, and condensed the Topology toolbar to one row. 3D and Plan
  show only Zones, Surfaces, and Openings; Network shows only Metric and Layout.
- Made cross-view semantic selection linking and following permanent, and
  removed the redundant top workspace link/status/navigation bar and its saved
  mode state. Removed the separate Reveal in Semantic command while retaining
  automatic cross-view selection synchronization.
- Replaced the Input View heading, line counter, and auxiliary reveal control
  with Semantic, Text, JSON, and Table tabs that use the same visual and
  accessibility pattern as the analysis tabs.
- Removed the separate Raw Text pane, its splitter, position synchronization,
  editor history state, and delayed-analysis settings. Semantic, Text, JSON,
  and Table now edit one in-memory source document used directly by analysis,
  saving, exports, Profile, HVAC, and Simulation.
- Removed the global Main workspace Back and Forward buttons while retaining
  keyboard/browser history navigation and panel-specific controls such as the
  HVAC Back and Forward actions.
- Renamed the main Summary analysis to Metrics throughout the UI, CLI, API,
  source symbols, tests, and current documentation, and normalized the former
  Geometry panel namespace to Topology while retaining true geometry models.
- Consolidated the desktop/CLI command, Wails application modules, internal Go
  packages, frontend, command tests, and Wails project configuration under
  `cmd/semantic-idf`. Frontend assets are exposed through a dedicated embedded
  filesystem package, while existing commands and `build/bin` artifacts keep
  their paths.
- Simplified the main Simulation view by removing Integrity and Custom Outputs
  purpose controls, the Run Plan display, Advanced run options, and the manual
  EnergyPlus version selector. Allocation, output application, frequency,
  Basic Energy and Zone Heat Flow detail, period, and zone scope now use fixed
  main-view defaults: direct-only allocation, add-missing-only application,
  purpose-default frequency, Heat Drivers, surface detail, the full period, and
  all zones. The view no longer shows a separate section heading, concatenated
  selection summary, or detail/weight tier badges. Backend request options
  remain available.
- Simplified Batch Simulation with the same fixed output-application,
  frequency, detail, allocation, period, and scope defaults. Removed its manual
  EnergyPlus selector, Integrity purpose, advanced result/detail controls, and
  run-plan preview. Each input now automatically uses a registered or detected
  EnergyPlus installation with a compatible major/minor version; an
  incompatible input fails independently without stopping compatible files.
- Fixed Batch Metrics topology values to the multiplier-aware effective area
  basis and removed the Physical/Model-total selector from that workflow.
- Removed Batch Output QA and the main Output result tab. The core Output
  analysis, preview, apply, purpose-output apply, and discovery APIs remain
  available to backend clients and automation. Semantic output entities now
  open their Input/source anchor instead of targeting a removed Output panel.
- Automatically selects a registered EnergyPlus installation compatible with
  the detected model version when possible, and marks Simulation unavailable
  when no usable or version-compatible installation exists.
- Removed the manual Simulation environment refresh action, aligned the
  Weather selector with the application control styling, and placed Run &
  Inspect on the same control row without the redundant idle/blocked status
  header.
- Unified the Topology Level selector across 3D, Plan, and Network: All shows
  the full model and a specific level applies the former story filter. 3D and
  Plan now share Zones, Surfaces, and Openings visibility, while Network shows
  only Metric and Layout instead of Scope or Advanced controls. Selecting a
  zone, boundary, or other topology object emphasizes its one-hop connections
  and strongly fades unrelated objects.
- Unified selected-object details across Topology 3D, Plan, and Network in the
  resizable lower panel. Network no longer reserves a separate right-side
  inspector, leaving the full viewport width available to its diagram while
  retaining connection, boundary, area, UA, exposure, QA, and source details.
- Topology Network nodes can now be repositioned by dragging, with connected
  routes updating in place. Outdoor boundaries use compact directional points
  with separate N/E/S/W/Roof/Floor area and UA totals, while Adiabatic surfaces
  appear as selectable detached wall stubs instead of a shared environment node.
- Moved Topology Fit/Expand, HVAC Fit/Expand, and Heat-Flow plan zoom actions
  into their drawing areas as compact icon controls. HVAC now keeps its
  inspector visible, uses a fixed Focused/Fit graph, and replaces its long
  service/path/medium filter button rows with recoverable dropdowns that remain
  available when no paths match. The separate inspector toggle and graph
  scope/scale presets were removed.
- Removed the redundant HVAC and Profile top headers and text search fields.
  HVAC now uses exactly four compact picker cards: Zone Services,
  AirLoopHVAC, PlantLoop, and Other. The standalone Components and Couplings
  pickers/views, top view switcher, Warnings surface, Current focus card, and
  picker helper text were removed. Loop schematics are again shown directly
  when a loop is selected instead of being hidden in a collapsed detail.
  Ctrl/Meta + mouse wheel now zooms the Loop and Zone Services diagrams around
  the pointer without changing the WebView/application zoom; an unmodified
  wheel continues to scroll the HVAC panel.
  Back, Forward, Clear focus, Zone Services, and Expand now share a horizontal
  icon toolbar at the upper-right of the HVAC drawing instead of a text
  breadcrumb row.
  Apply Profile now sits at the right edge of the compact Profile selector.
- Moved diagnostics and cleanup review out of the main result tabs into
  Tools / Diagnose, where one input snapshot owns both the issue list and fixes.
- Removed the redundant Profile Graph deck-status line such as
  `5 series · actual · year · single`.
- Reworked the Profile overview into a compact, table-aligned selector without
  horizontal scrolling. Profile assignments now show only the zone count with
  full zone details available on hover; Zone rows show the assigned Profile
  name once without a redundant sentence. Each metric label/value is rendered
  in its own aligned cell instead of a slash-separated summary. A click sets
  the primary profile or zone, Ctrl/Cmd toggles additional rows, and Shift
  selects a range. Line views overlay the selected rows with an always-visible
  legend, while annual heatmaps show each selection in parallel panels for
  direct comparison. Removed the separate Profile Matrix, Source Objects, and
  Parameter Candidates sections; graph, Apply, semantic navigation, and
  backend Profile QA data remain available.
- Expanded Profile engineering calculations across People, Lights, all
  standard equipment families, infiltration, ventilation, and design outdoor
  air. Zone, ZoneList, Space, and SpaceList targets now use their resolved
  physical floor-area, volume, exterior-area, and occupant bases; Zone and
  ZoneGroup multipliers are applied once without changing representative
  densities. Schedule curves combine each object's own design contribution and
  schedule instead of applying the first schedule to an aggregate. Unsupported
  weather- or occupancy-dependent operating models are identified as nominal
  or partial rather than presented as exact actual profiles.
- Replaced visible `N/A` sentinels with an em dash. Missing configuration and
  configured-but-incomplete calculations retain distinct status, warning,
  tooltip, and accessibility metadata instead of sharing an ambiguous label.
- Simplified Profile Graph to a fixed Time Profile over selected Profile
  assignments. Removed Graph Type, Scope, and Compare selectors, replaced the
  View dropdown with five direct view buttons, removed the Through / For rules
  view, and moved graph Scale to the Profile Analysis section in Settings while
  preserving legacy view preferences during migration. Added engineering-style,
  view-specific X/Y axes with
  responsive ticks and explicit units; annual heatmaps now separate calendar
  axes from their value color scale. Profile time series are fixed to Actual
  value in their engineering units.
- Removed direct Profile-to-Topology and Topology-to-Profile links, including
  the Profile detail action, Topology Inspector profile summary, and shared
  related-view/link-bar destinations between those two panels.
- Removed obsolete frontend state, renderers, event branches, styles, and
  translations left behind by the simplified Output, Simulation, Profile,
  HVAC, Diagnose, and Topology controls while retaining backend compatibility
  APIs.

## Fixed

- Preserved the live Main workspace while moving through Tools, Guide, and
  Settings. Auxiliary-page hydration no longer invalidates the analysis cache,
  empty editor snapshots remain restorable, and selecting a different Diagnose
  input resets only document-specific context.
- Corrected Metrics floor-area totals to honor each Zone's Part of Total Floor
  Area flag, and transformed relative surface vertices through the Building and
  Zone coordinate systems before calculating footprint bounds and long/short
  dimensions.
- Preserved engineering precision for small Profile design rates and marked
  missing, interpolated, weather-dependent, or otherwise approximated schedule
  curves as partial instead of presenting step fallbacks as exact results.
- Profile curves that resolve to the same visible path now use interleaved
  color phases instead of hiding behind the last-drawn series. Matching legend
  entries use fixed-width line swatches and identify overlap at the current
  scale without shifting or otherwise altering the plotted values.

## Performance

- Replaced per-render listeners in the large Input, Profile, HVAC, Topology,
  and Simulation surfaces with fixed delegated handlers, and reused
  indexed lookups for semantic, geometry, HVAC, profile, and simulation data.
- Changed the Batch Metrics parse cache to an O(1) LRU that coalesces concurrent reads
  of the same input. Diagnose cleanup scans now honor the bounded worker setting,
  while Batch Simulation reports completion from one serialized collector.
- Removed unused private analysis helpers and reduced repeated allocations and
  scans in Metrics, simulation output lookup, and workbook generation without
  changing the public JSON or automation APIs.
- Reorganized the Batch workspace into Tools with three focused surfaces: Batch Metrics, Batch Simulation, and single-file Diagnose. Diagnose now restores the current app snapshot, supports selecting another input, applies reviewed fixes back to the snapshot, and saves cleaned copies.
- Removed the superseded multi-file Batch Diagnose, Cleanup Report, and Convert / Export runtime endpoints and helpers; single-file diagnostics and reviewed cleanup remain available in Tools / Diagnose.
