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

- Simulation HVAC integrates equipment availability and control sequence into
  the default loop view, preserving node charts and controls. A shared bottom
  frame updates schedule/manager stacks, current control values and related
  history plots. Already-loaded Timestep data is reused only when all existing
  loop properties remain available; required gaps remain explicit diagnostics.
- Profile adds Cooling setpoint and Heating setpoint as independent checkbox
  dimensions alongside existing loads and airflow. Configured thermostat targets
  are shown in °C, with gaps for inactive, missing or unsupported values rather
  than invented zero temperatures. These dimensions support comparison, not
  thermostat editing through Profile Apply.

## Changed

- Loop equipment inspection shows On / Off conditions as short AND/OR formulas
  with named manager definitions, source inputs and explicit permission rules.
  Repeated rules are defined once; runtime-dependent manager results are
  distinguished from missing or unresolved configuration.
  Parameter control connects inputs/targets, controllers/comparisons, commands
  and equipment response/outputs with directed signal and feedback links.
  Selecting a compact source block opens its original fields and ports while
  preserving supported comparison signs and native Action meanings.
  Aligned control blocks, short smooth connections and separate rounded lanes
  simplify direct inputs and returning references. Hover and keyboard focus
  emphasize connected paths and their endpoints.
  Blocks use a common content-sized width and wrap full labels and source names;
  extra panel width expands the outer margins and connection gaps.
  Connection styles distinguish signals, setpoints/targets, calculation measurement
  inputs and controller feedback using their native relationship, independently
  of card placement. Legend tooltips explain the four meanings.
  Performance-only curves with a declared equipment owner move into the response
  inspector's Performance model section, keeping the main diagram focused on
  control flow. Curve sources, coefficients, unresolved references and native
  target relationships remain available alongside rated equipment settings.
  The timeline's Configured availability is distinguished from the broader
  Run permission and On command; manager/fan permission rules and unresolved
  runtime decisions versus configuration problems use distinct names.
- Availability stacks compact ON/OFF patches for the minimum configured static
  conditions, with owning objects at the left and the combined result at the
  bottom. Supported scheduled-manager rules and plant activation remain;
  runtime managers/load selection are excluded, while configuration errors stay
  unknown. Equipment without a separate schedule has an explicit unrestricted
  row. Rep. day/week and Annual share time-axis zoom/pan and a status cursor.
- Energy Path separates thermal gains/losses and storage/release before native
  interval aggregation. Directional cards show one positive gross magnitude with
  gain/loss in the label, including nonzero exchange with zero load. Ribbons
  retain their allocated load contribution.
  Older Monthly-only evidence remains explicitly net-only; new Basic Energy
  runs request Timestep outputs for reversible thermal quantities.

## Fixed

- Repeated result reads use stable boundary-source/month accumulation and
  cross-loop relationship ordering, avoiding map-order rounding differences
  and reordered equipment links for the same input.
- HVAC availability acquires native Detailed manager decisions instead of
  interpreting averaged status codes as actions. OFF fan schedules retain their
  raw state when a CycleOn override permits operation. Availability and chart
  legends highlight matching observations; HVAC time-series use plain lines,
  and the bottom frame bar supports one-frame buttons and Ctrl+Left/Right.
- Restoring or opening input with HVAC selected no longer stops quick analysis
  with a `textContent` error from the removed inspector. Input views remain usable
  while the remaining analysis stages load.
- Default four-purpose simulation results no longer exceed the native WebView
  text limit on the bundled Large Office case: repeated Heat Flow and Thermal
  Topology arrays are transferred once, preserving every value and observation.
  Result-receiving and JSON-decoding failures now have specific diagnostics and
  are identified separately from EnergyPlus failures.
- Repository cleanup removes identified temporary browser profiles, captures
  and the `.runtime/tmp/` scratch workspace while preserving all build outputs
  during normal cleanup.
- Energy Path restores exterior convection and longwave observations from native
  Zone Timestep SQL timestamps, retaining complete Monthly net evidence when a
  gross split is unavailable. Verified ground-contact surfaces expose outside-face
  conduction separately from room-side load drivers.

## Performance

- Result reading removes the extra native Heat Flow count scan, prepares
  directional classifications once per dictionary, batches fan-pool SQL reads,
  and avoids allocating discarded CSV series when SQL already supplies them.
  Initial Series and Heat Flow reading overlap on multicore runs with two
  independent readers. A preserved 1.63 GB four-purpose result reduced reading
  and construction from 516.7 s to 371.0 s locally, with exact output comparison.
  Native samples, numerical precision, missing-value checks and source priority
  remain unchanged.
