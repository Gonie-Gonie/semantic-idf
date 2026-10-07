# Units, glossary and troubleshooting

Use this chapter when a number, graph or status seems inconsistent. First establish the quantity's source, unit, scope, period, area basis and knownness. Comparing two labels that look alike is not enough.

## Interpretation checklist {#checklist}

For every important value, record:

1. **Identity:** which object, source dictionary, run and Zone/Building?
2. **Quantity:** energy, power, temperature, flow, area or a ratio?
3. **Domain:** thermal load, site consumption, generation, context or geometric evidence?
4. **Time:** Annual, month, instantaneous frame or reported interval?
5. **Multiplier:** representative source or already model-total observation?
6. **Denominator:** physical/effective area, served load, complete owners or another budget?
7. **Knownness:** observed finite value, observed zero, partially known or absent?
8. **Presentation:** rounding, sampling, grouping or allocation?

For example, 2 kWh/m² from one Zone and 2 kWh/m² from a Building do not necessarily represent the same absolute quantity. Multiply each by its corresponding area before comparing totals.

## Unit reference {#units}

| Unit | Quantity | Common interpretation error |
| --- | --- | --- |
| m² | Surface/floor area | Mixing representative physical and multiplier-adjusted effective area |
| m³ | Volume | Replacing a missing volume with an invented height |
| W | Power or heat-flow rate | Reading a frame value as annual energy |
| kWh | Energy | Treating load and consumed site energy as the same domain |
| kWh/m² | Area-normalized energy | Ignoring the selected scope's denominator |
| W/m² | Power density | Treating a design density as simulated average consumption |
| W/(m²·K) | U-value | Treating partial construction coverage as whole-envelope coverage |
| W/K | UA, heat-transfer conductance | Reading static UA as a measured heat-flow rate |
| °C | Temperature | Adding temperatures like independent energy contributions |
| kg/s, m³/s | Mass/volume flow rates | Comparing them without density or compatible observations |
| ACH, 1/h | Air changes per hour | Confusing total airflow with a per-volume rate |
| people/m², m²/person | Occupancy density / area per person | Inverting zero or unknown occupancy |
| m³/s/person | Outdoor airflow per person | Using a different or unknown occupancy denominator |
| fraction, [-] | Dimensionless value | Assuming every fraction is a probability or efficiency |

Basic unit conversions:

```text
1 kWh = 3,600,000 J
energy (kWh) = Σ[power (W) × interval seconds] / 3,600,000
UA (W/K) = U (W/(m²·K)) × area (m²)
ACH (1/h) = volume flow (m³/s) × 3,600 / volume (m³)
```

Use the actual reporting interval when integrating rates. A monthly aggregate attached to a particular Time row is not an instantaneous rate. Missing intervals cannot be silently assigned a full month.

U×A is static conductance. Estimating heat flow with UA×temperature difference requires an additional temperature/physical model and does not turn the app's static UA into a measured simulation result.

## Physical and effective quantities {#multipliers}

A representative Zone can stand for several physical instances. For supported Zone/ZoneGroup inputs, the effective factor combines the native Zone multiplier and group/list multiplier.

Representative Zone quantities can need that factor once. Component energy and meters already covering a model-total pool must not receive it again. Different variable families have different authority; multiplying every SQL value by the Zone factor is incorrect.

For factor 6 and physical area 24 m², effective area is 144 m². A representative Zone energy of 10 kWh becomes 60 kWh when the observation requires multiplication. 60/144 and 10/24 are both about 0.4167 kWh/m². Multiplying the energy twice gives a false intensity.

Surface area, Floor Area and thermal pair area have their own basis rules. Use the feature's stated basis rather than transferring one multiplier rule to all metrics.

## Zero, missing and partial {#knownness}

| State | Meaning | Correct response |
| --- | --- | --- |
| Observed positive/negative | A valid source provided a finite quantity | Preserve sign, unit and source role |
| Observed zero | Complete valid observations establish zero | Keep availability; ratio applicability may still depend on a denominator |
| Missing/absent | Required source, value or ownership evidence unavailable | Do not replace it with zero |
| Partial | Some supported inputs/coverage are known | Inspect coverage and omitted/unresolved sources |
| Not applicable | The requested relation has no meaningful denominator or service | Do not report an invented efficiency |
| Rounded display zero | Presentation erased a small nonzero value | Inspect original/exported quantity before asserting exact zero |

Statuses are feature-specific. A Metrics status, run status and Energy Path quality label describe different contracts; do not map every word partial or ready to one universal score.

A source dictionary name alone does not establish complete observations. A requested output may be missing; an unrequested output may be absent for an expected reason. SQL NULL and no row are not observed zero.

## Precision, grouping and sampling {#precision}

Energy Path commonly displays kWh/m² with two decimal places. A positive 0.004 kWh over 100 m² is 0.00004 kWh/m² and can display as 0.00. The source still contains positive energy.

Aggregation and allocation have their own precision contracts. Completed monthly allocations are summed for Annual; do not independently allocate Annual totals and expect identical per-Zone remainders. Small rounded budgets require deterministic remainder handling.

Other grouping changes presentation membership without changing original source identity or quantities. A material category must remain visible under the grouping rule even if an older fixed node-count preference would hide it.

Sampled overview/frame data improves inspection performance. It is not a substitute for full authoritative energy aggregation or a complete Hourly source trace. Two charts can show different sampled time points while referring to the same underlying run; check their source and matching-time rules.

## Glossary {#glossary}

| Term | Meaning in this application |
| --- | --- |
| Source identity | Original object/dictionary/key/unit/frequency/path identity used to qualify a quantity |
| Provenance | Retained evidence connecting result to input, output, engine/weather and transformations |
| Thermal boundary | Declared EnergyPlus relationship between a surface/Zone and its thermal target |
| Geometric adjacency | Spatial-nearness/overlap evidence used for QA, not authority to invent a thermal connection |
| Air coupling | Supported declared transfer/mixing relationship; separate from opaque conductance |
| Static UA | Input-derived conductance of supported known constructions and canonical boundaries |
| Load driver | Displayed contribution allocated to actual thermal load; raw directional pressure supplies allocation weights rather than another consumed-energy budget |
| Delivered thermal load | Selected heating/cooling delivered energy in the thermal domain |
| Site energy | Consumed carrier energy, separate from delivered thermal quantity; purchase, generation and storage-supply observations retain their own accounting/context roles |
| Direct energy | Observation with established direct recipient identity |
| Allocated energy | Observed shared budget distributed under supported ownership/load policy |
| Unassigned energy | Budget retained at Building level when a component/Zone split is not established |
| Non-additive context | Useful observation that must not be added again to a selected load or budget |
| Reconciliation | A comparison/closure ledger; it does not authorize fabricating absent observations |
| COP / efficiency ratio | Supported paired thermal/site ratio with matching scope and source evidence |
| Run copy | Separately prepared execution input carrying run controls/output requests |
| Reporting calendar | Qualified engine environment/time records, excluding invalid warmup/sizing context |

## Symptom-based troubleshooting {#troubleshooting}

| Symptom | First checks | Read next |
| --- | --- | --- |
| Model looks empty or analysis fails | File content/Version, parse issues, unresolved required fields | [Input](./input.en.md) |
| Input object is hard to find | Shared filter, real name/index, current selection and reference ambiguity | [Input](./input.en.md) |
| Area differs between panels | Physical/effective basis, Zone/Group factors, gross/net opening treatment | [Metrics](./metrics.en.md), [Topology](./topology.en.md) |
| Low U-value but suspicious coverage | Supported layers, missing constructions, coverage denominator | [Metrics](./metrics.en.md) |
| Nearby Zones have no Network edge | Declared boundary references rather than visual proximity | [Topology](./topology.en.md) |
| Profile patterns unexpectedly match/differ | Source weights, schedule resolution, comparison mode/tolerance | [Metrics and Profile](./metrics.en.md) |
| HVAC path is missing | Supported object ports, exact Node references, branch/Zone service proof | [HVAC](./hvac.en.md) |
| Engine is not found | Settings installation path, requested version and executable | [Tools](./tools.en.md) |
| Run succeeds but Hourly chart is unavailable | Purpose/output requests, actual source identity, reporting frequency/calendar | [Simulation](./simulation.en.md) |
| Receiving results is slow | Run completed vs reading/transport/display stage, output volume | [Simulation](./simulation.en.md) |
| Zone consumption excludes Building energy | Direct/allocated/unassigned policy and complete recipient cohort | [Energy Path](./energy-path.en.md) |
| COP is blank despite energy | Pairing, thermal/site domains, service/scope/month and denominator availability | [Energy Path](./energy-path.en.md) |
| Batch delta is not comparable | Unit, status and U-value coverage mismatch; zero baseline for percent | [Tools](./tools.en.md#deltas) |
| Export differs from current editor | Retained executed input and saved-result identity | [Automation](./automation.en.md) |

Work from the source outwards: input and run identity, requested output, actual observations, supported transformation, then presentation. Changing a chart's scale cannot fix an absent source or unresolved physical ownership.

## Scope and support limits {#support-limits}

The analyzer handles supported object families and evidence patterns. An EnergyPlus object may execute successfully yet lack a recognized static graph, ownership rule or Energy Path conversion in this application.

A full synthetic/offline test pass, successful engine run and independent physical/numeric model review answer different questions. The manual describes supported interpretations rather than approval of every equipment type, climate or design.

When extending or comparing a model, preserve unresolved status, original outputs and independent evidence. Useful diagnostic context should remain visible even when it cannot receive an additive allocation.
