# Energy Path developer handbook

Reference for Energy Path wire data, file/API consumers, physical boundaries and
regression coverage.
Use the existing Go builder for calculations and the test catalog for affected
verification; preserve source identity, physical boundaries and unknown values
when extending a feature.

## Contents

- [Wire contract](#wire-contract): [selection](#envelope-and-selection),
  [nodes](#nodes-values-and-domains), [links](#links-and-ratios),
  [allocation](#driver-allocation-and-multipliers),
  [native boundaries](#restricted-native-boundaries),
  [quality and exports](#quality-provenance-and-exports),
  [v1 migration](#stored-v1-migration),
  [wire example](#small-wire-example) and
  [Python reconstruction](#executable-python-reconstruction).
- [CLI and Python](#cli-and-python): [CLI flags](#cli),
  [response](#shared-response-and-selection),
  [input identity](#sql-and-model-identity),
  [HTTP API](#local-http-api) and [Python client](#python-client).
- [Regression map](#regression-map):
  [test selection](#choose-the-affected-tests),
  [backend contracts](#backend-numerical-and-identity-contracts),
  [browser integration](#frontend-behavior-and-cached-interaction) and
  [evidence limits](#evidence-limits-and-maintenance).
- [Native fixtures](#native-fixtures): approved coverage, independent evidence,
  [execution modes](#execution-modes) and review workflow.
- [Equipment boundaries](#equipment-boundaries):
  [District Energy](#district-energy), [Fan Coil](#fan-coil),
  [Mixed heating fuels](#mixed-heating-fuels),
  [Pool and Zone multiplier](#pool-and-zone-multiplier), [PTHP](#pthp),
  [PV and storage](#pv-and-storage), [Radiant](#radiant), [VRF](#vrf)
  and [ZoneGroup](#zonegroup).
- [Development decisions and acceptance baseline](#development-decisions-and-acceptance-baseline)
  and [historical document references](#historical-document-references).

## Wire contract

The canonical graph explains **load drivers → thermal load → end-use energy →
energy carrier** using separate thermal and site-energy scales. It is a measured
load explanation, not a single-scale Sankey balance or causal savings estimate.
See [CLI and Python](energy-path.md#cli-and-python) for file/API access and
[regression coverage](energy-path.md#regression-map) for the maintained test map.

The authoritative wire types are
[`energy_explanation.go`](../cmd/semantic-idf/internal/simulation/energy_explanation.go).
`BuildPurposeResultBundle` builds the result; the read adapter and canonical
transformations live in
[`energy_path_v2.go`](../cmd/semantic-idf/internal/simulation/energy_path_v2.go).
Consumers reuse these results rather than repeat SQL parsing, multipliers,
allocation or equipment-energy inference.

### Envelope and selection

The simulation response contains sibling objects under `purposeResults`:

| Object | Schema | Contents |
| --- | --- | --- |
| `energyExplanation` | `semantic-idf.energy-explanation/v2` | `scope`, `nodes`, `links`, `periods`, `zoneResults`, `sources`, quality, reconciliation and warnings |
| `energyExplanationSummary` | `semantic-idf.energy-explanation-summary/v2` | `scope`, `period`, `drivers`, `loads`, `endUses`, `carriers`, `ratios`, `residuals`, `topZones`, quality |

The explanation has no root `summary` or `period` scalar: its top-level graph is
Annual. `periods[]` contains `id`, `label`, `kind` and period-local nodes, links,
summary, quality, reconciliation and warnings. Interactive periods are `annual`
and `M1`–`M12`; preserve optional Building `selected_range`, `D<n>` and `H<n>`
records without making them interactive or adding Annual to monthly totals.

`scope` is `{kind, zoneName?, aggregationBasis}`. Kind is `building` or `zone`;
normal effective accounting is `model_total`. `zoneResults[]` contains already
calculated Zone graphs, summaries and periods; the source dictionary stays on
the parent explanation. `availableZones` only discovers names. A Zone scope is
not automatically one physical copy: respect its declared basis and multipliers.
Missing, ambiguous or explicitly empty scopes/periods never fall back to another
Zone, Building or Annual.

### Nodes, values and domains

| `level` | Meaning | `scaleDomain` | Typical unit |
| --- | --- | --- | --- |
| `driver` | Allocated contribution to actual service load | `thermal` | `kWh` thermal |
| `load` | Observed cooling/heating at the declared boundary | `thermal` | `kWh` thermal |
| `end_use` | Equipment or direct-use site energy | `site` | `kWh` site |
| `carrier` | Electricity, gas, district energy or another carrier | `site` | `kWh` site |

Join exact opaque node `id`s. Meaning comes from `level`, `kind`, `serviceKind`,
`driverCategory`, `endUse` and `carrier`, not parsed IDs or labels. `value` is the
main magnitude; `rawValue`, `effectiveValue`, `allocatedValue`, `displayValue`,
`signedValue`, `allocationApplied`, `basis`, `multiplier`, source IDs and related
entity/path IDs retain separate evidence. Explicit allocated zero must not fall
back to raw pressure. Missing/null numeric fields remain unknown; pruned zero
summary categories cannot be treated as reported zero comparison values.

Use independent linear thermal/site width scales even when both units say kWh.
A 100 thermal → 25 electricity conversion is valid; its width difference is not
a 75 kWh reconciliation error and pixel-width ratios are never COP.
Carrier-neutral end uses can branch to multiple carriers: each branch keeps its
own source IDs, and facility totals are not added to their end-use components.
`support` nodes / `support_supply` links retain purchased, produced, sold and
storage context rather than a fifth conserved stage. Generation/discharge do not
inflate consumption. Raw water volume enters no energy scale unless an explicit
energy equivalent with an energy unit has been derived.

Load `loadBreakdown`, `offsetEffects`, `latentShare` and `simultaneousLoad` are
non-additive details. Optional `thermalBoundary` identifies:

| Value | Boundary and interpretation |
| --- | --- |
| `active_surface_source` | Native fluid heat inserted into/removed from a radiant surface, already model-total; not same-period Zone-air delivery |
| `mixed_thermal_boundaries` | Combined active-surface and air/other components; not one Zone-air measurement boundary |

An unset marker retains the existing contract. Do not infer it from source names,
add active-surface heat again as delivered load, or decompose combined radiant
heat into invented sensible/latent observations. Surface storage and exchange
can shift the Zone-air response between periods.

### Links and ratios

The wire collection is `links`, not `edges`. Required link data is `id`,
`fromId`, `toId`, `relation`, `basis`, `fromValue`, `fromUnit`, `toValue`,
`toUnit`. Optional evidence includes `ratio`, `ratioKind`, `ratioLabel`, `ruleId`,
`explanation`, `period`, `zoneName`, `serviceKind`, `sourceIds`, `relatedPathIds`.
There is no link `value`, `unit`, `domain`, `scaleDomain` or `formula`: domains
come from nodes, formulas from relationship rules / source allocation metadata.

| `relation` | Direction | Quantities |
| --- | --- | --- |
| `driver_to_load` | driver → load | Same allocated thermal contribution at both ends |
| `load_to_end_use` | load → end use | Paired actual thermal/site quantities, normally unequal |
| `end_use_to_carrier` | end use → carrier | Same site-energy branch at both ends |
| `direct_end_use_to_carrier` | direct end use → carrier | Same site/site interpretation |
| `residual` | positive unclassified residual → carrier | Material additive site-energy gap |
| `source_correspondence` | thermal driver → matching direct end use | Independent endpoint quantities; **non-flow** |

Legacy thermal residual → load links remain reconciliation context, not primary
drivers or site-carrier branches. Source correspondence stays in `links`; exclude
it from conservation, ratios and primary traversal. Lighting/equipment heat and
site energy are related evidence; People has no matching direct energy node.

Ratio is `fromValue / toValue`: cooling 100/25 gives COP 4; heating 85/100 fuel
gives efficiency 0.85. Honor the supplied kind/label: district load/purchased
energy and mixed-carrier load/site energy do not establish equipment efficiency.
Electric heating alone proves no heat-pump COP. Missing carrier evidence, zero
denominators or unusable time overlap permit no invented ratio. Marked radiant
or mixed-boundary conversions use `ratioKind=load_to_site_energy`, retaining
both quantities rather than claiming COP, efficiency or Zone-air equivalence.

An annual conversion uses only overlapping measured periods. Preserve its paired
quantities even if full annual node totals are larger. Annual ratios divide
summed pairs, never averaged monthly ratios. Auxiliary fan/pump energy is not
silently added to a cooling COP denominator; unsupported carrier splits receive
no inferred fuel-specific ratio.

### Driver allocation and multipliers

For each Zone-month and service, let `L` be actual canonical load and `H_i` an
effective signed heat-balance driver pressure:

```text
Cooling pressure P_i = max(H_i, 0)
Heating pressure P_i = max(-H_i, 0)
Allocated contribution A_i = L * P_i / sum(P_j)
```

`basis=heat_balance_share` is deterministic and non-causal. Synthetic closure
terms do not enter the pressure denominator. Positive load without positive
pressure goes to Other/storage with zero raw/effective pressure. Zero actual
load gives zero allocation even with nonzero raw pressure. At three-decimal
transport precision, largest fractional remainders apportion the fixed budget:
each driver owns its floor/ceil quota, ties use stable semantic identity and
incoming contributions close without assigning all rounding to the last driver.
Outside the representable milli-unit budget range, legacy allocation remains.

Allocate **before** aggregating: Annual/Building sums completed Zone-month
allocations, never reallocates net annual heat. Building interzone presentation
removes internal double counting while preserving raw imbalance and closure.
Opposite-sign `offsetEffects` are non-additive diagnostics, not avoided loads or
savings. Simultaneous load combines completed Zone-month pairs; no overlap may
be manufactured between different Zones or months. Annual-only site energy can
remain when no monthly counterpart exists.

Source scalars retain signed annual net, including seasonal cancellation to zero.
Directional drivers are gross service contributions and do not replace that net.
A source solely owning each directional allocation receives their summed
`allocatedValue` and an identifying `allocationFormula`; its `allocationFactor`
is category context rather than a ratio against net zero. Shared-category
evidence proves no individual complete attribution. The v2 writer preserves
known source / Zone-detail zeroes while missing/null stays unknown; the frozen
v1 source writer is unchanged.

Apply Zone × ZoneGroup once to eligible native representative-Zone outputs.
Already model-total, facility, system and plant quantities get no second factor;
geometric surface multipliers are not reapplied. Inspect `effectiveMultiplier`,
`multiplierApplication`, `aggregationBasis` and warnings: a conservative unknown
factor left at 1 is not verified applicability. Logic lives in
[`energy_driver_allocation.go`](../cmd/semantic-idf/internal/simulation/energy_driver_allocation.go)
and [`energy_multiplier.go`](../cmd/semantic-idf/internal/simulation/energy_multiplier.go).

### Zone equipment allocation

Default `allocationPolicy=by_service_path_load_share` uses this priority:

1. Exact component/Zone observations, including zero.
2. Related HVAC service-path load share.
3. Matching-service Zone load share.
4. Unassigned.

Subtract direct observations from their central pool and exclude those Zones
from that same undifferentiated pool's subsequent allocation. Separately observed
shared components retain their own budgets and can serve the same Zone. Missing
lighting/equipment observations authorize neither equal-area/equal-Zone shares
nor invented direct use. `direct_only` disables central path/load fallbacks.
Cooling/heating have separate paths and denominators. Fans use complete related
AirLoop supply-volume evidence, otherwise load share; pumps require unambiguous
PlantLoop paths; heat rejection uses related condenser/cooling paths. Missing or
ambiguous paths stay unassigned. Direct totals exceeding the pool stay overmapped
rather than clamped. Building unassigned HVAC energy does not belong to a
selected Zone; reconciliation retains direct/allocated/unassigned amounts.

Reviewed native equipment contracts:

| Equipment / evidence | Required accounting boundary |
| --- | --- |
| Electric radiant-convective / convective baseboards | Exact typed EquipmentList/EquipmentConnections Zone ownership binds native Baseboard Electricity Energy, already model-total. Baseboard Total Heating Energy/Rate is overlapping response context, not added load or inferred electricity. Radiant recipient exchange remains mixed HVAC/environment; convective-only types acquire no radiant recipients. Invalid selected capacity is not repaired through defaults. |
| EnergyPlus 25.1 WindowAirConditioner | Exact original Fan:OnOff, single-speed DX coil, mixer and Zone air nodes bind coil, crankcase and fan electricity, already model-total. Package electricity overlaps, not another consumer; fan end use stays outside coil-only ratios. Unresolved paths borrow neither another package nor central service paths. |
| HotWaterEquipment | District-water consumption is InteriorEquipment, not space Heating. Representative energy receives Zone/Group factors once; Facility / InteriorEquipment district meters are separate model-total reconciliation views. Modern/legacy aliases are not summed and cannot replace observed monthly zero. |
| Native baseboard plus central boiler ancillary electricity | Allocate each consuming component's own budget. Exact typed local delivery paths exclude unrelated paths on a shared node. An owned baseboard cannot exclude its Zone from a separately verified central boiler; unidentified meter remainder stays unassigned. Hourly/Rate companions are trace, not another monthly budget. |

Shared source raw/effective values remain actual Building component totals;
Zone source details report allocated shares rather than fabricated measurements.
Local direct and central allocated consumption retain separate carrier branches
even with the same carrier. Their combined node/conversion uses conservative
allocated basis, preserving the original consumption, shares and single load.

At Building scope, carrier-qualified contributions may share one physical Zone
load: count that paired thermal load once and retain only contributing source
identities. An unserved plenum can remain in the larger load node without proving
served HVAC consumption. Aggregation/readback must not replace paired thermal
values with the whole node total.

Canonical bases are `reported_meter`, `reported_variable`,
`reported_end_use_subtotal`, `integrated_rate`, `heat_balance_share`,
`service_path_allocation`, `zone_load_allocation`, `direct_zone_energy`,
`derived_ratio` and `residual`. Equal units do not make measured and allocated
values interchangeable.

### Restricted native boundaries

#### Storage charge

Optional support-node `storageChargeBoundaries[]` records contain
`schema=semantic-idf.storage-charge-boundary/v1`, `state`, `sourceKey`,
`originalVersion`, `busType`, `reason`, optional `objects[]`, `sourceIds[]`.
Objects use original physical `objectType`, `objectName` and unique nonnegative
`objectIndex`, not executed offsets or Output indices. Missing/null index is
invalid; explicit zero is valid.

`state=native_storage_non_consumption` / reason
`native_charge_is_not_facility_consumption` covers reviewed 25.1 Simple,
Battery/KiBaM and LiIonNMCBattery on AlternatingCurrentWithStorage,
DirectCurrentWithInverterACStorage or DirectCurrentWithInverterDCStorage buses.
Native Charge Energy is unmetered transfer; distinct Production Decrement
already participates in ElectricityProduced. Charge adds neither a Facility end
use nor a supply ribbon. Genuine inverter ancillary electricity is separate.

`unresolved_native_storage_boundary` retains `unreviewed_original_version`,
`unresolved_storage_reporting_owner`, `unresolved_storage_distribution`,
`unbound_source_key_in_original_model` or
`unproved_native_charge_reporting_identity`. A present malformed/unknown record
denies flow and diagnoses it even after readback; it cannot become legacy
consumption. Omitted-original legacy payloads without annotation keep their
compatibility behavior. Internal class is `storage_charge_context`, not `meter`;
SQL metadata / observations are unchanged.

Requests retain key/frequency availability, including absent AC versus observed
DC and zero versus unknown. Annual metadata can union pruned-zero constituents
without counting their quantities/IDs as numeric contributors. These records
prove no battery efficiency, SOC energy, full circuit balance, Zone thermal
allocation or charge-minus-discharge heat equality.

#### Pool and non-Zone plant demand

A SwimmingPool:Indoor is process-water demand coupled to an original floor,
not a ZoneHVAC device. Reviewed 25.1 Pool water-heating / boiler thermal-output
Energy/Rate are non-additive context. Boiler fuel, ancillary fuel/electricity
and each pump have distinct already model-total identities. Zone-air/surface
outputs still use original Zone/Group factors once. Thermal context supplies no
extra Zone load or inferred fuel split.

Positive source-local allocation needs complete original typed water/air
ownership: components/ports, branch/splitter/mixer rosters, outdoor-air paths,
terminal/ADU/Zone connections and observed Monthly Energy. Partial demand census
or unquantified Pool demand denies whole-loop Zone-air allocation. Purchased
Heating and hot-water pumps remain observed but process/space splits unassigned.
An independently proved chilled-water pump allocates only its own electricity
across its complete served cooling roster, never a hot-water pump or broad Pumps
meter. Annual sums monthly shares; humidity detail cannot fill unknown primary
loads.

Optional `serviceBoundaryRestrictions` are semantic denial records: `reason`,
`endUse`, `serviceKind`, `carrier`, actual Pool demand, and when known,
`plantLoopName`, `consumerSourceIds`, `meterSourceIds`. Reasons are
`non_zone_demand_unquantified` and `demand_topology_incomplete`. Unresolved IDs
can be absent; unsupported fuels retain topology-only denial without invented
observations. IDs prove no complete topology; unknown and measured-zero
contributors preserve denial through merge, periods, scopes and JSON.
Affected Heating has no `load_to_end_use` link or cached/summary ratio even
with positive purchased Heating and Zone load. Actual site/carrier ribbons and
unrelated cooling remain. A proved chilled-water projection does not inherit a
hot-water denial merely by sharing Pumps. V1 without private allocation evidence
remains conservative; serialization restores no unsupported generic allocation.

### Quality, provenance and exports

Availability records `quality.drivers`, `loads`, `endUses`, `carriers` have
`level`, `status`, `found`, `total`, optional `message` and **no `percent` field**.
Only established status/denominator permits `100 * found / total`; otherwise it
is unavailable. Counts measure run-level expected requested source groups, not
provenance IDs or selected-month observations. `quality.ratios` uses the same
shape but counts valid source-traced, usable conversions among candidate
selected-period Zone/service pairs.

Accounting is separate:

| Percentage | Required status / denominator |
| --- | --- |
| `driverToLoadClosedPct` | `driverToLoadStatus`; `driver_to_load` into actual loads |
| `endUseToCarrierClosedPct` | `endUseToCarrierStatus`; end-use branches into reported facility totals, excluding residuals and Zone subtotals |
| `zoneAllocatedPct`, `unassignedPct` | `zoneAllocationStatus`; central Building HVAC pool |

Closure is `100 * max(0, 1 - sum(abs(expected - explained)) / sum(expected))`
with an established denominator, not conversion efficiency. Adding the residual
back to explained energy falsely closes missing coverage. `zoneAllocatedPct`
means assigned **direct + allocated**, can exceed 100 when overmapped, and remains
Building-wide coverage when copied into a Zone. Unavailable accounting zero is
not measured zero.

Sources preserve native meter/variable names, source/normalized units (often
J → kWh), frequency, aggregation, dictionary/table coordinates and input entity
references. Scalar source values / `scopeDetails` have no month field; Monthly
frequency cannot fill a period-local node blank with an annual scalar. Native
radiant dictionary identity with NULL, absent, duplicate or invalid observations
proves no measured series or zero. Fully observed literal zero stays zero; absent
dictionary identity establishes no source measurement.

Batch comparison/export is Building / Annual. Default workbook sheets are Energy
Path Summary, Energy Path Delta, Data Quality and Runs. Optional trace sheets
retain raw/canonical data and original submitted JSON. Reconstruct each JSON part
by result index, part and zero-based chunk index; `representation` distinguishes
original JSON from typed canonical fallback. The desktop
`semantic-idf.energy-path-batch-export/v2` presentation snapshot uses the same UI
comparator, not another graph or SQL aggregate. Each comparison side retains its
units/basis and null stays unknown.

Static UA, load factors, schedules and Topology/Profile properties are context,
not equipment energy or causal fractions of a bill. Weather, controls,
efficiency, part-load operation and system interactions still matter; driver
shares and observed ratios prove no savings from changing a parameter.

### Stored v1 migration

Read v1 through `EnergyExplanationResult.UnmarshalJSON` /
`UpgradeEnergyExplanationV1`; recognized legacy `edges` without v2 `links` enter
that same boundary. Relation-specific transformations preserve paired quantities,
provenance and non-flow correspondence while canonicalizing taxonomy/basis.
Renaming fields or reversing every arrow is insufficient.

Legacy `measured_energy_variable` → `reported_variable`, `measured_variable` →
`direct_zone_energy`, and `measured_meter` / `sql_tabular` → `reported_meter`;
path/load allocation bases remain specific. The adapter filters invalid flow and
upgrades summaries. New v2 writes emit `links`, never `Edges` or v1 summary
aliases. Manifest reading never reruns EnergyPlus. Preserve original audit data
and reuse the Go adapter rather than port migration into Python.

### Small wire example

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

### Executable Python reconstruction

The following standard-library-only example reconstructs the **canonical
semantic graph**, preserving node/link fields, IDs, units and provenance. It
does not promise identical browser pixel layout. The browser applies scope/period projection, carrier validation,
display-only end-use/Other grouping, stable ordering and independent-domain layout; see `energyPathGraphForState`
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

## CLI and Python

Read an existing EnergyPlus run through the same Go loader, v2 builder and CSV
formatter used by desktop `LoadEnergyPath`. Loading is read-only: it neither
changes the open model nor discovers outputs, edits run files or runs EnergyPlus.
See the [schema](energy-path.md#wire-contract) for graph/calculation contracts.

### CLI

```powershell
semantic-idf energy-path .\run --input .\run\model.idf --format json
semantic-idf energy-path .\run\eplusout.sql --input .\model.epjson --period M1 --service cooling
semantic-idf energy-path .\run --scope zone --zone Core_bottom --period annual --format csv
semantic-idf energy-path .\run --format csv --include-trace -o .\energy-path-trace.csv
```

If the executable is not on PATH, use its actual packaged path, for example
`.\build\bin\semantic-idf-vX.Y.Z.exe`. Optional `cli` prefix also works. Options
can precede/follow the result path; quote paths and Zone names containing spaces,
and use `--` before a positional path beginning with `-`.

| Argument | Meaning / default |
| --- | --- |
| Result path | Exactly one existing run directory or SQL file |
| `--input` | That run's IDF/epJSON; optional only when verified metadata resolves it |
| `--scope` | `building` (default) or `zone` |
| `--zone` | Exact Zone name, required for Zone scope |
| `--period` | `annual` (default) or `M1`–`M12` |
| `--service` | `all` (default), `cooling` or `heating` |
| `--format` | `json` (default) or `csv` |
| `--include-trace` | Source/link CSV rows; JSON already contains provenance |
| `-o`, `--output` | Report file; omitted or `-` writes stdout |

Models/SQLite are file inputs, not stdin payloads. Report output cannot overwrite
selected SQL, input or run metadata, including hardlink/symlink aliases; an
explicit unrelated report file can be replaced.

### Shared response and selection

| JSON field | Contract |
| --- | --- |
| `selection` | Normalized Scope, Zone, Period and Service |
| `purposeResults` | Unchanged GUI builder payload (`energyExplanation`, `energyExplanationSummary`), retaining Annual/monthly, Building/Zone and full source dictionary |
| `view` | Selected canonical nodes, links, sources, summary, quality, reconciliation and warnings; data projection rather than browser layout/grouping |
| `provenance` | Resolved SQL/model paths, model hash and input/historical-plan verification; not a hash of SQL contents |

Missing month/Zone never falls back to Annual/Building; an explicitly empty graph
stays empty. Selection preserves source IDs, units, measurements and both
conversion quantities; source correspondence remains non-flow. Annual source
scalars stay full-run metadata, never become monthly observations.

Cooling/Heating selection retains full carrier totals as context rather than
claiming service-specific meter observations. Quality/reconciliation belongs to
the selected scope/period, not a new service-subset balance. A Zone subtotal does
not establish a reported facility denominator for carrier closure.

CSV starts with Drivers, Loads, End uses, Carriers, Ratios and Quality summary
rows. Optional trace adds source/normalized units, basis, source references and
paired link quantities. Unknown coverage denominators/accounting stay blank with
explicit status; they are not measured zero. JSON retains the structured graph.

### SQL and model identity

Explicit SQL means that file: missing/unreadable SQL never substitutes a sibling.
Pass an exact SQL path to resolve directory ambiguity. Metadata is accepted only
when it belongs to the selected output. The read-only SQLite loader rejects WAL
mode and journal/shared-memory sidecars; provide a checkpointed, non-WAL snapshot.
Closing a connection does not change persistent WAL mode, and reading cannot
discard pending observations or create shared-memory artifacts.

SemanticIDF hashes the actual run-copy model, including added output requests.
If manifest identity differs from `--input`, select that run copy rather than an
original with different requests. Models changed during loading are rejected.
External SQL/model pairs without verified metadata retain observed data but
cannot claim verified provenance or known requested-output coverage. The loader
does not invent a historical output plan from the current model.

### Local HTTP API

`POST /api/energy-path` accepts one JSON object. Paths are on the API host; this
endpoint does not upload files or read the caller's remote filesystem.

```json
{
  "resultPath": "C:/runs/office/eplusout.sql",
  "inputPath": "C:/runs/office/model.idf",
  "scope": "zone",
  "zone": "Core_bottom",
  "period": "M1",
  "service": "cooling",
  "format": "json",
  "includeTrace": false
}
```

CLI defaults apply. JSON returns the shared projection; `format: "csv"` returns
UTF-8 CSV. Unknown fields, invalid choices/paths, multiple objects and bodies
larger than 64 KiB fail. This is the existing local app API.

### Python client

[`semantic_idf_client.py`](../clients/python/semantic_idf_client.py) uses only
Python's standard library. Make `clients/python` importable (for example via
`PYTHONPATH`). HTTP requires the local app API; stdio requires only the executable.

```python
from semantic_idf_client import SemanticIDFClient

client = SemanticIDFClient("http://127.0.0.1:34115")
result = client.energy_path(
    "C:/runs/office", input_path="C:/runs/office/model.idf",
    scope="zone", zone="Core_bottom", period="M1", service="cooling",
)

same_result = SemanticIDFClient.energy_path_stdio(
    "C:/tools/semantic-idf.exe", "C:/runs/office",
    input_path="C:/runs/office/model.idf",
    scope="zone", zone="Core_bottom", period="M1", service="cooling",
)

csv_text = client.energy_path("C:/runs/office", output_format="csv", include_trace=True)
```

Both transports return a dictionary for JSON and exact decoded CSV text for CSV;
`timeout` is seconds, default 120. Stdio passes a shell-free argument list and
raises `subprocess.CalledProcessError` with captured stderr. HTTP uses normal
`urllib` errors. Neither client recalculates aggregates, migrates v1 or infers
allocation; use the Go boundary and the schema's reconstruction example.

## Regression map

Use this map when changing Energy Path calculations, transport or presentation.
The [schema](energy-path.md#wire-contract) defines the contract;
[test selection](testing.md) defines how to run the relevant area. This document
maps current assertions rather than preserving development checkpoints.

Tests remain beside their Go packages. Synthetic IDF/SQL fixtures exercise the
builder and browser fixtures exercise the actual app/modules, but neither proves
an EnergyPlus model run. Real-run qualification, independently reviewed numeric
expectations and saved-run acceptance are documented in
[real-model verification](energy-path.md#native-fixtures) and the equipment
verification sections below.

### Choose the affected tests

```powershell
.\dev.bat test -Plan
.\dev.bat test -Area energy-wire -Plan
.\dev.bat test -Area energy-wire
```

Feature areas include `energy-drivers`, `energy-allocation`, `energy-accounting`,
`energy-hvac`, `energy-auxiliary`, `energy-generation`, `energy-pools`,
`energy-quality`, `energy-wire` and `energy-oracle`. `energy-path` selects all
Energy Path layers, including browser and heavy regression tests. The catalog
in `scripts/test-groups-*.json` is authoritative; a focused prefix below locates
assertions, not a substitute for the relevant area or changed-file selection.

### Backend numerical and identity contracts

Files below are in `cmd/semantic-idf/internal/simulation`.

| Change / contract | Representative assertions and fixtures |
| --- | --- |
| Graph directions | `TestEPATH190CanonicalPrimaryDirectionsAcrossEveryScopeAndPeriod`: exact driver → load → end use → carrier across Building/Zone and Annual/months; unique IDs, both HVAC services, non-HVAC direct uses, non-flow correspondence. |
| Conversion quantities | `TestEPATH121GeneratedDualValueConversionsAndTracesSurviveTwoReloads`: cooling 100 thermal / 25 site = COP 4; gas heating 85 / 100 = 0.85; independent units, domains, source identities and JSON reloads. |
| Monthly-first aggregation | `TestEPATH192WallContributionsAggregateTwelveMonthsBeforeAnnual`: signed wall annual net 0, heating/cooling gross contributions 38 each. Source-accounting tests retain known zero and reject incomplete individual attribution. |
| Multipliers and authority | EPATH-051 / 052 / 070 / 071: Zone 2 × ZoneGroup 5 = 10 once; already model-total outputs stay factor 1; Energy wins over Rate, including zero; physical tiers never double count Zone/system/plant load. |
| Surface identity | `TestEPATH194SurfaceSourcesUseActualAnalyzedTopology`: 13 surface/opening categories, exact parsed Topology ownership and unresolved warning; raw values/units/Zone survive readback. |
| Overlapping driver evidence | EPATH-060 / 061 and outdoor-air mapping: physical detail + separate residual closes once to aggregate. Internal components exclude totals/aliases; infiltration + ventilation do not also consume the outdoor-air aggregate. |
| Allocation closure and rounding | EPATH-080 / 081 / 120: completed Zone-month service allocations close before aggregation; zero load differs from zero pressure; Other/storage fallback, offsets and local simultaneous loads remain distinct. Quantum tests apportion each driver's quota and fixed three-decimal budget. |
| End uses and carrier balance | EPATH-090 / 091 / 111: Heating 120 branches electricity 70 / gas 50; unknown end uses collapse without loss. Facility 100 = consumption 90 + residual 10 excludes generation 25 and discharge 8. |
| Direct versus central Zone energy | EPATH-094 / 100: exact direct observation wins; Office electricity 30 remains direct while gas 10 is separately allocated. Related path denominators exclude other services; unassigned 0.001 remains Building-only. |
| Auxiliaries | EPATH-101: fan 60/40 uses related AirLoop Zones once; pump cooling 75/25 and heating 20/80 retain their own plants/services. Ambiguous paths remain unassigned; annual allocations sum months. |
| Wire and consumer reconstruction | EPATH-180 executes the exact schema JSON/Python fences against Go wire types, actual v2 conversions, the frozen v1 golden after Go migration, strict missing-month handling and Zone source dictionaries. |
| Native equipment boundaries | Baseboard, radiant, WindowAC, HotWaterEquipment, storage-charge and Pool tests retain source-local ownership, response/context distinctions and denial of unsupported conversions. See schema boundary sections. |

Start with these test files when locating the corresponding implementation:

- Directions, conversions and source nets: `energy_path_direction_epath190_test.go`,
  `energy_path_conversion_epath121_test.go`, `energy_path_monthly_aggregation_epath192_test.go`,
  `energy_path_source_accounting_epath192_test.go`.
- Drivers and loads: `energy_canonical_pipeline_test.go`,
  `energy_load_selection_acceptance_test.go`, `energy_building_load_aggregation_acceptance_test.go`,
  `energy_path_surface_categories_epath194_test.go`, `energy_driver_acceptance_review_test.go`,
  `energy_driver_mapping_test.go`.
- Allocation: `energy_driver_contribution_allocation_acceptance_test.go`,
  `energy_driver_allocation_quantum_test.go`, `energy_offset_simultaneous_acceptance_test.go`,
  `energy_path_driver_links_epath120_test.go`, `energy_zone_direct_use_audit_epath094_test.go`,
  `energy_service_path_allocation_audit_epath100_test.go`,
  `energy_auxiliary_zone_allocation_audit_epath101_test.go`.
- Accounting and wire: `energy_end_use_carrier_split_review_test.go`,
  `energy_end_use_taxonomy_audit_test.go`, `energy_carrier_reconciliation_audit_epath111_test.go`,
  `energy_path_schema_docs_epath180_test.go`.

These files also contain negative cases, order invariance, source-provenance and
stored-payload assertions; keep those when changing a formula or fixture.

### Frontend behavior and cached interaction

Files below are in `cmd/semantic-idf/internal/frontendchecks`. Tests use fresh,
isolated headless browser profiles and synthetic completed-run data. Tests of a
pure resolver/model establish that helper's contract; actual-app tests establish
integration through the app shell and delegated handlers.

| Behavior / retained module contract | Test files / assertions |
| --- | --- |
| Primary controls | `energy_path_controls_epath200_browser_test.go`: fresh Building/Annual; Scope and Period selects; Zone search appears only in Zone scope; Annual and 12 months; absent months stay empty; both services remain visible. Service, KPI and old subview controls are absent. No Analyze/Run calls or result mutations. |
| Four stages and dual scales | `energy_path_layout_epath142_browser_test.go`, `energy_path_ribbons_epath143_browser_test.go`: visible/hit-testable nodes, direct-use lane, thermal/site divider and independently scaled conversion ribbons; pure geometry companions cover calculations. |
| Ordering, grouping and selection | `energy_path_interaction_epath145_browser_test.go`, `energy_path_other_grouping_epath123_browser_test.go`: fixed taxonomy; directed path dimming; strict <1% grouping preserves exact-1% and protected categories, carrier branches, members and source identities. |
| Native keyboard input | `energy_path_keyboard_epath201_browser_test.go`: trusted Tab/Shift+Tab, Enter, Space and Escape; exact node/link targets and focus; selection retains graph DOM, layout/projection counters and completed result. |
| Component chart | `energy_path_inspector_epath150_browser_test.go`, `energy_path_component_chart_browser_test.go`: every visible node/link exposes Monthly/Hourly; old information/action sections are absent. Paired thermal/site values remain separate; real zero differs from missing; source identity, shared calendar and proven multiplier govern Hourly data. Monthly-only data creates no Hourly curve. Preference/focus restore without rebuilding the flow graph. |
| Quality / Output module contracts | `energy_path_quality_epath131_browser_test.go`, `energy_path_output_requests_epath131_browser_test.go`: isolated module fixtures distinguish availability/accounting and missing/not-requested evidence; exact type/key/frequency/index qualifies the retained Output resolver. Derived/ambiguous evidence cannot invent a request. These helper checks do not require visible Output actions in the current app. |
| Auxiliary coverage | `energy_path_auxiliary_allocation_browser_test.go`: direct / allocated / unassigned remain separate, with period denominators and overmapped truth; selected Zones acquire no unassigned Building energy. |
| History and restored focus | `energy_path_history_epath204_browser_test.go`, `energy_path_history_semantic_epath204_browser_test.go`: scope/period commit once before mutation; Back/Forward restores context, chart/drawer and exact focus. No-op/invalid/typing events retain graph and history; a retained analyzed Zone cannot overwrite restored Energy selection. |
| Workspace and redraw cache | `energy_path_workspace_epath160_browser_test.go`, `energy_path_redraw_epath161_browser_test.go`: cold Settings/Tools/Batch returns restore valid saved results; changed input or stale selection cannot invent results or rerun analysis. Selection actions retain graph geometry in unchanged context. |

Topology/Profile/HVAC/Output resolver fixtures test retained shared adapters,
not the presence of navigation actions in the current app. Keep them when changing
those helpers without restoring removed inspector actions, old five-section
component details or Service/KPI controls.

### Evidence limits and maintenance

- A passing synthetic graph, render or source compiler is not real-model numeric
  acceptance. Capture/oracle diagnostics, candidate coverage and approved expected
  comparisons are separate gates; a valid snapshot hash alone proves no result.
- Preserve native unknowns, observed zeroes, ownership, calendar and version
  restrictions in negative fixtures. Do not broaden accepted boundaries merely
  to make a candidate pass.
- Update this map when behavior changes; do not retain obsolete UI requirements,
  development dates, temporary screenshots, local timing logs or old pass claims.
  Exact test discovery and costs belong to the test catalog, and model approval
  identities belong to their fixture manifests / model-specific evidence.

## Native fixtures

The checked-in catalog contains 20 original fixtures: 19 have separately reviewed
expected headers and lossless metric companions; `no-heating-25-1` (DataCenter)
remains a rejected capture. The approved SimpleVentilation fixture is the
distinct `no-heating-ventilation-25-1` input, not a repaired or relabeled
DataCenter result.

The [acceptance baseline](#development-decisions-and-acceptance-baseline) records
the last complete native replay; later changes require their own affected checks.

### Where authoritative evidence lives

Under
[`internal/simulation/testdata/energy_path_real_models`](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models):

| Artifact | Purpose |
| --- | --- |
| `catalog.json` | Stable fixture/version identity, original model hashes, source release/license, controlled weather, coverage tags and recipe/expected paths |
| `models/<version>/*.idf`, `licenses/*` | Unmodified official distribution bytes |
| `oracles/<fixture>.json` | Independently reviewed native-source/ownership/selection declarations; no candidate-derived expectations |
| `expected/<fixture>.json` | Separately authored approval header and provenance |
| `expected/<fixture>.metrics.json.gz` | Full lossless metric array bound by count, key registry and compressed/uncompressed SHA-256 |
| Ignored `.runtime/energy-path-acceptance/` | Closed native captures, manifests, SQL/MTD, candidates, diagnostics and pending-review evidence |

The catalog and approved headers are authoritative for exact original identities
and checksums; do not duplicate or rewrite their evidence during doc maintenance.
Each equipment reference below links its recipe/header and retained capture.

All examples use controlled
`USA_IL_Chicago-OHare.Intl.AP.725300_TMY3.epw`, SHA-256
`c7d4efcf93ba316a1d874352e743df5cf137ba5c0e3459eb2dc4b5442d5b7f5c`.
This is a test climate, not each example's native location or design certification.
Runtime annual controls and additive output requests affect separate copies
only. Original, annualized and executed inputs retain different paths, hashes
and change records. Do not run outputs into checked-in `models/` or installed
`ExampleFiles/`.

### Approved coverage and equipment references

Metric counts are the full independent expectation arrays, not selected summaries.

| Fixture | Metrics | Development reference |
| --- | ---: | --- |
| Large Office 25.1 | 46,224 | Six of nineteen Zones have factor 10; sixteen conditioned owners, three unserved plenums; four measured 5/5/5/1 AirLoop fan pools |
| Large Office 22.1 / 23.2 / 24.2 | 46,224 each | Separate official originals and native/version-specific provenance; compatibility does not replace 25.1 model coverage |
| Small Office 25.1 | 17,675 | Five exact DX/gas supply paths and one-Zone fan pools; Attic has no fabricated direct-use owner |
| Ideal Loads 25.1 | 17,121 | Annual-only Tabular district consumption; Monthly electricity and non-additive latent supply context |
| PTAC 25.1 | 17,021 | Native DX/Fuel consuming cohorts, cooling crankcase and real zero constituents |
| PTHP 25.1 | 17,113 | [DX heating/defrost/crankcase and supplemental gas](energy-path.md#pthp) |
| Fan Coil 25.1 | 8,966 | [Hydronic service ownership, estimated pumps, zero-pressure fallback](energy-path.md#fan-coil) |
| VRF 25.1 | 16,360 | [Local/shared constituent budgets, all-owner denominator and reservation](energy-path.md#vrf) |
| Radiant 25.1 | 9,615 | [Active-surface thermal boundary and mixed plant auxiliaries](energy-path.md#radiant) |
| District Energy 25.1 | 17,001 | [Purchased-energy ratios, multiple paths and mixed-fan nonallocation](energy-path.md#district-energy) |
| Mixed Heating Fuels 25.1 | 17,275 | [Native baseboards plus central gas service without duplicate delivered load](energy-path.md#mixed-heating-fuels) |
| ZoneGroup 25.1 | 21,413 | [Native WindowAC/baseboards and separate Zone/Group factors](energy-path.md#zonegroup) |
| Zone multiplier / Pool 25.1 | 16,976 | [Process-heating exclusion and joint cooling-pump budget](energy-path.md#pool-and-zone-multiplier) |
| PV / storage 25.1 | 16,523 | [Native electrical equations, Cogeneration and source apportionment](energy-path.md#pv-and-storage) |
| Furnace / no cooling 25.1 | 8,965 | Actual zero cooling, positive heating/fans; source absence and finite no-service classification |
| SimpleVentilation / no heating 25.1 | 9,101 | Separate ventilation owners, direct fan consumption and observed zero delivered conditioning; ratio quality not applicable |
| Simultaneous central heat pump 25.1 | 17,074 | Same-time water-side operating evidence, component lineage and carrier-qualified reservations |

DataCenter's two Severe messages are retained as rejected native evidence.
A separate root-finding trial also failed. Exit code zero, dictionary names,
monthly heating/cooling coexistence and nearby model similarity cannot
override engine errors or prove another model's acceptance.

### Shared contracts discovered by native fixtures

**Reporting calendar and identity.** Weather environment excludes design/sizing
and explicit warmup. Aggregate Hourly/Daily/Monthly rows can have documented NULL
WarmupFlag; unknown Timestep flags remain invalid. Full annual proof checks
twelve complete month endpoints and cumulative SimulationDays, including
Monthly-only SQL. Run Period energy attached to the final Monthly Time row stays
annual; a rate cannot borrow December's duration. Exact key/name/unit/frequency
and native owner distinguish identities, including actual RDD/MDD aliases.
Missing source in one Zone does not inherit another Zone's availability.

**Authority and knownness.** Representative Zone variables receive Zone ×
ListMultiplier once; model-total component energy/meters are not multiplied.
Native source evidence, non-additive response, derived pressure and allocated
flow have different meanings. Signed surface contributions aggregate by
Zone/category/month before directional splitting; positive inside-face
convection is heat entering the surface, so Zone-air contribution uses the
opposite sign. Observed zero requires complete valid rows; dictionary presence,
missing/NULL or display rounding cannot establish zero. Canonical thermal
selection does not add every reported alternative.

**Allocation and precision.** Known topology bounds recipients even with zero
load: unserved plenums never enter a fallback denominator. Zero or unknown
denominator leaves energy unassigned. Completed Zone-month allocation precedes
Annual/Building sums. Each independently rounded fan/source budget closes with
stable largest remainders and its own floor/ceiling quota for verifiable native
inputs. The allocation implementation retains a compatibility fallback for
legacy inputs whose quotas cannot be verified; do not infer a universal quota
guarantee from that fallback. Raw source pressure
remains traceable in months without a visible delivered-flow node. Small
positive paired ratios keep their underlying value/kind even if ordinary
three-decimal display rounding would erase them.

**Accounting and provenance.** Native `<resource>:Building/HVAC/Plant` subtotals
are overlapping context, not more Facility/end-use energy. Parallel branches
retain distinct original source/path/month identities and stable semantic
qualifiers; endpoint-only duplicate IDs must not delete evidence. Thermal
Sensible/Latent reconciliation rows remain distinct. Conversion numerators use
served thermal scope, and branch evidence is month-local even when its Annual
load endpoint includes other months. Measured fan pools remain on carrier
branches; selected-Zone load weights are allocation context, not consumption.
Zone allocation quality uses the matching Building period, never an unrelated
Annual value.

**Original wire and coverage.** Independent candidate reading checks original
numeric presence before permissive application compatibility decoding. Mandatory
node/link/reconciliation/quality fields must be present, non-NULL finite numbers;
count pairs obey found ≤ total. Pruned whole rows are optional only with complete
independent zero/precision proof. Every primary and mandatory context/non-flow/
referenced-accounting record needs exact selector coverage; numeric success
without full coverage cannot approve a real model. Cosmetic IDs, extra nodes,
sum-preserving swapped shares and driver provenance moved onto load endpoints
must still fail.

### Important model-specific edge cases

- Large Office gas remains Building-unassigned in July/August when the sixteen
  served Zones have zero heating, even though the three plenums have positive
  heating. Original Hourly AirLoop pools are ingested without double-counting
  Fans or deleting original outputs. Runtime v2 graphs build Annual plus twelve
  months; the frozen v1 adapter retains its detailed-period contract.
- Small Office's domestic-hot-water pump has real positive annual consumption
  3.5376923076923375e-8 kWh and remains unassigned. `NoReheat` alone is not a
  thermal source. Supported cooling/heating inference requires actual continuous
  native coil/wrapper supply branches; unresolved parallel/arbitrary wrappers
  do not gain guessed service.
- Ideal Loads district cooling 19,149.37 kWh and heating 7,129.51 kWh come from
  exact annual utility Tabular cells. They are not physical district-network
  proof, monthly zeros or twelve invented monthly shares. Annual-only energy
  never creates Monthly sources/conversions. Exact utility report/table/row/
  column/unit/cell identity and source precision are required; source-energy
  and peak-demand tables cannot substitute. Twenty equipment-keyed latent
  Energy/Rate observations and proven Hourly reconciliation companions remain
  non-additive context.
- Simultaneous operation requires six identity-bound cooling/heating flow and
  temperature dictionaries joined on the same non-warmup TimeIndex. The retained
  native operation check finds 7,624 of 52,560 ten-minute intervals with both
  positive flows and the appropriate temperature differences. Monthly totals
  alone cannot prove simultaneous operation or native Energy Path accounting.

### Execution modes

Ordinary tests never launch EnergyPlus and cannot approve expectations.
Use process-local environment variables only. Select exact opt-in tests in
`./cmd/semantic-idf/internal/simulation`; explicit unknown/empty fixture selectors
fail instead of silently skipping the requested model.

| Test | Explicit inputs | Meaning |
| --- | --- | --- |
| `TestEnergyPathRealModelEvidence` | `EPATH_REAL_CAPTURE=1`; optional `EPATH_REAL_FIXTURES` / `EPATH_REAL_VERSIONS` | New engine/capture evidence, not acceptance |
| `TestEnergyPathRealModelAcceptance` | `EPATH_REAL_RUN=1`; approved recipe/header already present | New run matches independent SQL and approved expectations |
| `TestEnergyPathRealModelSavedEvidence` | `EPATH_REAL_VERIFY_DIR` | Read-only same-run evidence check; acceptance is opt-in |
| `TestEnergyPathRealOracleMaterializeCandidate` | `EPATH_REAL_ORACLE_CAPTURE_DIR`, new `EPATH_REAL_ORACLE_SNAPSHOT_NEW` | One current shared-loader rebuild plus provenance sidecar, not approval |
| `TestEnergyPathRealSQLModelSavedCandidate` | `EPATH_REAL_ORACLE_CAPTURE_DIR`, `EPATH_REAL_ORACLE_SNAPSHOT` | Independent eight-group/full-coverage diagnostic |
| `TestEnergyPathRealOracleCandidateWireSaved` | `EPATH_REAL_ORACLE_WIRE_PATH` | Original numeric-presence gate only |
| `TestEnergyPathRealLargeOfficeSavedReplay` | `EPATH_REAL_REPLAY_DIR` | Large Office loader/discovery regression, not full acceptance |
| `TestEnergyPathRealSimultaneousSavedSQL` | `EPATH_REAL_SIMULTANEOUS=1` | Retained same-time native operation proof |
| `TestEnergyPathNoHeatingHybridSolverTrial` | `EPATH_REAL_NUMERICAL_TRIAL=1`, failed `EPATH_REAL_BASELINE_DIR` | Separate failed-model experiment, never fixture approval |

Saved evidence can rebuild with `EPATH_REAL_VERIFY_REBUILD=1`.
`EPATH_REAL_VERIFY_ACCEPTANCE=1` requires the approved header/companion and all
eight groups. Alternatively `EPATH_REAL_VERIFY_SNAPSHOT` selects an existing
SHA-bound candidate with explicit acceptance and is mutually exclusive with
rebuild. Engine-run and saved-run modes cannot combine. These modes never rewrite
the original capture, approved expectations or SQL.

For one existing capture in PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
. .\scripts\toolchain.ps1
$paths = Use-RepoToolchain -RequireGo
$env:EPATH_REAL_VERIFY_DIR = 'C:\absolute\path\to\one\closed\capture'
$env:EPATH_REAL_VERIFY_REBUILD = '1'
$env:EPATH_REAL_VERIFY_ACCEPTANCE = '1'
try {
    & $paths.GoExe test -count=1 -timeout=20m -run '^TestEnergyPathRealModelSavedEvidence$' ./cmd/semantic-idf/internal/simulation
    if ($LASTEXITCODE -ne 0) { throw 'Saved acceptance failed.' }
} finally {
    Remove-Item Env:EPATH_REAL_VERIFY_DIR, Env:EPATH_REAL_VERIFY_REBUILD, Env:EPATH_REAL_VERIFY_ACCEPTANCE
}
```

Engine overrides use `EPATH_REAL_ENGINE_<major>_<minor>`, such as
`EPATH_REAL_ENGINE_22_1`; `EPATH_REAL_WEATHER` supplies an explicit verified EPW.
Local engine installation and ignored captures are required for these modes and
are not supplied by ordinary checkout.

### Diagnostic, review and packaging workflow

1. Preserve a normal completed native capture and its source/run/engine/weather
   hashes. A capture alone never approves numerical behavior.
2. Independently review original ownership, exact SQL observations, selectors and
   model-specific recipe boundaries. Production classification/allocation/
   quality functions and candidate scalar values cannot produce expected numbers.
3. Materialize a new candidate when production or native provenance changes.
   Sidecars bind original inputs, closed SQL, engine/weather and current non-test
   Go contents, including uncommitted source. Existing candidates remain evidence.
4. Run all eight numeric/contract groups plus exact required-record coverage.
   Failed reports remain failed; do not remove selectors or enlarge tolerances.
5. Optional `EPATH_REAL_ORACLE_PENDING_NEW` writes a new pending-review artifact
   only after complete success. Destinations must physically remain inside
   `.runtime`, be new, and have unchanged provenance. Pending is not approval.
6. Review independent original-to-pending arithmetic and complete provenance/
   registry separately, then author the approval header explicitly.
7. `TestEnergyPathRealExpectedGeneratePayload` packages the reviewed full array:
   `EPATH_REAL_EXPECTED_PAYLOAD_MODE=prepare|install`,
   `EPATH_REAL_EXPECTED_PENDING`, `EPATH_REAL_EXPECTED_REVIEW_SHA256`.
   Prepare also needs new `EPATH_REAL_EXPECTED_PAYLOAD_NEW`; install requires
   the matching already-authored catalog header. Neither mode overwrites files
   or authors approval.
8. Run saved acceptance with the unchanged approved header/companion; rerun
   affected prior fixtures when shared engineering behavior changes.

The reader preserves inline-v1 compatibility while rejecting ambiguous
inline/companion payloads, duplicate JSON fields, missing explicit values,
corrupt/truncated/extra-member gzip, trailing data, path escape and changed
hash/count/key registries. `TestEnergyPathRealApprovedExpectedCatalog` requires
all 19 approved artifacts in ordinary offline tests.

See [testing](testing.md) for daily scoped tests and release verification.

## Equipment boundaries

Each section identifies its reviewed fixture, physical limits and regressions.
Metric counts are in [approved coverage](#approved-coverage-and-equipment-references);
linked recipes/headers bind immutable source declarations, expectations and
companion checksums. These are fixture transformation baselines, not general
HVAC design certification or support for every object configuration. Use the
shared [execution modes](#execution-modes) for deliberate native replay and
[testing](testing.md) for ordinary development.

### District Energy

Fixture `district-energy-25-1`.

#### Model and accounting boundaries

- Official EnergyPlus 25.1 model: `5ZoneFanCoilDOAS_ERVOnAirLoopMainBranch.idf`.
  PLENUM-1 and SPACE1-1 through SPACE5-1 have factor one. Only the five SPACE
  Zones own local FourPipe FanCoils and shared DOAS terminals.
- Hydronic cooling and heating reach separate district-water plants. A DOAS
  coil can legitimately appear on air and water branches. Multiple delivery
  paths must not multiply one Zone's delivered load.
- Monthly Zone Air System Sensible Cooling/Heating Energy is load authority.
  PLENUM's 24 sensible observations are real zeros. Missing latent loads and
  absent plenum gains are unavailable observations, not measured zeros.
- District conversion is `load_to_purchased_energy`. Broad end-use and
  facility district meters close monthly. Electricity Building/HVAC/Plant
  subtotals are overlapping context.
- Forty-six passive surfaces comprise eight exterior walls, one roof, five
  ground floors, six openings and 26 interzone faces. Four infiltration
  gain/loss families remain separate. Machine-scale outdoor residuals and
  DOAS recovery context do not establish additional measured Zone delivery.
- Fans combine one DOAS fan and five local fans. DOAS airflow cannot split this
  unmeasured six-fan budget; Fans remain unassigned. Pumps use an explicit
  month-first cooling-plus-heating `plant_loop_load_share` over the five
  proven recipients. This is an estimate, not per-pump or Zone metering.
- Exact reporting frequency is part of Output navigation identity. Native
  terminal/ADU or DX-wrapper port contradictions cannot authorize service edges.
  Unknown composite ownership cannot be replaced by guessed connectivity.

#### District evidence and limits

Capture:
`.runtime/energy-path-acceptance/25.1/district-energy-25-1/real-district-energy-25-1-20260907T163605.594498300`.

Native SQL SHA-256:
`3949ede8734d220bdbb1a02a1d95902ceebcb4a2f03dc71d7570578c72e276a2`.

The capture retains eight warnings and no Severe errors. All 347 Monthly
identities have twelve finite observations; 72 Hourly variables and five
Run Period meters retain their actual frequency. Aggregate NULL WarmupFlag
is valid in this SQL. Original hourly requests survive additive purpose plans.

Building floor area is 927.2 m², including the 463.6 m² PLENUM explicitly included
in EnergyPlus total floor area. Zero load does not remove floor area. Actual
Hourly charts retain 8,760 timestamps and existing 0.001-kWh quantization;
rounded display zeros are not independent native zero observations. Monthly
accounting remains source-aware and is not reconstructed from rounded charts.

#### District regressions

- IDF topology: `internal/idf/hvac_district_service_test.go` and
  `hvac_hydronic_delivery_service_test.go`.
- Requests and auxiliary allocation: `energy_path_district_purpose_test.go`,
  `energy_auxiliary_zone_allocation_district_test.go`.
- Immutable declarations: [recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/district-energy-25-1.json)
  and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/district-energy-25-1.json).

### Fan Coil

Fixture `fan-coil-25-1`.

#### Ownership and consumption

Official EnergyPlus 25.1 model `FanCoilAutoSize.idf` has three Zones. Each owns
a FourPipeFanCoil, ConstantVolume fan, water cooling/heating coils and local
outdoor-air mixer; there is no AirLoop. CHW serves the cooling coil and HW serves
the heating coil, with purchased district cooling/heating sources.

Native binding requires typed coil ownership, exact water nodes and unique
demand Branch/PlantLoop membership. Do not form a Cartesian product of combined
terminal services and plants: CHW-to-heating, HW-to-cooling and plant-backed
ventilation are invalid. Foreign metadata cannot win same-ID deduplication.
Mixed plants retain legitimate mixed service; detached or ambiguous paths fail
closed. A local electric heater cannot borrow the cooling plant.

Broad Fans remain unassigned: six original inlet/outlet Hourly mass-flow traces
do not isolate fan energy. The two single-service plant pumps serve all three
Zones; broad Pumps use an explicit cooling-plus-heating load-share estimate.
Neither allocation is a measured per-component or Zone budget.

All 21 heat-transfer surfaces retain signed Zone/category/month aggregation
before directional splitting. Three self-referencing floors are Other/storage,
not ground or interzone exchange. WEST lighting is absent; EAST/NORTH lighting
is reported. District conversion is Load / purchased energy, not COP/efficiency.
Electricity Building/HVAC/Plant remain non-additive context.

#### Precision and zero-pressure fallback

NORTH August has real heating load 0.03294921830534119 kWh, corroborated by 13
positive Hourly rate samples, while district heating is observed zero. All
matching monthly heating pressures are zero. Independently rounding internal
gain terms first manufactured a false -0.001-kWh pressure.

Canonical Monthly calculation uses original selected-source precision before
presentation rounding. A pure zero-pressure Other/storage fallback requires
complete original evidence, exact zero raw/effective/signed physical pressure
and only the actual heating-load source. It retains Period M8, Zone/Building
identity and displayed paired load 0.033 kWh. Missing/NULL observations,
unavailable alternative dictionaries, nonfinite rows or small real signals
cannot be converted to zero proof. Mixed Annual/Building storage is not
asserted to have pure zero pressure.

Annual paired ratios follow completed monthly conversion branches when some
months have load but zero purchased energy. They are not an unrelated quotient
of complete annual load and site totals. Monthly pump shares are independently
derived, then summed for Annual; broad-meter and own-Zone weight traces remain
separate.

#### Fan Coil evidence and regression entry points

Capture:
`.runtime/energy-path-acceptance/25.1/fan-coil-25-1/real-fan-coil-25-1-20260907T163224.387644700`.

SQL SHA-256:
`c0004ad8c0310942375b1a2ad601ba6eee5b44ad5774fee2b3e5dbc4e080bb91`.

All 167 Monthly identities have twelve finite observations; 36 Hourly identities
have 8,760 observations. Five Run Period meters remain annual even when attached
to the final Monthly Time row. The engine retains seven warnings and no
Severe/Fatal errors; the error parser separately retains the unavailable-output
header as an eighth issue.

- Topology: `internal/idf/hvac_hydronic_delivery_service_test.go`.
- Auxiliary allocation: `energy_auxiliary_zone_allocation_fancoil_test.go`.
- Native calculation precision: `energy_driver_monthly_precision_test.go` and
  `energy_driver_monthly_precision_multiplier_test.go`.
- [Reviewed recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/fan-coil-25-1.json)
  and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/fan-coil-25-1.json).

### Mixed heating fuels

Fixture `mixed-heating-fuels-25-1`.

#### Native boundaries

Official EnergyPlus 25.1 `5ZoneElectricBaseboard.idf` has five served SPACE
Zones and an unserved return plenum. SPACE2-1 and SPACE4-1 own local
radiant-convective electric baseboards and also receive central boiler heating.
Local equipment does not exclude a Zone from central service.

- Native baseboard electricity is already model-total. Zone Air System sensible
  energy is the aggregate thermal boundary; baseboard Total Heating is
  non-additive response. Do not count thermal load once per carrier.
- Ten configured radiant-recipient surfaces retain observed mixed
  environmental/HVAC convection. Configuration fractions do not measure actual
  operation or isolate passive/baseboard-only heat gain.
- Local electricity and shared-boiler electricity are separate source-local
  budgets. Missing/NULL/duplicate consumption cannot acquire gas-carrier
  fallback shares. Observed zero boiler ancillary electricity stays known zero.
- Direct and allocated carrier branches remain distinct even when both use
  electricity. A mixed subtotal cannot claim complete directly measured HVAC
  consumption. Direct baseboard branches cite only their exact delivery paths.
- Building conversion uses actually served load once and excludes the unserved
  plenum, while the plenum's real load remains in canonical Building totals.
  Missing or ambiguous topology cannot be replaced with guessed path provenance.
- The single supply fan uses a five-Zone cooling-plus-heating load-share
  estimate. Mixed hot/chilled/condenser Pumps remain unassigned. HeatRejection
  is a broad-meter cooling estimate; its one MTD tower constituent does not
  create unobserved native tower electricity.
- Monthly energy is consuming authority. Hourly/Rate companions retain exact
  source ownership, calendar and request identity as non-additive context.
  Original and executed Output indices are bound independently from components.

Original-SQL baseboard electricity is 251.68286880208026 and
455.3018727624689 kWh (706.984741564549 total). Their Total Heating responses are
221.1551861493879 and 398.20613250492454 kWh and are not additional delivered
loads. Honest display overlap of 0.001-kWh branches is retained rather than
hidden or used to relax native closure.

#### Mixed heating evidence

Capture:
`.runtime/energy-path-acceptance/25.1/mixed-heating-fuels-25-1/real-mixed-heating-fuels-25-1-20260914T125004.684985500`.

SQL SHA-256:
`d104dfb1fd7ec9f7f7e9001d59cc02b282217e9bc9e4d51bb31812c4a93cfc32`.

The new requests preserve 222,871 pre-existing observations and the 8,772-row
weather axis. The engine retains 23 warnings with no Severe/Fatal errors.
Original physical objects and weather are unchanged.

#### Mixed heating regressions

- Native devices/paths/shared budget: `energy_path_baseboard_components_test.go`,
  `energy_path_baseboard_paths_test.go`, `energy_path_baseboard_shared_test.go`.
- Recipient and original-index proof: `energy_path_real_sql_baseboard_*_test.go`.
- Tests retain V1-to-v2 conversion, two JSON reloads, DirectOnly, VRF coexistence,
  one multiplier application, exact monthly-to-annual sums and
  missing/NULL/duplicate/known-zero cases.
- [Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/mixed-heating-fuels-25-1.json)
  and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/mixed-heating-fuels-25-1.json).

### Pool and Zone multiplier

Fixture `zone-multiplier-25-1`.

#### Physical and accounting boundaries

Official EnergyPlus 25.1 `5ZoneSwimmingPoolZoneMultipliers.idf` has six Zones
with Zone multiplier 3 and ListMultiplier 1. Five are supply recipients; the
return plenum keeps its measured load without becoming a supplied Zone.
Original, executed and SQL object indices are different namespaces, resolved
independently rather than by an offset.

- The hot-water plant serves five reheat coils, outdoor/main heating coils and
  the indoor pool. Pool-water heat and boiler output are non-additive thermal
  context. Process demand prevents allocating the entire Heating meter to
  space heating. Building/Zone conversion and cached legacy summaries retain an
  explicit unquantified-service boundary; consumption remains
  Building-accounted and Zone-unassigned.
- Native chilled-water pump electricity has an independently proved
  cooling-only plant and five-Zone roster. Only that budget may use primary
  sensible-cooling weights. Hot-water pump electricity remains unassigned.
  Native components are already model-total; representative Zone load receives
  factor 3 once.
- Joint display budgets require exact milli-kWh equality across Zone allocations,
  Building ledger and canonical CW-source allocated details. Individual
  proportional rounding intervals are insufficient. A 0.001-kWh budget cannot
  become three 0.001-kWh shares. Annual allocation sums completed months.
- Native subquantum monthly energy may round to zero allocation while its
  independently integrated annual source remains nonzero. Annual source
  scalars use native integration followed by one decimal-three transport step,
  independently of 8,760 separately rounded Hourly chart samples.
- All 28 Pool/boiler/pump Energy/Rate Monthly/Hourly identities require exact
  raw/effective/source/calendar proof. Missing/NULL source, chart or mandatory
  graph `value` fields cannot become reported zero through typed decoding.
- F1-1 Ground floor stays in all twelve signed surface cells. Its measured
  convection is coupled to Pool operation; it is not an isolated passive
  envelope quantity. Exact executed Pool/floor ownership and explanation remain.
- Ordinary measured surface sources may carry only the existing literal
  allocation explanation when `AllocationApplied` is true and
  `Formula == AllocationFormula`. Fabricated subtraction, extended/mismatched
  formulas and arbitrary `InputSourceIDs` remain invalid.

#### Pool evidence

Capture:
`.runtime/energy-path-acceptance/25.1/zone-multiplier-25-1/real-zone-multiplier-25-1-20260914T185259.656652100`.

SQL SHA-256:
`95d3e0a145d0f39fd0f2518c47a4163f583fa900cee62c1bd904a290a94d20d1`.

The recapture preserves all 309 physical objects and 932,530 common observations.
Seven warnings remain: four unsupported legacy gas aliases and three original
MeterFileOnly duplicate requests. There are no Severe/Fatal errors.
Optional unavailable outputs are missing, not measured zeros.

#### Pool regressions

`energy_path_real_sql_pool_*_test.go` separates native ownership, services,
joint pump budgets, source/Chart boundaries, surface qualifiers and calendar
equivalence. Consumer/mutation tests retain graph presence, exact ledgers,
Monthly-to-Annual closure and failure on forged source transformations.

[Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/zone-multiplier-25-1.json)
and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/zone-multiplier-25-1.json).

### PTHP

Fixture `pthp-25-1`.

#### Ownership and additive energy

Official EnergyPlus 25.1 `DOAToPTHP.idf` has five SPACE Zones, each owning a
PTHP, DX cooling/heating coils, supplemental NaturalGas coil and local fan.
DOAS mixer continuity, EquipmentConnections, EquipmentList and typed references
are checked against the entire original input before scope filtering.
PLENUM-1 owns no HVAC equipment and has genuinely zero sensible loads.
All six Zone factors are one; other regressions cover already-model-total
component energy without a second multiplier.

| Native consuming family | Five-owner annual kWh |
| --- | ---: |
| DX cooling electricity | 7565.922327728445 |
| DX heating electricity | 129.0817607060335 |
| Heating defrost electricity | 44.544796591612524 |
| Heating crankcase electricity | 3963.256209764137 |
| Supplemental NaturalGas | 1718.862453752996 |
| Supplemental ancillary NaturalGas | 0 |
| Supplemental fuel-coil electricity | 0 |

The 35 exact Monthly/J/non-meter identities are separately owned cohorts.
Large crankcase energy is measured heating consumption and must remain in its
denominator. PTAC's cooling crankcase is a different physical contract.
DX and Fuel coils both report `Heating Coil Electricity Energy`; name
recognition alone cannot establish owner/type. Cross-type duplicates, foreign
coils, shared references and stale requests fail exact ownership proof.
Missing/NULL/duplicate/negative members invalidate a complete carrier cohort;
known-zero owners cannot receive its remainder.

Cooling uses COP; mixed electricity/NaturalGas heating uses Load / site energy.
Combined denominator and carrier-qualified ledgers are checked separately.
Broad Fans combine the DOAS and five local fans. The 32 original hourly
node-flow series do not establish an isolated fan-energy pool; all
1010.3597051453659 kWh remains explicitly unassigned.

#### Wire and reconciliation contracts

Original numeric presence is checked before compatibility decoding. A native
Go zero cannot prove an omitted/NULL v2 wire value. July's genuinely zero
NaturalGas facility/heating meters permit a pruned reconciliation row only when
independent expected, explained and residual quantities all admit zero.
Positive balanced energy, partial rows, contradictory context and invented
Monthly zeros cannot use this whole-row absence proof. Referenced accounting
records retain their exact Building origin rather than becoming Zone-local rows.

#### PTHP evidence and limits

Accepted capture:
`.runtime/energy-path-acceptance/25.1/pthp-25-1/real-pthp-25-1-20260908T095959.079056400`.

SQL SHA-256:
`5470d4b338f0ad78c6b050ae4c196a78d856fa03a1c511b3db8aa98bf87b3b3e`.

The older September 7 capture lacked all 35 consuming identities; it is
incomplete evidence, not a zero-valued direct-energy fixture. Added requests
preserve the prior 340 Monthly identities and 4,080 observations; the new
420 observations have complete 2017 weather months with no NULL/duplicates.
The accepted run retains 24,523 warnings and no Severe/Fatal errors.
The example identifies Miami; controlled Chicago TMY3 validates transformation,
not climate suitability or every heat-pump configuration.

Regression files:
`energy_path_direct_hvac_components_pthp_test.go`,
`energy_path_direct_hvac_sql_pthp_test.go`, and the strict native Site/
Reconciliation/AnnualSite proof families.

[Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/pthp-25-1.json)
and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/pthp-25-1.json).

### PV and storage

Fixture `pv-storage-25-1`.

#### Electrical and source boundaries

Official EnergyPlus 25.1 model `ShopWithPVandBattery.idf` preserves 466
physical objects and controlled Chicago weather. Native electrical quantities
have different roles and must not be combined into invented consumption:

| Native quantity | Annual kWh |
| --- | ---: |
| PV DC production | 54522.196715 |
| Inverter AC output | 49224.732528 |
| Inverter loss | 4037.580186 |
| Storage charge | 10576.973327 |
| Storage discharge | 9317.089326 |
| Storage thermal output | 3479.368355 |

Storage thermal output is not charge minus discharge; unobserved state-of-charge
closure is unavailable. Inverter ancillary electricity and its 446.8-kWh
consumed Cogeneration parent describe one budget. Consumed
`Cogeneration:<resource>` participates in end-use availability and electricity
ancillary consumption remains canonical Other, not onsite production.
Positive gas equipment consumption is 2564.370621 kWh; zero latent gain cannot
imply zero consumption.

Twenty electrical Monthly/Hourly identities plus consumed-Cogeneration evidence
retain signs, raw/effective values, full calendars and chart axes. Nine native
equations are checked at monthly/hourly/annual scopes; Facility closure uses
only the native EPSM deadband. The exact SQL-sibling MTD checksum remains
mandatory through capture, pending and approval. Removing graph nodes cannot
bypass source-registry obligations.

#### HVAC, fans and allocation precision

- Five-Zone service topology and terminal delivery retain original/executed
  ownership. SHW pump consumption remains separately unassigned. Facility
  provenance can contextualize a Zone subtotal, never duplicate the entire
  Facility amount into Zone consumption.
- Basic Energy requests exact Hourly AirLoop fan pools only after complete
  original loop/branch/component/fan inventory and recipient proof.
  Mixed local/nested/exhaust/unowned fans cannot trigger partial heavy requests.
  Existing exact unfiltered keys or blank-key wildcards are reused literally.
- This fixture adds five fan series (43,800 observations); all 733 earlier
  dictionaries and 5,864,508 common observations remain exact, including native
  zeros. Monthly pools close Fans without rescaling.
- Driver allocation uses stable largest fractional remainders at three-decimal
  precision. Each positive branch remains within its own floor/ceiling quota
  and the monthly load closes exactly. The last source cannot accumulate other
  sources' rounding errors. Raw signed pressure, eligibility, source identity
  and pure zero-pressure fallback are unchanged.
- The 114 additional Hourly companions are non-additive context of Monthly
  authorities, with original ownership and complete-month correspondence.
  Source display/allocated consistency is not another independent native proof.

Requested-stage availability in the approved fixture is 12/18 drivers,
9/14 loads, 10/12 end uses and 2/2 carriers. Driver closure is 100%;
end-use/carrier closure is partial at 94.122%. Complete oracle coverage
does not mean absent native outputs have become available.

#### PV/storage evidence and regression entry points

Capture:
`.runtime/energy-path-acceptance/25.1/pv-storage-25-1/real-pv-storage-25-1-20260925T031519.297356600`.

| Artifact | SHA-256 |
| --- | --- |
| SQL | `5f4dde5c868f419b2c169defd38ce7d3ec597fef9e1e970f28fbf8aa3548f2d8` |
| MTD | `87bf8f501d8bb3eacf32d80c29401004c9ff42a2af9d51fb39ae793e11686438` |

The capture retains 59 warnings and no Severe errors. It validates accounting,
not climate suitability or general design quality.

Regressions: `energy_path_real_sql_pv_*_test.go`,
`energy_path_real_oracle_pv_mtd_provenance*_test.go`,
`energy_driver_allocation_quantum_test.go`, and native fan-request tests.
Independent native-only representative arithmetic supplements the complete
eight-group/required-field oracle; it does not replace it.

[Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/pv-storage-25-1.json)
and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/pv-storage-25-1.json).

### Radiant

Fixture `radiant-25-1`.

#### Native delivery and ownership

Official EnergyPlus 25.1 `RadLoTempCFloHeatCool.idf` has WEST/EAST/NORTH
(factor one), each with a `ZoneHVAC:LowTemperatureRadiant:ConstantFlow` floor
system. Exact EquipmentList/Zone/design/surface ownership, separate cooling and
heating ports, demand Branches and PlantLoops must agree. There are no terminal
Coil objects, AirLoops or Fan objects. Native binding cannot invent plant-free
ventilation or cross-attach CHW heating before ID deduplication.

Cooling is a genuine mixed electric-chiller/district pool with separate
on/off-peak operation lists; heating is district hot water. Header comments do
not override typed objects or native observations. There are 21 heat-transfer
surfaces, including three active radiant floors.

EnergyPlus Zone Air System sensible energy excludes radiant systems and does
not include Zone/ZoneList factors. ConstantFlow radiant heating/cooling instead
reports the active surface source/sink terms, already multiplied by those
factors. It is not same-month air delivery or measured equipment COP. Monthly
storage and surface exchange retain explicit `thermalBoundary` metadata.
Active-surface convection is non-additive context, not an independent passive
driver. Mixed systems selecting air-system load warn that combined delivery
has not been established.

Equipment outputs use exact equipment keys, not Zone names. Additional Monthly
J/W requests retain original wildcard Timestep requests. This capture's 72
air-system sensible observations are real zeros; material native equipment
fallback can become canonical load without summing both authorities. Unowned
native radiant observations retain thermal context with unresolved Zone and
cannot create a load.

#### Shared services and auxiliary budgets

- Three cooling/heating paths have the exact native recipients WEST/EAST/NORTH.
  Service-specific Monthly shares are `service_path_allocation`, not measured
  per-Zone consumption. Electric and district cooling pools retain separate
  source-local, month-first proofs.
- Big Tower's condenser path is cooling-only. HeatRejection uses
  `condenser_loop_load_share` with cooling weights. It cannot borrow heating
  weights or claim district cooling itself consumes tower energy.
- Broad Pumps mixes CHW/HW/condenser pumps and three embedded radiant pumps.
  Their union of recipients does not prove one complete denominator; the pool
  stays unassigned. Embedded pump traces do not create missing central-pump
  observations. Pump fluid heat (652.2377465115 kWh) and pump electricity
  (738.6610945765 kWh embedded) are different quantities.
- Native infiltration gain/loss families stay separate for directional pressure.
  Only reconciliation uses signed net sensible energy. Outdoor-minus-infiltration
  fallback uses original Monthly precision; pre-rounded subtraction must not
  manufacture mechanical ventilation. Missing precision remains unknown.
- Stable surface type/name identity survives different original/annualized
  indices. Original input, executed Output and SQL dictionaries are separately
  bound. Case-insensitive EnergyPlus owner names do not permit foreign owners.
- Native J aggregation is `sum_report_data`. Source raw/effective values,
  one multiplier application, observed-zero/null distinctions and missing
  dictionary identity remain independently checked.

#### Radiant evidence and limits

Accepted capture:
`.runtime/energy-path-acceptance/25.1/radiant-25-1/real-radiant-25-1-20260908T152059.721318500`.

| Artifact | SHA-256 |
| --- | --- |
| SQL | `9a36262ab875cd690523fca0835f6b8c582b844167fc18f1692658aac038441e` |
| Original MTD | `6046eefa4e329b521265247839c4e3f483e80088f54e4833f8d74d1e7e04fb63` |

The new requests preserve all 258 old dictionaries and 4,574,728 old
observations; thirteen new Monthly identities provide radiant J/W and
HeatRejection. The old capture's tower remainder was derived; the new
3437.5528507403956-kWh HeatRejection meter is actually observed.
Both captures retain 56,132 warnings and no Severe errors. WEST lighting is
absent, not measured zero. Annualization is recorded separately from original
design-day-only controls.

#### Radiant regressions

Topology: `internal/idf/hvac_radiant_delivery_service_test.go`.
Production: `energy_path_radiant_*_test.go`, `energy_driver_monthly_precision*_test.go`.
Independent proof: `energy_path_real_sql_radiant*_test.go`, covering active
surface context, native loads, service/carrier ledgers and scalar presence.

[Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/radiant-25-1.json)
and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/radiant-25-1.json).

### VRF

Fixture `vrf-25-1`. Reviewed support is electric, air-cooled,
non-heat-recovery VRF with draw-through ConstantVolume terminals and no
supplemental heater.

#### Ownership and consuming cohorts

Official EnergyPlus 25.1 `DOAToVRF.idf` has one `VRF Heat Pump` and terminals
TU3/TU4/TU1/TU2/TU5 serving SPACE1-1 through SPACE5-1. PLENUM-1 is not a recipient.
Both InletSide and SupplySide DOAS mixer connections matter. Typed duplicate or
contradictory ownership fails before selected-Zone filtering.

The MTD additive boundaries are:

- Cooling electricity: five terminal observations + outdoor main + outdoor
  crankcase. Crankcase belongs to cooling here, unlike PTHP's heating cohort.
- Heating electricity: five terminal observations + outdoor main + outdoor
  defrost. Thermal coil transfer and fan consumption are separate.

Local and shared outdoor consumption can serve the same Zone. A known-zero
terminal remains eligible for the outdoor pool. Terminal metering does not
establish fully measured Zone service.

#### Observation, allocation and reservation contracts

- Each of 14 constituent identities requires exact native type/key, non-meter
  J unit, Monthly frequency, Basic Energy purpose and individually validated
  request. A missing request cannot erase other valid identities.
- Known zero requires actual finite rows. Missing/NULL/negative/nonfinite,
  duplicate identities or duplicate months are unknown/invalid. Invalid months
  do not erase unrelated valid months; all-NULL dictionaries retain identity.
  Timestep J/W cannot heal missing Monthly consuming measurements.
- Actual Weather months define the axis; short runs are not padded to twelve.
  Allocation retains unrounded Monthly J until final presentation.
- Original selected load evidence and its Zone/ZoneGroup factor are applied once
  to weights. Already-model-total native consumption is not multiplied.
- Every shared source uses all original owners' known service loads each month.
  Missing owner load cannot shrink the denominator. Missing either main or
  auxiliary keeps the service's shared pool unassigned. Annual sums completed
  months without annual reweighting.
- Local and allocated knownness remain separate. Partial/DirectOnly terminal
  energy cannot create a complete terminal-only COP for the whole system.
  Cooling uses COP; heating conservatively uses Load / site energy.
- Reserve the full native local/outdoor subtotal before unrelated non-VRF
  recipients receive broad-meter remainder, including unallocated outdoor
  energy. Unknown constituents prevent a guessed remainder split.
  Electric VRF paths cannot establish unrelated carrier ownership.
- Output request indices and physical equipment indices remain separate.
  Nodes/links retain exact HVAC targets, source-local budgets and service paths.
  Existing direct lighting/equipment carrier evidence survives a VRF merge.
- Display ledgers sum separately rounded source budgets. Source-specific integer
  apportionment uses native weights with exact per-source closure. Ledger Direct/
  Allocated/Unassigned, metadata, source role and Annual identities are proved
  separately; sum-preserving swaps still fail.

Native closure and displayed accounting are distinct. The approved display has
a -0.003-kWh signed heating residual and 0.005-kWh positive monthly overlap.
Recorded overmapped quality is retained without claiming excess native
physical consumption.

#### VRF evidence

Capture:
`.runtime/energy-path-acceptance/25.1/vrf-25-1/real-vrf-25-1-20260908T130012.807798800`.

SQL SHA-256:
`2e5d6f46d1d0fcf89cd433352faf51fb17a24f47ba0ebe4f0a35679c4408cc0e`.

Fourteen new Monthly requests preserve 3,984 previous Monthly and 455,520
Timestep values, original manual requests and TU1 fan Rate. The new 168
observations include ten real zeros and close each broad service meter.
The engine retains 11,724 warnings with no Severe errors. The older capture
lacked four terminals' consuming evidence; broad residuals cannot manufacture
those missing identities. Dictionary IDs are capture-local, resolved by full
identity in every run.

#### VRF regressions

Production: `energy_path_vrf_*_test.go` (components, SQL, load shadows,
allocation, projection and reservation).
Independent original proof: `energy_path_real_sql_vrf_*_test.go`, including
ledger mutations and original path/hash context at every oracle entry point.

[Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/vrf-25-1.json)
and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/vrf-25-1.json).

### ZoneGroup

Fixture `zone-group-25-1`.

#### Model and multiplier contracts

Official EnergyPlus 25.1 `MultiStory.idf` has nine representative Zones.
Each owns a WindowAirConditioner, Fan:OnOff, DX cooling coil and convective
electric baseboard. Exact EquipmentList/EquipmentConnections, typed component
ports and outdoor nodes establish ownership without a central AirLoop.

Each representative Zone is 24 m². Original Zone and floor-group factors match
both SQL multiplier fields independently; effective floor area is 1,440 m².
The ordinary AllZones gain list is not another multiplier.

| Observation | Effective accounting |
| --- | --- |
| Coil/crankcase/fan electricity | Already model-total; no extra factor; exclude overlapping WindowAC package total |
| Baseboard electricity | Already model-total purchased heating input |
| Baseboard Total Heating | Already model-total non-additive response |
| Zone Air System sensible energy | Zone factor × Group factor exactly once |
| Zone Hot Water Equipment district-water energy | Zone factor × Group factor once; interior equipment, not HVAC heating |
| Facility/broad end-use meters | Already model-total reconciliation boundaries |

Convective baseboards have no radiant recipients. Native default capacity/
efficiency rules remain; invalid selected capacities remain invalid. Editable
field comments do not override native positional ownership or multiplier fields.
Direct local and allocated central evidence stay distinct across subtotals,
Monthly/Annual aggregation and stored JSON.

Monthly consuming sources and Hourly/Rate context retain exact source/path/
request identity. Missing/NULL/duplicate/wrong-unit observations cannot stand in
for observed zero. Optional exact-zero flow leaves do not excuse missing,
nonzero or cancelling sources. Resistance heating uses Load / site energy;
tiny paired ratios retain transport bounds rather than being forced to nominal
efficiency.

#### ZoneGroup evidence and limitations

Accepted capture:
`.runtime/energy-path-acceptance/25.1/zone-group-25-1/real-zone-group-25-1-20260914T161342.768401800`.

SQL SHA-256:
`c72db1c09745099d7e7ae304ef98d72ed67db9d6678f9b9a388e6c64fc1ed5cf`.

Original model SHA-256:
`c7165328a5f3a90aa81ac3f95928600079cf9f7a109fd615ccc6686caf518ba2`.

Native cooling/fan totals are 46992.05968821516 and 1387.209444590907 kWh.
All nine crankcase outputs are real zeros. Baseboard Total Heating equals
representative air heating times both factors at all Monthly/Hourly timestamps;
it is not added a second time.

The first new capture omitted district-water Building meters and has no SQL
Tabular fallback. The accepted second capture adds only two meters at
Monthly/Hourly frequency: 17,544 observations. All 1,191 prior dictionaries,
5,053,118 common observations and 8,772 Time records remain exact.
Each new annual meter is 11336.763487621001 kWh, matching independently
multiplied HotWaterEquipment use. A separate internal-gain feature flag prevents
false district HVAC-heating requests.

The engine retains 7,168 physical-model warning occurrences plus four legacy
district-alias warnings. SQL completion flags remain native string FALSE;
runner/ERR terminal success supplies completion evidence. Nothing rewrites
those flags or suppresses design/convergence warnings. Baseboard context scalars
retain documented per-row 0.001-kWh transport; context rounding cannot replace
load or purchased-energy authority.

#### ZoneGroup regressions

`energy_multiplier_native_fields_test.go`,
`energy_path_baseboard_convective_*_test.go`,
`energy_path_real_sql_zone_group_ratio_unit_test.go`, and native
HotWaterEquipment/mixed-provenance/JSON-reload tests.

Parser regressions reject blank references before NodeList lookup, avoid
zero-field panics and retain genuine competing-port failures in either list
order. Source-only/Hourly companions and copied Building ledgers retain exact
non-additive/referenced-accounting roles.

[Recipe](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/oracles/zone-group-25-1.json)
and [approved header](../cmd/semantic-idf/internal/simulation/testdata/energy_path_real_models/expected/zone-group-25-1.json).

## Development decisions and acceptance baseline

### Current development boundaries

Energy Path uses the existing chart-focused UI with Scope/Period selection,
kWh/m2 presentation and Monthly/Hourly component inspection. Removed KPI cards,
Service selection, detailed legacy inspectors and cross-panel HVAC/Output
navigation are not requirements for restoring the current view. Wire/consumer
service and trace contracts remain independent of visible controls. Regression
fixtures also exercise compatibility drawer/request state without requiring
legacy actions in the app. The [simulation runner](simulation-runner.md) describes current views.

Engineering changes must preserve physical ownership, units and multipliers,
source identity, independent paid budgets, and absent-versus-known-zero values.
Unproved ownership remains unassigned rather than gaining an invented allocation.

Other grouping is presentation-only and follows the strict <1% grouping rule;
a material category is retained even when that exceeds the former eleven-node
cap. Grouping must not alter original source IDs or export quantities.

### Tracked evidence and its limits

The recorded 2026-09-25 baseline replay rebuilt all 19 approved captures and
checked 413,095 semantic metrics. App/HTTP/CLI/Python parity, the monthly capacity
fixture and the repository/Wails gates also passed at that checkpoint. Those are
historical baseline results; changing code later requires the relevant current
checks. A catalog entry, ordinary test pass or successful build does not prove
that all optional native replays were rerun.

The [native fixture evidence](#native-fixtures) binds exact identities and
independently authored expectations; changing code never authorizes replacing
them with candidate-derived values.

### Contracts exposed by the native review

| Boundary | Required behavior |
| --- | --- |
| Large Office and older engine versions | Complete recipient cohorts; deterministic monthly-first rounding remainders; absent, zero and typed nulls remain distinct |
| Central simultaneous heating/cooling | Native component ports and service/carrier-qualified reservations establish lineage; module count does not multiply model-total consumption |
| SimpleVentilation | Direct fan electricity and sensible/latent ventilation remain separate; measured zero load does not create absent conditioning observations or COP |
| Furnace without cooling | Complete native recipients qualify fuel/fan allocation; delivered load/fuel efficiency uses actual observations rather than nominal burner efficiency |
| Shared plant and auxiliary pools | A real service path does not alone prove a component split; retain unassigned budgets when recipient or ownership evidence is incomplete |

The [equipment references](energy-path.md#equipment-boundaries) cover the
other approved families and their regression locations. [Performance notes](performance.md)
separate engine execution, SQL reading, graph construction and transport costs.

### Continuing development

For source/accounting changes, inspect original physical ownership and native
observations, then run focused negative regressions and the relevant saved
acceptance checks. Keep failed captures and previous approved evidence intact.

## Historical document references

Fixture approval headers retain the review paths used when their independent
expectations were authored. Those historical filenames are mapped below;
the original documents remain in Git history (before this consolidation).
Do not rewrite approval headers, hashes or numeric expectations to rename docs.

| Former document | Current section |
| --- | --- |
| energy-path-schema.md | [Wire contract](#wire-contract) |
| energy-path-cli.md | [CLI and Python](#cli-and-python) |
| energy-path-acceptance.md | [Regression map](#regression-map) |
| energy-path-real-models.md | [Native fixtures](#native-fixtures) |
| energy-path-progress.md | [Development decisions and acceptance baseline](#development-decisions-and-acceptance-baseline) |
| energy-path-district-acceptance.md | [District Energy](#district-energy) |
| energy-path-fan-coil-acceptance.md | [Fan Coil](#fan-coil) |
| energy-path-mixed-heating-acceptance.md | [Mixed heating fuels](#mixed-heating-fuels) |
| energy-path-pool-acceptance.md | [Pool and Zone multiplier](#pool-and-zone-multiplier) |
| energy-path-pthp-acceptance.md | [PTHP](#pthp) |
| energy-path-pv-acceptance.md | [PV and storage](#pv-and-storage) |
| energy-path-radiant-acceptance.md | [Radiant](#radiant) |
| energy-path-vrf-acceptance.md | [VRF](#vrf) |
| energy-path-zone-group-acceptance.md | [ZoneGroup](#zonegroup) |
