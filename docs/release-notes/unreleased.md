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
- Fixed Batch Summary topology metrics to the multiplier-aware effective area
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
- Split Topology controls by 3D, Plan, and Network behavior. Sync locate now
  remains a shared default, 3D and Plan retain independent visibility layers,
  Network story selection appears only for Story scope, and the obsolete
  Select aid and Network Openings options were removed.
- Moved Geometry Fit/Expand, HVAC Expand, and Heat-Flow plan zoom actions into
  their drawing areas as compact icon controls. HVAC now keeps its inspector
  visible and no longer shows a separate inspector toggle button.
- Removed the redundant HVAC and Profile top headers and text search fields.
  HVAC now uses only its compact lower picker cards: the top view switcher,
  Warnings surface, Current focus card, and picker helper text were removed.
  Back, Forward, Clear focus, Zone Services, and Expand now share a horizontal
  icon toolbar at the upper-right of the HVAC drawing instead of a text
  breadcrumb row.
  Apply Profile now sits at the right edge of the compact Profile selector.
- Removed the Diagnose result header and its issue-search, severity, source,
  and hide-code filters. Fix actions now sit in the Fixes card; the independent
  fix-candidate search remains available for cleanup review.
- Removed the redundant Profile Graph deck-status line such as
  `5 series · actual · year · single`.
- Reworked the Profile overview into a compact, table-aligned selector without
  horizontal scrolling. A click sets the primary profile or zone, Ctrl/Cmd
  toggles additional rows, and Shift selects a range. Line views overlay the
  selected rows with an always-visible legend, while annual heatmaps show each
  selection in parallel panels for direct comparison.
- Simplified Profile Graph to a fixed Time Profile over selected Profile
  assignments. Removed Graph Type, Scope, and Compare selectors, replaced the
  View dropdown with six direct view buttons, and moved graph Scale to the
  Profile Analysis section in Settings while preserving legacy view preferences
  during migration.
- Removed direct Profile-to-Topology and Topology-to-Profile links, including
  the Profile detail action, Topology Inspector profile summary, and shared
  related-view/link-bar destinations between those two panels.
- Removed obsolete frontend state, renderers, event branches, styles, and
  translations left behind by the simplified Output, Simulation, Profile,
  HVAC, Diagnose, and Topology controls while retaining backend compatibility
  APIs.

## Fixed

- _None._
