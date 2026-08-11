# Thermal topology schema

`geometry.topology` is the canonical static thermal-network report used by the
desktop UI, JSON/GraphML/DOT exporters, CLI, local API, and Batch summaries.

## Versions and identity

- Static schema: `semantic-idf.thermal-topology/v1`
- Simulation overlay: `semantic-idf.thermal-topology-simulation/v1`
- `sourceModelHash` hashes the normalized parsed document.
- IDs are deterministic from semantic entity identity, not array position.
- `sourceAnchors` identify source object/field occurrences for navigation.

The topology payload is additive under the existing `geometry` report. The
internal result-tab, route, API, workspace, and shortcut ID remains `geometry`.

## Static report

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

### Node

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

`thermal_boundary`, `thermal_interface`, `window`, and
`thermal_boundary_group` are not serialized `nodes[].kind` values. Boundaries
and openings are authoritative records in their own arrays and are exposed by
the current zone-level Network inspector. Older frontend detail projections
could synthesize those node kinds locally, but they were never part of the
backend schema and the current renderer does not create them.

### Boundary

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

### Opening

`id`, `windowId`, `entityId`, `objectIndex`, `name`, and `surfaceType` identify
the fenestration. `baseSurfaceId`, `ownerZoneId`, and `ownerSpaceId` locate it.
`counterpartOpeningId` and `pairId` canonicalize an interior pair.
`constructionName`, `constructionStatus`, `uValue`, and `hasUValue` describe
thermal performance. `physicalArea`, `effectiveArea`, `ua`, and `hasUa` expose
quantities. `diagnosticIds` and `sourceAnchors` preserve evidence.

### Air coupling

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

### Connection

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

### Zone signature

| Fields | Meaning |
| --- | --- |
| `zoneId`, `zoneName`, `areaBasis`, `spaceIds` | Zone scope and aggregate basis. |
| `exteriorArea`, `groundArea`, `interzoneArea`, `adiabaticArea`, `otherBoundaryArea` | Area by relation family. |
| `exteriorUa`, `groundUa`, `interzoneUa`, `totalUa`, `hasTotalUa`, `uaCoverage` | Static conductance and completeness. |
| `windowArea`, `exteriorWwr` | Opening envelope summary. |
| `adjacentZoneIds` | Resolved thermal neighbors. |
| `closedShell`, `openEdgeCount`, `nonManifoldEdgeCount`, `computedVolume`, `declaredVolume`, `volumeDifferencePct` | Enclosure integrity. |
| `diagnosticIds` | Linked topology/Diagnose issues. |

### Matrix cell

`id`, `rowNodeId`, `columnNodeId`, and `connectionId` identify the symmetric
backend projection. `surfaceCount`, `area`, `ua`, `hasUa`, and
`diagnosticCount` use the report's multiplier-adjusted aggregate basis and
refer to the same compact connection records used by Network. Air couplings
are not mixed into this conductive matrix. The matrix remains an export/API
field; the current main UI does not expose a separate Matrix view.

### Issue link and geometry QA

An issue link contains `id`, `code`, `severity`, `message`, optional
`entityId`/`boundaryId`/`openingId`/`airCouplingId`, `relatedEntityIds`, and
`sourceAnchors`. A geometric adjacency observation contains `surfaceAId`,
`surfaceBId`, `overlapRatio`, `declaredConnection`, and `observationKind`.
The UI derives a deterministic `thermal_observation` target from the sorted
surface IDs and observation kind so both source surfaces remain selected in QA
and 3D/Plan without changing the canonical static schema.
Enclosure records include zone identity, closed/open/non-manifold counts,
computed/declared volume, difference, open edges, and diagnostic IDs.

## Area and UA formulas

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
and UA with Multiplier as a separate inspector variable. Batch Summary requests
the fixed multiplier-adjusted basis, while backend and CLI callers can still
request physical Batch values for compatibility. `hasUa` is false when any
required U-value is unavailable. `uaCoverage` is the covered aggregate area
divided by total applicable area; no partial value is presented as a complete
total.

Reciprocal interzone surfaces and openings use one canonical pair. Their area,
UA, connection count, matrix quantity, and simulated flow are counted once,
while both source IDs remain navigable.

## Relation kinds

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

## Diagnostic codes

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

## Simulation overlay (separate schema)

Simulation data is not inserted into the static report. The purpose result uses
`semantic-idf.thermal-topology-simulation/v1`:

This remains a backend and Simulation-result contract. The current Topology
tab renders only the static Network report and does not expose overlay metrics,
period controls, or a heat-flow ledger.

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
