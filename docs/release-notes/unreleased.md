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

- Simplified the main Simulation setup by removing Integrity and Custom Outputs
  purpose setup, the Run Plan display, Advanced run options, and the manual
  EnergyPlus version selector. Allocation, output application, frequency,
  Basic Energy and Zone Heat Flow detail, period, and zone scope now use fixed
  main-view defaults: direct-only allocation, add-missing-only application,
  purpose-default frequency, Heat Drivers, surface detail, the full period, and
  all zones. Backend request options remain available.
- Simplified Batch Simulation with the same fixed output-application,
  frequency, detail, allocation, period, and scope defaults. Removed its manual
  EnergyPlus selector, Integrity purpose, advanced result/detail controls, and
  run-plan preview. Each input now automatically uses a registered or detected
  EnergyPlus installation with a compatible major/minor version; an
  incompatible input fails independently without stopping compatible files.
- Removed Batch Output QA and the main Output result tab. The core Output
  analysis, preview, apply, purpose-output apply, and discovery APIs remain
  available to backend clients and automation. Semantic output entities now
  open their Input/source anchor instead of targeting a removed Output panel.
- Automatically selects a registered EnergyPlus installation compatible with
  the detected model version when possible, and marks Simulation unavailable
  when no usable or version-compatible installation exists.
- Removed the manual Simulation environment refresh action and aligned the
  Weather selector with the application control styling.
- Split Topology controls by 3D, Plan, and Network behavior. Sync locate now
  remains a shared default, 3D and Plan retain independent visibility layers,
  Network story selection appears only for Story scope, and the obsolete
  Select aid and Network Openings options were removed.
- Moved Geometry Fit/Expand, HVAC Expand, and Heat-Flow plan zoom actions into
  their drawing areas as compact icon controls. HVAC now keeps its inspector
  visible and no longer shows a separate inspector toggle button.
- Removed the redundant Profile Graph deck-status line such as
  `5 series · actual · year · single`.
- Removed direct Profile-to-Topology and Topology-to-Profile links, including
  the Profile detail action, Topology Inspector profile summary, and shared
  related-view/link-bar destinations between those two panels.

## Fixed

- _None._
