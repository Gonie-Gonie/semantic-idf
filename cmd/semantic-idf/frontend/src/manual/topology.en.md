# Geometry and static thermal Topology

Topology combines spatial geometry with the model's declared thermal
relationships. Use it to inspect where an object is, what it is connected to,
and whether geometry and references support that declared relationship.

## Start a Topology investigation {#topology-workflow}

1. Open **Topology** after its current analysis is ready.
2. Choose **3D** for overall form, **Plan** for floor composition, or **Network**
   for Zone-level thermal relationships.
3. Set **Level** to All or a particular level. Level is shared by all three views.
4. Select the Zone, surface, opening or connection of interest and read the
   resizable lower details panel.
5. Check boundary rules, construction, quantities and source relationships.
6. For problems, open **Tools / Diagnose**; then edit the source and repeat the
   check after analysis refreshes.

Fit resets the drawing extent. Expand gives the drawing more space. 3D/Plan
share Zones, Surfaces and Openings visibility; Network has Metric and Layout
controls. Changing a display control does not alter the input model.

Default keys while Topology is active are `1`/`2`/`3` for 3D/Plan/Network,
`F` for Fit, and `T`/`A`/`U`/`Q` for Connectivity/Area/UA/QA. Configurable
shortcuts do not consume editor input. Network targets can be reached in
deterministic keyboard order and activated with Enter or Space.

## Spatial views and coordinate rules {#spatial-coordinates}

3D shows envelope shape, orientation and Zone/surface/opening placement.
Plan shows floor composition at the selected level. Both project the same
world-space polygons; a selected Zone has the same identity in Network.

GlobalGeometryRules distinguishes detailed and rectangular coordinate systems,
starting vertex and entry direction. Supported legacy rectangular geometry is
expanded into polygons from its declared position, dimensions and orientation.
Detailed surfaces use explicit vertices or supported vertex-count resolution.
An unresolved polygon is not reconstructed merely to make the drawing complete.

For relative coordinates, the Zone origin is rotated by Building North Axis,
and local vertices by Building North Axis + Zone Direction of Relative North;
the transformed origin is then added. In coordinate form, with the application's
rotation matrix R:

```text
world vertex = R(building north + zone direction) × local vertex
             + R(building north) × zone origin
R(θ)(x,y,z) = (x cosθ - y sinθ, x sinθ + y cosθ, z)
```

World-coordinate vertices are used as declared and are not rotated/transposed
a second time. Azimuth is resolved consistently with the coordinate declaration
and vertex-normal convention. Inspect the source North Axis and relative north
before correcting a seemingly rotated model.

Levels are inferred from supported floor elevations and Zone vertical context.
They are presentation groups, not additional EnergyPlus Zone objects or a
guarantee of architectural storey naming. A tall Zone, mezzanine or sloped floor
can need inspection in All rather than a single inferred level.

Missing objects in a drawing can result from the selected level, visibility,
unresolved ownership or unusable geometry. Clear display restrictions and
inspect the source before concluding that the object was removed.

## Boundaries, interfaces and connections {#thermal-relations}

| Term | Meaning |
| --- | --- |
| Thermal boundary | Authoritative relation from one heat-transfer surface's owner to its declared target |
| Thermal interface | One reciprocal interzone surface pair treated as one interface |
| Thermal connection | Compact owner-to-target edge containing one or more boundaries/interfaces |
| Geometric adjacency | Polygon evidence that checks modeling intent without creating a thermal connection |
| Air coupling | Air-movement relation represented separately from surface conduction |

The surface's **Outside Boundary Condition** and its referenced object govern
the relation. Outdoors and Ground lead to their environment targets. Foundation
resolves its Kiva target. Ground-preprocessor, OtherSideCoefficients and
OtherSideConditionsModel families retain their specific boundary roles.
Adiabatic remains an explicit no-transfer boundary and is shown as a detached,
selectable wall stub in Network.

A Surface pair must be reciprocal and owned by different Zones. Both surfaces
remain available for source navigation, while area, UA and connection totals
count the canonical interface once. Interior opening pairs also count once.
An unambiguous self-reference can represent an adiabatic self relation; other
invalid self references remain QA issues.

Declared Zone or Space targets can use virtual counterpart relationships.
They are not evidence that a second detailed polygon exists. Missing, one-way
or duplicate Surface counterparts are not repaired by the graph builder.
Invalid/unresolved relations remain visible for QA and are not ordinary thermal
totals.

Two polygons touching in world space do not automatically exchange heat in the
model. For example, adjacent adiabatic walls remain adiabatic; QA can highlight
the two surfaces as a geometric observation without generating an interzone
thermal edge.

## Read Network metrics and selection {#network-metrics}

Network is a Zone-level graph. Compact Outdoors endpoints communicate exposure
direction; connections retain boundary/opening members in their inspector.
It does not create a separate graph node for each wall/window or expose a
boundary drill-down/Matrix view.

| Metric | Question |
| --- | --- |
| Connectivity | Which authoritative thermal relationships connect these owners/targets? |
| Area | How much applicable boundary area does this relationship represent? |
| UA | What static construction/area conductance is available? |
| Exposure | What outdoor orientation and sun/wind exposure is declared? |
| QA | Where do declared rules, references or geometry need review? |
| Air | Which separate modeled air-movement paths connect owners/targets? |

Spatial and Network layouts use the same deterministic records. Dragging an
endpoint changes the visual layout and routes, not source coordinates or flow.
A selected Zone emphasizes incident connections and one-hop endpoints. A
selected boundary/opening/connection emphasizes its relation; unrelated objects
in the active Level become faint rather than being removed from the model.

Topology selection follows the shared Text/JSON/Table source-navigation model.
It does not change Level to force a new scope. History restores view, level,
metric, layout, visibility, pan/zoom and stable selection using current cached
reports. Selection itself does not request a new analysis.

## Area, U and UA interpretation {#topology-quantities}

Gross area includes openings. Opaque area subtracts resolved opening area and
is clamped at zero when that subtraction would be negative. The excess-opening
diagnostic still matters; the clamp does not make impossible geometry valid.

```text
opaque area = max(0, gross area - sum(opening areas))
opaque UA = opaque area × opaque construction U
opening UA = sum(opening area × opening construction U)
total UA = opaque UA + opening UA
effective area = physical instance area × applicable multiplier factors
```

The main Network UI reports physical Gross area and UA, with Multiplier shown
separately. Multiplier-adjusted effective fields are preserved in reports and
are the fixed basis for aggregate signatures and Batch Metrics. There is no
main-UI area-basis selector. A multiplier expresses repeated model instances;
it does not stretch the polygon. Inspect surface/opening factors as well as Zone
factors when reconciling quantities. Static Geometry/Topology currently reads
Zone.Multiplier for its effective geometry; it does not expand ZoneGroup's
ZoneList multiplier. Metrics does include that additional group factor. In a
ZoneGroup model, compare the disclosed factors before equating effective
Topology summaries with building Metrics totals.

Example: a physical 20 m² wall has 4 m² of windows; opaque U=0.4 and window
U=2 W/(m²·K). Physical UA is 16×0.4+4×2=14.4 W/K. A uniform applicable factor
3 gives effective UA 43.2 W/K and gross area 60 m². An interzone counterpart
does not make either quantity twice as large.

Complete UA requires the needed opaque/opening U values. An unavailable U is
not a zero-U material. Coverage records how much applicable area has resolvable
thermal performance; a covered subset is not presented as a complete total.
The construction estimate follows supported material/direct fields; review
[construction assumptions](./metrics.en.md#construction-thermal) before using it.

Static UA has unit W/K. It is conductance, not cooling/heating load, power at
the current weather, or annual energy. Exposure is declared metadata; it does
not integrate weather-dependent solar gain or wind-driven infiltration.

## Air connections and their limits {#air-coupling}

Air records cover supported ZoneMixing, ZoneCrossMixing, refrigeration-door
mixing, Construction:AirBoundary and AirflowNetwork surface/component paths,
along with applicable outdoor ventilation relations. Inspect direction,
design-flow value/unit, schedule, component and base surface when provided.

ZoneMixing is directional; cross-mixing or other bidirectional rules retain
their own semantics. Air edges are rendered separately from conductive edges
and are excluded from the conductive matrix. Design flow in m³/s is not W/K
or kWh. It does not establish actual annual exchange without schedules,
pressure/temperature effects and engine results.

Ordinary zone-local infiltration does not create an arbitrary Zone-to-Zone
edge. A missing AFN component or referenced Zone/surface is a resolution issue,
not proof of zero flow. [Profile airflow](./metrics.en.md#profile-algorithms)
explains nominal design profiles and weather-dependent limits.

## Geometry QA algorithm {#geometry-qa}

Geometry QA builds world-space plane normals, centroids, bounds, polygon areas
and quantized edge evidence. Its distance tolerance in metres is
`max(0.00001, model bounding diagonal × 0.000001)`. The report retains this
tolerance and the geometry-rule version so evidence can be interpreted.

For a declared reciprocal pair, validation checks:

- opposed normals, with dot product ≤ -0.99;
- plane distance within the supported separation allowance;
- relative area difference ≤ 1%;
- projected polygon overlap ratio ≥ 0.99.

The separation allowance starts at 10×distance tolerance and can account for
resolved construction thickness. Plane-distance diagnostics also retain the
geometric distance evidence; a warning alone should be read with the combined
pair status and construction context.

Overlap projects polygons onto a dominant plane and uses polygon clipping.
It divides intersection area by the smaller polygon area. A large overlap
ratio alone does not prove equal areas; the separate area check matters.
Irregular/non-convex geometry and tolerance-scale features merit source review
rather than treating QA as a complete solid-modeling proof.

Zone enclosure QA counts quantized undirected edges: a closed shell should have
two surface uses per edge; one is open and more than two is non-manifold.
Computed enclosure volume sums triangular tetrahedral contributions around a
common geometric center. It is a check for the supported enclosure representation,
not a replacement for every declared EnergyPlus volume or an exact arbitrary
non-convex solid solver. A computed/declared difference above 10% is flagged.

## Diagnose issue families {#topology-diagnostics}

| Family | Representative codes | Investigation |
| --- | --- | --- |
| Boundary/reference | `missing_boundary_target`, `invalid_boundary_condition`, `surface_self_reference_invalid`, `surface_counterpart_missing`, `surface_counterpart_one_way`, `surface_counterpart_duplicate`, `surface_pair_zone_mismatch` | Check boundary condition, exact target and reciprocal ownership |
| Construction/exposure | `surface_missing_construction`, `surface_construction_unresolved`, `boundary_exposure_rule_mismatch`, `surface_pair_construction_mismatch`, `surface_pair_layer_order_mismatch` | Resolve construction; review counterpart layer order and sun/wind rules |
| Pair geometry | `surface_pair_area_mismatch`, `surface_pair_plane_mismatch`, `surface_pair_normal_mismatch`, `surface_pair_overlap_mismatch` | Inspect both world polygons, entry direction and tolerances |
| Openings | `fenestration_base_surface_missing`, `fenestration_zone_mismatch`, `fenestration_counterpart_missing`, `fenestration_counterpart_one_way`, `fenestration_area_mismatch`, `fenestration_area_exceeds_base`, `fenestration_construction_mismatch` | Check base surface, pairing, area and construction |
| Enclosure | `zone_shell_open`, `zone_shell_non_manifold`, `zone_volume_mismatch` | Locate open/duplicate edges and compare declared dimensions |
| Air | `air_coupling_target_missing`, `airflow_network_surface_missing`, `airflow_network_component_missing` | Resolve the actual air-path source and referenced component |

Topology and Diagnose share issue identity for the same analyzed document.
Geometric observations and declared-rule issues remain distinguishable.
Selecting an observation can identify both source surfaces without declaring
an EnergyPlus thermal connection. The Topology inspector presents quantities
and related objects; diagnosis/fix actions are in Tools.

## Static Topology, simulation and exports {#topology-exports}

Topology has no simulated-heat metric, period selector or heat-flow ledger.
Signed observed heat flow belongs in Simulation's Heat Flow inspection. A
static thermal graph can exist even when no required engine output was recorded.
Conversely, a simulation observation does not change the canonical model boundary.

The static report uses `semantic-idf.thermal-topology/v1` with stable semantic
IDs, source anchors and a model hash. Simulation overlays use a separate schema
and source/period metadata. Preserve those distinctions when processing exports.

The main Network toolbar does not offer a JSON exporter. CLI topology export
supports canonical JSON, GraphML and DOT, including level/metric/scope and
physical/effective area-basis options. For example:

```text
semantic-idf topology --level zone --metric ua --area-basis effective model.idf
semantic-idf topology --level boundary --metric qa --format graphml model.idf
```

Batch Metrics uses multiplier-adjusted topology summaries. A numerical delta
is meaningful only when boundary scope and thermal-performance coverage agree.
A changed source model hash identifies a different analyzed model; do not join
simulation values to a static graph merely by matching display labels.
