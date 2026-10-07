# Topology

Use this reference for the Topology views, static thermal-network payload,
simulation overlay, CLI/export behavior, and regression coverage.

- [Views and interaction](#views-and-interaction)
- [Data contracts](#data-contracts)
- [Regression coverage](#regression-coverage)

Topology uses one selection to show location and thermal connections. Frontend
tab, route, workspace, and shortcut identity is `topology`; saved `geometry`
views normalize on load. Backend reports remain under `report.geometry`, with
`geometry.topology` supplying the canonical static report for desktop, CLI,
local API, Batch Metrics, and JSON/GraphML/DOT exports.

## Views and interaction

Level is shared across 3D, Plan, and Network. **All** shows the whole model;
a specific level filters the projection. 3D/Plan share Zones, Surfaces, and
Openings visibility. Network exposes Metric and Layout. All views share one
resizable lower details panel; Fit and Expand sit inside the drawing area.

### 3D

Inspect position, orientation, envelope shape, and relationships among zones,
surfaces, and openings. Spatial quantities use polygon geometry.

### Plan

Inspect floor-plan composition, boundaries, and openings at all levels or the
selected level. Zone selection maps to the same Network zone identity.

### Network

Inspect the authoritative zone-level thermal network. Compact connections
aggregate boundaries between zone/space owners and their targets; the lower
panel retains source boundary details. Spatial and Network layouts project
the same records deterministically. Dragging endpoints updates connected routes.
Outdoors uses compact directional points; each Adiabatic surface is a selectable
detached wall stub.

Metrics are Connectivity, Area, UA, Exposure, QA, and Air. Selecting a zone
emphasizes incident connections and one-hop endpoints. Selecting a boundary,
opening, or connection emphasizes its relation and endpoints. Unrelated objects
remain in the active Level at very low opacity.

### Boundary, geometry, and quantities

EnergyPlus `Outside Boundary Condition` and its target determine the thermal
relation. Surface pairs are valid only when reciprocal and owned by different
zones. World-space polygon adjacency can confirm a declared pair or report
touching disconnected surfaces; it never creates an authoritative thermal
relation.

Gross area is physical polygon area in m², including openings. Multiplier is
shown separately. The inspector retains opaque/opening contributions when
relevant. UA is static conductance in W/K: opaque area × construction U plus
opening area × opening U. A complete total requires every necessary U-value;
coverage is separate from the total.

Reciprocal surfaces form one interface; reciprocal interior openings likewise
count once. Both source IDs remain navigable. Network quantities and simulation
flow use the same canonical side. The UI shows physical area/UA, while Batch
Metrics uses multiplier-adjusted aggregates; both sets are preserved in the
backend schema. See [Data contracts](#data-contracts)
for exact formulas, fields, diagnostic codes, and simulation-overlay units.

Simulation observations belong in Simulation. Topology has no simulated-heat
metric, period control, or heat-flow ledger.

### Air and QA

Air records include ZoneMixing, ZoneCrossMixing, refrigeration door mixing,
Construction:AirBoundary, and AFN surface paths. They preserve direction,
design flow, schedule, AFN component/surface, and source anchors. Air metric
renders them separately from conduction. Ordinary zone-local infiltration
does not create a zone-pair edge.

QA distinguishes geometric observations from rule issues. Issues cover
missing/one-way/duplicate counterparts, construction/geometry mismatch,
unresolved external targets, open/non-manifold enclosures, and missing
air/AFN targets. Tools / Diagnose reports the same issue IDs for the same
document snapshot. A geometric observation can select its two source surfaces
without turning that observation into a thermal connection.

### Selection and navigation

3D, Plan, and Network share semantic selection with the visible Text, JSON,
and Table source views. Selection reveals the input object and emphasizes
its one-hop related objects without changing Level. Details show calculated
metrics and related objects; diagnostics and fixes are accessed in Tools.

History restores view, Level, metric, layout, spatial visibility, pan/zoom,
and stable selection. Navigation uses cached current reports and makes no
backend analysis request. Settings/Tools preserve the snapshot key so returning
restores the same topology context. Shared controller behavior is defined in
[semantic-navigation.md](semantic-navigation.md).

When Topology is active, default keys `1`/`2`/`3` switch 3D/Plan/Network;
`F` fits; `T`/`A`/`U`/`Q` select Connectivity/Area/UA/QA. Network targets
follow deterministic tab order and activate with Enter/Space. Shortcuts are
configurable and do not consume editor input.

### Canonical terms

#### Thermal boundary

The authoritative relation from one heat-transfer surface's owner to its target.

#### Thermal interface

One interzone interface formed by a reciprocal surface pair.

#### Thermal connection

A compact owner-to-target edge aggregating boundaries/interfaces of one relation.

#### Geometric adjacency

World-space polygon evidence used to check modeling intent; it never creates
an authoritative thermal relation.

#### Air coupling

Air movement represented separately from surface conduction.

#### Static UA

Construction/area-derived conductance. Static UA is not energy flow, a load,
or a simulation result.

#### Simulated heat flow

A signed EnergyPlus observation for a period/frame with output-source provenance.

### API and CLI

Stable IDs, source anchors, issue links, geometry evidence, coverage, and hashes
remain available in backend/CLI reports. The Network toolbar does not expose
a JSON exporter or Advanced menu.

```text
semantic-idf topology --level zone --metric ua --area-basis effective model.idf
semantic-idf topology --level boundary --metric qa --format graphml model.idf
```

## Data contracts

### Versions and identity

- Static schema: `semantic-idf.thermal-topology/v1`
- Simulation overlay: `semantic-idf.thermal-topology-simulation/v1`
- `sourceModelHash` hashes the normalized parsed document.
- IDs are deterministic from semantic entity identity, not array position.
- `sourceAnchors` identify source object/field occurrences for navigation.

The topology payload remains additive under the existing `report.geometry`
backend field, and `AnalyzeInputGeometryText` remains as an API compatibility
entrypoint.

### Static report

| Field | Type | Meaning |
| --- | --- | --- |
| `schema` | string | Static schema identifier. |
| `sourceModelHash` | string | Hash of the source model used to build the report. |
| `areaBasis` | string | Canonical aggregate basis; currently `effective`, meaning multiplier-adjusted. |
| `nodes` | node[] | Zone, space, environment/external-target, and unresolved graph nodes. |
| `boundaries` | boundary[] | One authoritative record per supported heat-transfer surface. |
| `connections` | connection[] | Compact owner-to-target aggregates, including the separate air layer. |
| `openings` | opening[] | Fenestration records attached to base boundaries. |
| `airCouplings` | air coupling[] | Mixing, air-boundary, ventilation, and AFN relations. |
| `zoneSignatures` | zone signature[] | Per-zone envelope/UA/adjacency/enclosure summary. |
| `matrix` | matrix cell[] | Symmetric compact conductive connection projection. |
| `issueLinks` | issue link[] | Stable issues shared with Diagnose. |
| `geometryDescriptors` | descriptor[] | Plane, centroid, bounds, area, and edge evidence. |
| `adjacencyObservations` | observation[] | Geometric QA evidence; never a generated thermal relation. |
| `zoneEnclosures` | enclosure[] | Closed-shell, open/non-manifold edge, and volume checks. |
| `geometryTolerance` | number | World-coordinate comparison tolerance in metres. |
| `geometryRuleVersion` | string | Geometry QA algorithm version. |
| `stats` | object | Node/boundary/connection/opening/air/invalid/diagnostic counts. |

#### Node

| Fields | Meaning |
| --- | --- |
| `id`, `entityId`, `kind`, `label` | Stable graph identity and display role. `entityId` is omitted for virtual environments. |
| `zoneName`, `spaceName`, `storyIndex` | Spatial ownership and story context. |
| `objectType`, `objectName`, `objectIndex` | Source object identity when the node is model-backed. |
| `physicalArea`, `effectiveArea`, `floorArea`, `volume`, `centroid` | Available spatial quantities. |
| `diagnosticIds`, `sourceAnchors` | Issue and source navigation links. |

Common backend `kind` values are `zone`, `space`, `outdoors` and its
orientation-specific variants, `ground`, `adiabatic`, `foundation`,
`ground_preprocessor`, `other_side_coefficients`,
`other_side_conditions_model`, and `unresolved_target`.

Boundaries and openings remain authoritative records in their own arrays and
are exposed by the zone-level Network inspector. `thermal_boundary`,
`thermal_interface`, `window`, and `thermal_boundary_group` are not serialized
`nodes[].kind` values; callers must not treat frontend target kinds as backend
graph nodes.

#### Boundary

| Field group | Fields |
| --- | --- |
| Identity/source | `id`, `surfaceId`, `surfaceEntityId`, `surfaceObjectIndex`, `surfaceName`, `surfaceType`, `sourceAnchors` |
| Ownership | `ownerZoneId`, `ownerSpaceId` |
| Declared rule | `boundaryConditionRaw`, `boundaryCondition`, `boundaryObjectRaw`, `relationKind` |
| Resolved target | `targetKind`, `targetId`, `targetName` |
| Reciprocal pair | `counterpartSurfaceId`, `counterpartSurfaceEntityId`, `pairId`, `virtualCounterpart` |
| Construction | `constructionName`, `constructionObjectIndex`, `constructionStatus`, `uValue`, `hasUValue` |
| Physical area | `physicalGrossArea`, `physicalOpeningArea`, `physicalOpaqueArea` |
| Multiplier-adjusted area | `effectiveGrossArea`, `effectiveOpeningArea`, `effectiveOpaqueArea` |
| Conductance | `opaqueUa`, `openingUa`, `totalUa`, `hasUa` |
| Exposure | `orientation`, `azimuth`, `sunExposure`, `windExposure` |
| Related data | `openingIds`, `diagnosticIds`, `geometryCheck` |

`geometryCheck` contains `status`, `areaDifferencePct`, `overlapRatio`,
`normalDot`, `planeDistance`, and an evidence `message`.

#### Opening

`id`, `windowId`, `entityId`, `objectIndex`, `name`, and `surfaceType` identify
the fenestration. `baseSurfaceId`, `ownerZoneId`, and `ownerSpaceId` locate it.
`counterpartOpeningId` and `pairId` canonicalize an interior pair.
`constructionName`, `constructionStatus`, `uValue`, and `hasUValue` describe
thermal performance. `physicalArea`, `effectiveArea`, `ua`, and `hasUa` expose
quantities. `diagnosticIds` and `sourceAnchors` preserve evidence.

#### Air coupling

| Field | Meaning |
| --- | --- |
| `id`, `entityId`, `objectType`, `objectName`, `objectIndex` | Stable coupling/source identity. |
| `fromNodeId`, `toNodeId`, `direction`, `couplingKind` | Directed or bidirectional relation. |
| `designFlowRate`, `unit`, `scheduleName` | Available design flow and control schedule. |
| `surfaceId`, `componentName` | Construction:AirBoundary or AFN surface/component context. |
| `diagnosticIds`, `sourceAnchors` | Missing target/component and source evidence. |

`couplingKind` includes `zone_mixing`, `zone_cross_mixing`,
`refrigeration_door_mixing`, `outdoor_ventilation`,
`construction_air_boundary`, and `airflow_network`.

#### Connection

`id`, `fromNodeId`, `toNodeId`, and `relationKind` identify a compact edge.
`qaOnly` excludes invalid/observation edges from thermal totals.
`boundaryIds`, `openingIds`, and `airCouplingIds` retain members.
`surfaceCount` and `openingCount` are canonical counts, so reciprocal pairs are
not doubled. Physical and effective gross/opaque/opening fields mirror the
boundary fields; the effective fields incorporate the applicable multipliers.
`opaqueUa`, `openingUa`, `totalUa`, and `hasUa` use that multiplier-adjusted
basis, while `physicalOpaqueUa`, `physicalOpeningUa`, `physicalTotalUa`, and
`hasPhysicalUa` retain the single-instance quantities. `orientations`,
`diagnosticIds`, and `sourceAnchors` provide presentation and traceability.

#### Zone signature

| Fields | Meaning |
| --- | --- |
| `zoneId`, `zoneName`, `areaBasis`, `spaceIds` | Zone scope and aggregate basis. |
| `exteriorArea`, `groundArea`, `interzoneArea`, `adiabaticArea`, `otherBoundaryArea` | Area by relation family. |
| `exteriorUa`, `groundUa`, `interzoneUa`, `totalUa`, `hasTotalUa`, `uaCoverage` | Static conductance and completeness. |
| `windowArea`, `exteriorWwr` | Opening envelope summary. |
| `adjacentZoneIds`, `airCoupledZoneIds` | Conductive thermal neighbors and separately resolved air-coupled zones. |
| `closedShell`, `openEdgeCount`, `nonManifoldEdgeCount`, `computedVolume`, `declaredVolume`, `volumeDifferencePct` | Enclosure integrity. |
| `diagnosticIds` | Linked topology/Diagnose issues. |

#### Matrix cell

`id`, `rowNodeId`, `columnNodeId`, and `connectionId` identify the symmetric
backend projection. `surfaceCount`, `area`, `ua`, `hasUa`, and
`diagnosticCount` use the report's multiplier-adjusted aggregate basis and
refer to the same compact connection records used by Network. Air couplings
are not mixed into this conductive matrix. The matrix remains an export/API
field; the current main UI does not expose a separate Matrix view.

#### Issue link and geometry QA

An issue link contains `id`, `code`, `severity`, `message`, optional
`entityId`/`boundaryId`/`openingId`/`airCouplingId`, `relatedEntityIds`, and
`sourceAnchors`. A geometric adjacency observation contains `surfaceAId`,
`surfaceBId`, `overlapRatio`, `declaredConnection`, and `observationKind`.
The UI derives a deterministic `thermal_observation` target from the sorted
surface IDs and observation kind so both source surfaces remain selected in QA
and 3D/Plan without changing the canonical static schema.
Enclosure records include zone identity, closed/open/non-manifold counts,
computed/declared volume, difference, open edges, and diagnostic IDs.

### Area and UA formulas

For a surface/opening instance:

```text
effective area = physical area × zone multiplier × surface/opening multiplier
opaque area = max(0, gross area - sum(opening areas))
opaque UA = opaque area × opaque construction U
opening UA = sum(opening area × opening construction U)
total UA = opaque UA + opening UA
```

The static report's `areaBasis` is fixed to `effective` for aggregate zone
signatures and matrix values. Here, effective means multiplier-adjusted; it
does not mean a second geometry. Boundary and connection records retain both
single-instance physical fields and multiplier-adjusted effective fields for
API and export consumers.

The main Network UI has no area-basis selector. It shows physical Gross area
and UA with Multiplier as a separate inspector variable. Batch Metrics requests
the fixed multiplier-adjusted basis, while backend and CLI callers can still
request physical batch values for compatibility. `hasUa` is false when any
required U-value is unavailable. `uaCoverage` is the covered aggregate area
divided by total applicable area; no partial value is presented as a complete
total.

Reciprocal interzone surfaces and openings use one canonical pair. Their area,
UA, connection count, matrix quantity, and simulated flow are counted once,
while both source IDs remain navigable.

### Relation kinds

| `relationKind` | Source rule |
| --- | --- |
| `exterior` | `Outdoors` |
| `ground` | `Ground` |
| `ground_preprocessor` | Ground FCfactor/slab/basement preprocessor families |
| `foundation` | `Foundation` resolved to `Foundation:Kiva` |
| `other_side_coefficients` | `OtherSideCoefficients` named target |
| `other_side_conditions_model` | `OtherSideConditionsModel` named target |
| `adiabatic_explicit` | Explicit `Adiabatic` |
| `adiabatic_self_reference` | Unambiguous Surface self-reference |
| `interzone_explicit_surface` | Reciprocal Surface pair owned by different zones |
| `interzone_implicit_zone` | `Zone` target with virtual counterpart |
| `interspace_implicit` | `Space` target with virtual counterpart |
| `air_coupling` | Separate compact air-movement edge |
| `invalid` | Unresolved or invalid boundary retained for QA |

### Diagnostic codes

Boundary/reference rules:

```text
missing_boundary_target
invalid_boundary_condition
surface_self_reference_invalid
surface_counterpart_missing
surface_counterpart_one_way
surface_counterpart_duplicate
surface_pair_zone_mismatch
surface_missing_construction
surface_construction_unresolved
boundary_exposure_rule_mismatch
```

Surface/opening validation:

```text
surface_pair_area_mismatch
surface_pair_plane_mismatch
surface_pair_normal_mismatch
surface_pair_overlap_mismatch
surface_pair_construction_mismatch
surface_pair_layer_order_mismatch
fenestration_base_surface_missing
fenestration_zone_mismatch
fenestration_counterpart_missing
fenestration_counterpart_one_way
fenestration_area_mismatch
fenestration_area_exceeds_base
fenestration_construction_mismatch
```

Enclosure and air rules:

```text
zone_shell_open
zone_shell_non_manifold
zone_volume_mismatch
air_coupling_target_missing
airflow_network_surface_missing
airflow_network_component_missing
```

### Simulation overlay (separate schema)

Simulation data uses `semantic-idf.thermal-topology-simulation/v1` in purpose
results, separately from the static report. Topology renders static Network;
simulation metrics, period controls, and heat-flow ledgers belong to Simulation.

| Field | Meaning |
| --- | --- |
| `schema`, `available`, `unavailableReason`, `state` | Overlay identity/capability. `state` distinguishes `static_topology` and `simulation_overlay`. |
| `signConvention` | Positive enters the canonical owner; negative leaves it. |
| `periods` | Annual/monthly/daily/hourly/selected-range projections. |
| `sources` | SQL/series variable provenance, units, and aggregation method. |
| `completeness`, `reconciliation`, `outputWeight` | Coverage, consistency, and requested-result size. |

A period contains `id`, `label`, `kind`, `labels`, `frameCount`,
`boundaryFlows`, and `connectionFlows`. Boundary flows provide stable
boundary/connection/owner/target IDs, related reciprocal boundary IDs, signed
`value`/`values`, unit, direction, aggregation method, source IDs, and
per-family traces. Connection flows carry the same signed quantities at compact
edge level. Energy outputs are normalized to kWh; rate-only sources are
integrated using their reporting intervals and disclose that aggregation in
source metadata.

### Implementation references

- [topology-view.js](../cmd/semantic-idf/frontend/src/js/views/topology-view.js) and [thermal-topology-view.js](../cmd/semantic-idf/frontend/src/js/views/thermal-topology-view.js): view state and shared projection; renderer/layout/details load lazily.
- [thermal_topology.go](../cmd/semantic-idf/internal/idf/thermal_topology.go): static model types and boundary/connection construction.
- [thermal_geometry_qa.go](../cmd/semantic-idf/internal/idf/thermal_geometry_qa.go): geometric evidence and enclosure rules.
- [thermal_export.go](../cmd/semantic-idf/internal/idf/thermal_export.go): JSON/GraphML/DOT projection and area-basis compatibility.
- [thermal_topology_test.go](../cmd/semantic-idf/internal/idf/thermal_topology_test.go): schema, stable identity, and model invariants.

## Regression coverage

These regression scenarios execute real parser/analyzer code and real
frontend ES modules. The browser harness serves the production frontend source
to an installed Chromium-family browser; it does not reimplement topology or
renderer logic in the test.

| Contract | Automated flow | Regression test |
| --- | --- | --- |
| TOPO-280 Exterior zone | Select a spatial exterior wall, project it to the compact Outdoors edge, and inspect Gross area, Multiplier, openings, physical UA, and the exact OBC source. Confirm the Network remains zone-level with no boundary drill-down or area-basis controls. | `TestTOPO280ExteriorZoneIntegratedFlow`; browser signal `topo280` |
| TOPO-281 Interzone pair | Select a reciprocal zone edge, retain both source boundaries and the canonical opening, verify reversed construction layers, and inspect Gross area and Multiplier without creating boundary graph nodes. | `TestTOPO281InterzonePairIntegratedFlow`; browser signal `topo281` |
| TOPO-282 Modeling decision/QA | Select a geometrically adjacent but adiabatic observation as a two-surface QA target, prove no thermal relation is generated, and preserve stable boundary identity after the source fix. Confirm the Topology inspector has no Diagnostics or Actions section. | `TestTOPO282ModelingDecisionAndQAIntegratedFlow`; browser signal `topo282` |
| TOPO-283 Air coupling | Keep ZoneMixing and AFN paths separate from conductive edges and expose direction, schedule, design flow, base surface, component, and source evidence. | `TestTOPO283AirCouplingIntegratedFlow`; browser signal `topo283` |
| TOPO-284 Simulation separation | Build and validate the signed simulation-overlay result as a separate backend contract, while confirming the Topology tab exposes no simulated-heat metric, period selector, or heat-flow ledger and continues to show static UA. | `TestTOPO284SimulationOverlayIntegratedFlow`; browser signal `topo284` |
| TOPO-285 Context/Tools | Capture and restore Network metric, selection, and pan/zoom without backend calls; compare the fixed multiplier-adjusted Batch Metrics interzone area 8→12 m² as Δ4 m² / 50%. | `TestTOPO285SettingsAndBatchRoundTripContract`; browser signal `topo285` |

Run all topology tiers using the feature catalog:

```powershell
.\dev.bat test -Area topology
```

For a focused uncached parser/browser run, use Go `-run 'TestTOPO28'` in the
IDF, Simulation, and frontendchecks packages. `scripts/verify.ps1` checks the
changed scope and a production Wails build; `-Full` includes every Go package
and browser harness. See [testing.md](testing.md).
