# Topology views and terminology

The Topology panel is the shared spatial and thermal model interface. Its 3D,
Plan, and Thermal views use one global semantic entity selection. Switching a
view changes the representation, not the selected model entity, and navigation
alone does not trigger analysis.

## View roles

### 3D

Use 3D to inspect position, orientation, envelope shape, and the spatial
relationships among zones, surfaces, and windows.

### Plan

Use Plan to inspect each story's floor-plan composition, zone boundaries, and
surface and window positions.

### Thermal

Use Thermal to inspect the authoritative thermal boundaries through which a
zone or space connects to outdoors, ground, another zone, or a special external
condition. Static Area/UA topology is distinct from a simulated heat-flow
overlay: the two use separate names, units, metrics, and legends.

## Canonical terms

### Thermal boundary

The authoritative relation by which one surface connects its owning space to
an external thermal target. The relation comes from the EnergyPlus boundary
rule and its source fields.

### Thermal interface

One interzone interface formed by two explicit surfaces with a reciprocal
relation.

### Thermal connection

An edge in the compact graph that aggregates one or more boundaries or
interfaces for the same zone-to-target pair.

### Geometric adjacency

A geometry observation that checks whether world-space polygons physically
touch. It is a validation overlay and never creates an authoritative thermal
relation.

### Air coupling

An air-movement relation, such as `ZoneMixing` or an AirflowNetwork path, that
is separate from surface conduction.

### Static UA

The static heat-transmission potential calculated from construction U-value
and model-effective area. Static UA is not energy flow, a load, or a simulation
result.

### Simulated heat flow

An actual simulation result aggregated for a specific period or instant from
EnergyPlus output data.
