# Energy Path JSON contract

For file-based CLI and Python/API access, see [Energy Path CLI and Python](energy-path-cli.md).

This document describes the canonical simulation data, not a conventional
single-scale Sankey balance. The graph explains **load drivers → thermal load →
end-use energy → energy source**. It does not establish causal savings.

The authoritative wire types are in
[`energy_explanation.go`](../cmd/semantic-idf/internal/simulation/energy_explanation.go).
The simulation entry point is `BuildPurposeResultBundle`; the current read
adapter and canonical graph transformations are in
[`energy_path_v2.go`](../cmd/semantic-idf/internal/simulation/energy_path_v2.go).
Clients must reuse the returned data rather than implement another SQL parser,
multiplier pass, annual allocation, or equipment-energy inference.

## Envelope and context

The simulation response contains sibling objects under `purposeResults`:

| Object | Schema | Main contents |
| --- | --- | --- |
| `energyExplanation` | `semantic-idf.energy-explanation/v2` | `scope`, `nodes`, `links`, `periods`, `sources`, `quality`, reconciliation and warnings |
| `energyExplanationSummary` | `semantic-idf.energy-explanation-summary/v2` | `scope`, `period`, `drivers`, `loads`, `endUses`, `carriers`, `ratios`, `residuals`, `topZones`, quality |

The graph has no root `summary` or `period` scalar. Its top-level graph is the
annual graph. `periods[]` contains `id`, `label`, `kind`,
and period-local `nodes`, `links`, `summary`, `quality`, reconciliation and
warnings. The primary UI and Python example below use `annual`, `M1`…`M12`.
The underlying Building payload can also retain `selected_range`, `D<n>` or
`H<n>` records when available; preserve these rather than assuming every wire
period is interactive. Do not add the top-level annual graph to monthly records.

`scope` is `{kind, zoneName?, aggregationBasis}`. Kind is `building` or `zone`;
the normal effective accounting basis is `model_total`. A Zone result is not
automatically a one-copy physical Zone. Respect its reported multipliers and
basis. `zoneResults[]` supplies already calculated Zone scopes and their graphs,
summaries and periods. Its shared source dictionary remains on the parent
explanation. `availableZones` is discovery metadata, not permission to invent a
Zone result. Missing or ambiguous scopes/months must remain unavailable.

## Four stages and two scales

| Node `level` | Meaning | `scaleDomain` | Typical unit |
| --- | --- | --- | --- |
| `driver` | Allocated contribution explaining an actual service load | `thermal` | `kWh` thermal |
| `load` | Observed cooling/heating quantity at the declared thermal boundary, with available component detail | `thermal` | `kWh` thermal |
| `end_use` | Site energy used by equipment or a direct use | `site` | `kWh` site |
| `carrier` | Electricity, gas, district energy or another energy carrier | `site` | `kWh` site |

The domain is a node field. `kWh` can appear on both sides without making the
domains interchangeable. Use two independent linear width scales. A thermal
100 kWh → electricity 25 kWh conversion is valid, not a 75 kWh reconciliation
error. Pixel-width ratios are never COP.

Node identity is the opaque `id`; join links by exact IDs. Use `level`, `kind`,
`serviceKind`, `driverCategory`, `endUse` and `carrier` for meaning, not ID parsing
or labels. Main node magnitude is `value`. `rawValue`, `effectiveValue`,
`allocatedValue`, `displayValue`, `signedValue`, `allocationApplied`, `basis`,
`multiplier`, source IDs and related entity/path IDs retain distinct evidence.
An explicit allocated zero must not fall back to nonzero raw pressure. Optional
missing numeric fields are not evidence of zero. Summary rows with zero values
can be pruned; an absent comparison category is therefore not a reported zero.

Cooling/heating load detail can include `loadBreakdown`, `offsetEffects`,
`latentShare` and `simultaneousLoad`. These do not create additional main stages.
The optional load-node `thermalBoundary` has two values:

- `active_surface_source`: fluid heat inserted into or removed from an active
  radiant surface. The native quantity already includes model multipliers; it
  is not same-period heat delivered to Zone air.
- `mixed_thermal_boundaries`: selected active-surface and air/other thermal
  component quantities have been combined. Their total is not a single
  Zone-air measurement boundary.

An unset field retains the existing node contract; clients must not infer it
from source names. A radiant source is a combined thermal component, not an
independently measured sensible/latent decomposition. Surface storage and heat
exchange with other surfaces can shift its Zone-air effect between periods;
do not add active-surface heat again as another delivered-load measurement.

Carrier-neutral end-use nodes may branch to several carriers; each branch keeps
its own carrier-specific source IDs. Do not copy a node's union of source IDs
onto every branch or add facility totals to their end-use components.

`support` nodes and `support_supply` links hold purchased/produced/sold/storage
context, not a fifth conserved stage. Generation/discharge must not inflate
consumption. Raw water volume is context, not site energy; only an explicitly
derived energy equivalent with an energy unit can enter the energy scale.

## Links and conversion ratios

The wire collection is `links`, not `edges`. A link has `id`, `fromId`, `toId`,
`relation`, `basis`, `fromValue`, `fromUnit`, `toValue`, `toUnit`, and optional
`ratio`, `ratioKind`, `ratioLabel`, `ruleId`, `explanation`, `period`, `zoneName`,
`serviceKind`, `sourceIds`, `relatedPathIds`. There is no link `value`, `unit`,
`domain`, `scaleDomain` or `formula` field. Domain comes from its endpoint nodes;
formulas are available through relationship rules and allocation source metadata.

| Relation | Direction | Quantities |
| --- | --- | --- |
| `driver_to_load` | driver → load | Same allocated thermal contribution at both ends |
| `load_to_end_use` | load → end use | Paired actual thermal load and actual site energy; normally unequal |
| `end_use_to_carrier` | end use → carrier | Same site-energy branch at both ends |
| `direct_end_use_to_carrier` | direct end use → carrier | Supported direct-use relation with the same site/site interpretation |
| `residual` | positive unclassified residual → carrier | Additive site-energy gap, when material enough to display |
| `source_correspondence` | thermal driver → matching direct end use | Independent endpoint quantities; **non-flow** |

Upgraded legacy data can also retain a thermal `residual` → load relation.
Preserve it as legacy reconciliation context, not a new primary driver category
or a site-carrier branch. The Python example keeps it in `contextLinks`.

Conversion ratio is `fromValue / toValue`, never its inverse. For example,
100 thermal / 25 electricity gives COP 4; 85 thermal / 100 fuel gives efficiency
0.85. Use the supplied kind and label: electric cooling can advertise COP,
single combustion heating efficiency or load/fuel, district energy
load/purchased energy, and mixed-carrier service load/site energy. Electric
heating alone does not establish a heat-pump COP. Missing carrier evidence,
zero denominators or unusable temporal overlap do not justify an invented ratio.
Conversions from either marked thermal boundary use
`ratioKind=load_to_site_energy`: active-surface source/site or mixed-boundary/site
comparison, not equipment COP or efficiency. The marker does not change node or
paired-link quantities, and storage prevents treating the numerator as an
equivalent same-period Zone-air delivery.

An annual conversion can cover only the periods where both measurements exist.
Keep its paired `fromValue`/`toValue`; do not replace them with full annual node
totals. Annual ratios divide summed paired quantities, not averaged monthly
ratios. Fans, pumps and other auxiliaries are not silently added to a cooling
COP denominator. A carrier split that cannot retain a defensible conversion
must not acquire a fuel-specific ratio by inference.

Source correspondence is stored in the same `links` array, not a root
`relations` field. Lighting/equipment heat and their site energy are related
evidence, not extra flowing energy. Exclude this relation from conservation,
ratio calculations and the primary flow traversal. The UI outlines the
counterpart on selection. People have no corresponding direct site-energy node.

## Driver allocation, signs and multipliers

For each completed Zone-month and service, let `L` be actual canonical delivered
load and `H_i` an effective signed heat-balance driver pressure:

```text
Cooling pressure P_i = max(H_i, 0)
Heating pressure P_i = max(-H_i, 0)
Allocated contribution A_i = L * P_i / sum(P_j)
```

This is `basis=heat_balance_share`: a deterministic, non-causal explanation of
the measured load. It is not a direct causal decomposition. Synthetic
pre-allocation closure terms are excluded from the denominator. Positive load
with no positive pressure is assigned to Other / storage, with zero raw/effective
pressure and a nonzero allocated contribution. Rounding remainder goes to the
last stable contributor so incoming allocated contributions close to the load.
Zero actual load produces zero allocation even when raw pressure is nonzero.

Opposite-sign pressure remains `offsetEffects`: diagnostic, non-additive and
not an avoided-load or savings quantity. Simultaneous load uses completed
Zone-month pairs; annual or Building aggregation must not manufacture overlap
between different months or Zones.

Complete the allocation before aggregation. Annual/Building views sum completed
Zone-month allocations rather than allocating net annual heat again. Building
interzone presentation removes internal double counting while preserving raw
pair imbalance and closure evidence. Annual-only site energy can still be
retained when no monthly counterpart exists; twelve measured months are not
guaranteed for every source.

Source dictionary scalars retain the original signed annual net, including a
known zero when seasonal gains and losses cancel. Directional driver nodes are
gross service contributions and cannot replace that net. For a source that
solely owns each of its directional allocations, `allocatedValue` is their
sum; `allocationFormula` identifies this case. Its `allocationFactor` is category
context, not a ratio against the annual source net. A shared category does not
prove an individual source's complete attribution. The v2 writer preserves
known source and Zone-detail zeroes explicitly; stored absent/null quantities
remain unknown. This does not change the frozen v1 source writer.

For eligible native Zone quantities, apply Zone × ZoneGroup multipliers exactly
once. Facility/system/plant quantities and already model-total outputs are not
multiplied again. Surface geometry multipliers are not reapplied to these output
quantities. Inspect `effectiveMultiplier`, `multiplierApplication`,
`aggregationBasis` and warnings; an unknown multiplier left at 1 is not proof
that its applicability was established. Canonical logic lives in
[`energy_driver_allocation.go`](../cmd/semantic-idf/internal/simulation/energy_driver_allocation.go)
and [`energy_multiplier.go`](../cmd/semantic-idf/internal/simulation/energy_multiplier.go).

## Zone equipment allocation

With the default `allocationPolicy=by_service_path_load_share`, the fixed
priority is direct component/Zone observation → related HVAC service
path load share → same-service Zone load share → unassigned. Exact direct
observations, including zero, win. Subtract them from the central pool and
exclude those Zones from further allocation. Do not distribute missing direct
lighting/equipment sources arbitrarily, and never substitute equal-area or
equal-Zone shares for missing HVAC evidence.

Cooling and heating allocations use their own service paths and denominators.
Fans use related AirLoop evidence (existing supply-volume evidence when complete,
otherwise load share). Pumps use unambiguous PlantLoop paths, and heat rejection
uses related condenser/cooling-plant paths. Missing or ambiguous auxiliary paths
remain unassigned. Direct totals exceeding the central pool remain overmapped,
not silently clamped. Unassigned Building HVAC energy is not a selected Zone's
energy. Reconciliation retains direct/allocated/unassigned quantities.
The supported `direct_only` policy does not enable those central path/load
fallback allocations; do not infer allocation solely from a Zone scope.

Canonical bases are `reported_meter`, `reported_variable`,
`reported_end_use_subtotal`, `integrated_rate`, `heat_balance_share`,
`service_path_allocation`, `zone_load_allocation`, `direct_zone_energy`,
`derived_ratio`, and `residual`. A measured value and an allocated value are not
directly comparable merely because they have the same unit.

## Quality and source trace

`quality.drivers`, `loads`, `endUses`, `carriers` are availability
records with `level`, `status`, `found`, `total`, and optional `message`.
They have **no wire `percent` field**. When status and denominator establish
availability, derive `100 * found / total`; otherwise show unavailable rather
than assuming 0% or 100%. Counts describe expected source groups, not a count of
all provenance IDs. Availability is run-level requested-output coverage, not
proof that every selected month contains a measurement.

`quality.ratios` has the same record shape but different semantics: it is
selected-period service-pair availability. Candidate Zone/service pairs form
the denominator; valid source-traced conversions with usable carrier branches
form the numerator. It is not another requested-output source-group count.

Accounting is separate: `driverToLoadClosedPct`, `endUseToCarrierClosedPct`,
`zoneAllocatedPct`, `unassignedPct` each require their corresponding
`driverToLoadStatus`, `endUseToCarrierStatus`, `zoneAllocationStatus`.
Closure is `100 * max(0, 1 - sum(abs(expected - explained)) / sum(expected))`
where a denominator is established, not a thermal/site conversion efficiency.
Driver closure uses `driver_to_load` into actual loads. Carrier closure uses
`end_use_to_carrier`/`direct_end_use_to_carrier` into reported facility totals,
excluding the displayed residual branch and observed/allocated Zone subtotals.
Adding a residual drawing back into explained energy would falsely turn missing
coverage into complete closure.
`zoneAllocatedPct` currently means assigned **direct + allocated** central HVAC
share, not allocated-only share; it can exceed 100 for overmapped evidence.
This is Building-wide allocation coverage even when copied into a Zone result.
Never render an unavailable accounting zero as a measured zero.

Sources preserve raw meter/variable names, source and normalized units,
reporting frequency, aggregation, dictionary/table coordinates and input entity
references. `sourceUnit` and `normalizedUnit` are different: J → kWh is common.
Scalar source values and `scopeDetails` have no month field; `Monthly` frequency
does not make an annual source scalar suitable for filling a monthly node blank.
Use period-local node/link evidence and retain sparse unknown values.
For supported native radiant Monthly outputs, a retained dictionary identity
with NULL, missing, duplicate or invalid monthly observations is not a numeric
load series or a reported zero. Fully observed literal zero remains known zero,
including on the v2 source wire; an absent dictionary cannot establish a source
measurement. Do not fill missing `rawValue` or `effectiveValue` from that identity.

Batch v2 comparison/export is fixed to Building / Annual. Its default workbook
contains Energy Path Summary, Energy Path Delta, Data Quality and Runs. The
`Include trace sheets` option adds raw/canonical trace detail, including original
submitted Energy JSON chunks. Concatenate `json` cells by result index, part and
zero-based chunk index to reconstruct each submitted explanation/summary.
`representation` distinguishes original JSON from a typed canonical fallback.
The desktop workbook request includes a
`semantic-idf.energy-path-batch-export/v2` presentation snapshot made by the same
UI comparator; this is not a replacement graph schema or another SQL aggregation.
Null remains unknown, and each comparison side retains its own units and basis.

## Static load-factor limitations

UA, envelope/load factors, schedules and other static model properties are useful
Topology/Profile context. They are not actual equipment energy, cannot replace
measured service loads, and do not establish the causal fraction of a bill.
Actual equipment energy also depends on weather, controls, efficiency, part-load
operation and system interactions. Neither normalized driver allocation nor an
observed load/energy ratio proves the savings from changing a model parameter.

## v1 read migration

Read stored v1 data through `EnergyExplanationResult.UnmarshalJSON` and the
existing `UpgradeEnergyExplanationV1` adapter. Recognized legacy `edges` without
v2 `links` also enter that boundary. Do not merely rename the field or reverse
every arrow: relation-specific transformations map carrier → end use → load →
heat into driver → load → end use → carrier, preserve paired values/provenance,
canonicalize taxonomy and basis, and retain non-flow correspondence separately
from primary flow interpretation.

For example, legacy `measured_energy_variable` maps to `reported_variable`,
`measured_variable` to `direct_zone_energy`, and `measured_meter`/`sql_tabular`
to `reported_meter`; allocation bases retain their specific path/load meaning.
The adapter also reconciles/filters invalid flow and upgrades summary groups.
New v2 serialization emits `links`, never the deprecated Go `Edges` alias or
v1 summary aliases. Reading a manifest does not rerun EnergyPlus. Preserve
original stored data for audit; do not port a second migration algorithm into
Python. Removal of the remaining internal legacy write assembler is tracked
separately by EPATH-222.

## Small wire example

This illustrative Building/Annual fixture is not a measured model. Its thermal
drivers sum to 100, cooling uses 25 site kWh (COP 4), and direct lighting uses
25 site kWh. The 20 → 25 lighting correspondence is deliberately not a flow.

<!-- EPATH180:JSON -->
```json
{
  "schema": "semantic-idf.energy-explanation/v2",
  "purpose": "energy_explanation",
  "scope": {"kind": "building", "aggregationBasis": "model_total"},
  "frequency": "Monthly",
  "nodes": [
    {"id":"dw","level":"driver","kind":"driver.surface.exterior_walls","label":"Exterior walls","value":80,"unit":"kWh","scaleDomain":"thermal","serviceKind":"cooling","driverCategory":"surface.exterior_walls","basis":"heat_balance_share","allocationApplied":true,"rawValue":80,"effectiveValue":80,"allocatedValue":80,"period":"annual","sourceIds":["qw"]},
    {"id":"dl","level":"driver","kind":"driver.internal.lighting","label":"Lighting heat","value":20,"unit":"kWh","scaleDomain":"thermal","serviceKind":"cooling","driverCategory":"internal.lighting","basis":"heat_balance_share","allocationApplied":true,"rawValue":20,"effectiveValue":20,"allocatedValue":20,"period":"annual","sourceIds":["ql"]},
    {"id":"lc","level":"load","kind":"load.cooling","label":"Cooling load","value":100,"unit":"kWh","scaleDomain":"thermal","serviceKind":"cooling","basis":"reported_variable","period":"annual","sourceIds":["load"]},
    {"id":"ec","level":"end_use","kind":"energy.cooling","label":"Cooling equipment","value":25,"unit":"kWh","scaleDomain":"site","endUse":"cooling","serviceKind":"cooling","basis":"reported_meter","period":"annual","sourceIds":["cool"]},
    {"id":"el","level":"end_use","kind":"energy.lighting","label":"Lighting","value":25,"unit":"kWh","scaleDomain":"site","endUse":"lighting","basis":"reported_meter","period":"annual","sourceIds":["light"]},
    {"id":"ce","level":"carrier","kind":"energy.electricity.total","label":"Electricity","value":50,"unit":"kWh","scaleDomain":"site","carrier":"electricity","basis":"reported_meter","period":"annual","sourceIds":["facility"]}
  ],
  "links": [
    {"id":"a","fromId":"dw","toId":"lc","relation":"driver_to_load","basis":"heat_balance_share","fromValue":80,"fromUnit":"kWh","toValue":80,"toUnit":"kWh","period":"annual","serviceKind":"cooling","sourceIds":["qw","load"]},
    {"id":"b","fromId":"dl","toId":"lc","relation":"driver_to_load","basis":"heat_balance_share","fromValue":20,"fromUnit":"kWh","toValue":20,"toUnit":"kWh","period":"annual","serviceKind":"cooling","sourceIds":["ql","load"]},
    {"id":"c","fromId":"lc","toId":"ec","relation":"load_to_end_use","basis":"reported_meter","fromValue":100,"fromUnit":"kWh","toValue":25,"toUnit":"kWh","ratio":4,"ratioKind":"coefficient_of_performance","period":"annual","serviceKind":"cooling","sourceIds":["load","cool"]},
    {"id":"d","fromId":"ec","toId":"ce","relation":"end_use_to_carrier","basis":"reported_meter","fromValue":25,"fromUnit":"kWh","toValue":25,"toUnit":"kWh","period":"annual","serviceKind":"cooling","sourceIds":["cool"]},
    {"id":"e","fromId":"el","toId":"ce","relation":"end_use_to_carrier","basis":"reported_meter","fromValue":25,"fromUnit":"kWh","toValue":25,"toUnit":"kWh","period":"annual","sourceIds":["light"]},
    {"id":"f","fromId":"dl","toId":"el","relation":"source_correspondence","basis":"reported_variable","fromValue":20,"fromUnit":"kWh","toValue":25,"toUnit":"kWh","period":"annual","sourceIds":["ql","light"]}
  ],
  "sources": [
    {"id":"qw","sourceType":"sql_variable","sourceUnit":"J","normalizedUnit":"kWh","reportingFrequency":"Monthly"},
    {"id":"ql","sourceType":"sql_variable","sourceUnit":"J","normalizedUnit":"kWh","reportingFrequency":"Monthly"},
    {"id":"load","sourceType":"sql_variable","sourceUnit":"J","normalizedUnit":"kWh","reportingFrequency":"Monthly"},
    {"id":"cool","sourceType":"sql_meter","isMeter":true,"name":"Cooling:Electricity","sourceUnit":"J","normalizedUnit":"kWh"},
    {"id":"light","sourceType":"sql_meter","isMeter":true,"name":"InteriorLights:Electricity","sourceUnit":"J","normalizedUnit":"kWh"},
    {"id":"facility","sourceType":"sql_meter","isMeter":true,"name":"Electricity:Facility","sourceUnit":"J","normalizedUnit":"kWh"}
  ],
  "completeness": {"status":"partial"}
}
```

## Executable Python reconstruction

The following standard-library-only example reconstructs the **canonical
semantic graph**, preserving node/link fields, IDs, units and provenance. It
does not promise identical browser pixel layout. The browser subsequently applies
service filtering, carrier validation, display-only end-use/Other grouping,
stable ordering and independent-domain layout; see `energyPathGraphForState`
and `prepareEnergyPathScene` in
[`energy-path-view.js`](../cmd/semantic-idf/frontend/src/js/views/energy-path-view.js).
Display regrouping must retain original members; do not write it back as new
measured energy. The native wire has no screen coordinates or ribbon paths.

Save the fence as a Python script, feed a v2 explanation or simulation response
on stdin, and optionally pass period then Zone name as arguments. It rejects a
missing month rather than returning annual energy under a monthly label. It
requires v1 to be upgraded through Semantic IDF's existing read adapter first.

<!-- EPATH180:PYTHON -->
```python
import json
import math
import sys


def reconstruct(payload, period="annual", zone=None):
    bundle = payload.get("purposeResults", payload)
    graph = bundle.get("energyExplanation", bundle)
    if graph.get("schema") != "semantic-idf.energy-explanation/v2":
        raise ValueError("v1 input must first pass through the Semantic IDF read adapter")
    selected = graph
    if zone is not None:
        matches = [item for item in (graph.get("zoneResults") or [])
                   if item.get("scope", {}).get("zoneName", "").casefold() == zone.casefold()]
        if graph.get("scope", {}).get("kind") == "zone" and graph["scope"].get("zoneName", "").casefold() == zone.casefold():
            matches.append(graph)
        if len(matches) != 1:
            raise ValueError("zone is missing or ambiguous")
        selected = matches[0]
    key = period.lower()
    if key not in {"annual", *("m" + str(month) for month in range(1, 13))}:
        raise ValueError("period must be annual or M1 through M12")
    periods = [item for item in (selected.get("periods") or []) if item.get("id", "").lower() == key]
    if len(periods) > 1:
        raise ValueError("period is ambiguous")
    exact = periods[0] if periods else None
    if exact is not None and (key != "annual" or exact.get("nodes") or exact.get("links")):
        nodes, links = exact.get("nodes") or [], exact.get("links") or []
    else:
        nodes = [node for node in (selected.get("nodes") or []) if (node.get("period") or "annual").lower() == key]
        links = [link for link in (selected.get("links") or []) if (link.get("period") or "annual").lower() == key]
        if key != "annual" and not nodes and not links:
            raise ValueError("requested month is unavailable; annual fallback is forbidden")
    by_id = {node["id"]: node for node in nodes}
    if len(by_id) != len(nodes) or "" in by_id:
        raise ValueError("node IDs must be nonempty and unique")
    numeric = lambda value: isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)
    if any(not numeric(node.get("value")) for node in nodes):
        raise ValueError("unknown node value must not be replaced with zero")
    stages = {level: [node for node in nodes if node["level"] == level]
              for level in ("driver", "load", "end_use", "carrier")}
    directions = {"driver_to_load": ("driver", "load"),
                  "load_to_end_use": ("load", "end_use"),
                  "end_use_to_carrier": ("end_use", "carrier"),
                  "direct_end_use_to_carrier": ("end_use", "carrier"),
                  "residual": ("residual", "carrier")}
    flows, relations, context = [], [], []
    seen_links = set()
    for link in links:
        if not link.get("id") or link["id"] in seen_links:
            raise ValueError("link IDs must be nonempty and unique")
        seen_links.add(link["id"])
        if link["fromId"] not in by_id or link["toId"] not in by_id:
            raise ValueError("link endpoint is unavailable in the selected graph")
        relation = link["relation"]
        if relation == "source_correspondence":
            relations.append(link)  # Never count this as physical flow.
        elif relation == "residual" and (by_id[link["fromId"]]["level"], by_id[link["toId"]]["level"]) != ("residual", "carrier"):
            context.append(link)  # Retained legacy thermal reconciliation.
        elif relation in directions:
            start, end = by_id[link["fromId"]], by_id[link["toId"]]
            if (start["level"], end["level"]) != directions[relation]:
                raise ValueError("invalid primary direction")
            expected_domains = ("thermal", "site") if relation == "load_to_end_use" else (("thermal", "thermal") if relation == "driver_to_load" else ("site", "site"))
            if (start.get("scaleDomain"), end.get("scaleDomain")) != expected_domains:
                raise ValueError("invalid accounting domains")
            if any(not numeric(link.get(field)) or link[field] < 0 for field in ("fromValue", "toValue")):
                raise ValueError("invalid paired quantities")
            flows.append(link)  # Preserve both quantities and the supplied ratio.
        else:
            context.append(link)  # Support/unknown context is not another flow.
    return {"scope": selected["scope"], "period": "annual" if key == "annual" else key.upper(),
            "stages": stages, "flows": flows, "nonFlowRelations": relations,
            "contextLinks": context,
            "auxiliaryNodes": [node for node in nodes if node["level"] not in stages],
            "sources": graph.get("sources") or []}


if __name__ == "__main__":
    result = reconstruct(json.load(sys.stdin), sys.argv[1] if len(sys.argv) > 1 else "annual",
                         sys.argv[2] if len(sys.argv) > 2 else None)
    print(json.dumps(result, ensure_ascii=False, allow_nan=False))
```

The documentation tests validate this example's keys against the actual Go wire
types and execute the exact Python fence on canonical conversion fixtures and
on the existing v1 golden after the real Go read migration.
