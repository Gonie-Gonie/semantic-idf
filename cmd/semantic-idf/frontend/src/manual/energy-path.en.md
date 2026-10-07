# Energy Path

Energy Path reads left to right: **Load drivers → Thermal loads → End-use energy → Energy sources** for a saved simulation. It combines actual observations with
explicitly labelled allocations. It is a load explanation and energy-accounting
view, not a calculation of savings caused by a model change.

Run Basic Energy as described in [Simulation](./simulation.en.md#purposes).
Use [Tools](./tools.en.md) for Building/Annual comparisons and
[Automation](./automation.en.md) for file/API consumption.

## Read the four stages {#reading-the-four-stages}

| Stage | Quantity | Domain | Example |
| --- | --- | --- | --- |
| Drivers | Contribution allocated to actual cooling or heating load. | Thermal energy. | Exterior-wall cooling contribution. |
| Loads | Observed thermal service at its declared boundary. | Thermal energy. | Zone sensible cooling. |
| End uses | Consumption by equipment or direct uses. | Site energy. | Coil electricity, lighting, gas heating. |
| Carriers | Facility or scoped consumption by energy resource. | Site energy. | Electricity, natural gas, purchased district cooling. |

Both thermal and site quantities commonly use kWh, but they use separate
graph scales. A 100-kWh cooling load served by 25-kWh electricity is valid.
The 75-kWh difference is not an accounting leak. Pixel widths across domains
do not establish efficiency.

Driver-to-load ribbons use the same allocated thermal quantity at both ends.
Load-to-end-use connections retain different thermal and site quantities.
End-use-to-carrier ribbons use the same site quantity at both ends.

A direct use such as lighting can enter site consumption without becoming
HVAC consumption. Lighting heat and lighting electricity are related source
evidence; they are not two interchangeable measures of one conserved quantity.
People gains do not imply a purchased People-energy end use.

## Scope, period, and component charts {#scope-and-period}

Scope and Period default to Building and Annual. Choose **Scope** and **Period**
for the saved result. Building and Zone results
already have their own accounting. A Zone is not necessarily one physical
copy: its model-total quantities can include Zone/Group multipliers.

Annual and available months are the interactive periods. A missing Zone,
missing month, or empty selection cannot substitute another Zone or Annual.
Changing display scope does not rerun the model.

Cooling and heating are displayed together. Select a component to see a
Monthly or Hourly graph:

- Monthly shows the component's calculated contributions for the selected scope.
- Hourly shows actual reported source energy, not invented hourly allocation.
- A selected month limits the Hourly calendar to that month.
- Unavailable Hourly identity, calendar, or area produces an unavailable chart.

A shared component can have a reported Building Hourly source while its
Monthly Zone contribution is allocated. These charts need not sum to the same
value. Do not interpret the Hourly source as newly measured Zone consumption.

Native Hourly charts can have more observations than the generic Series preview.
A rounded chart point does not establish that the native source was exactly zero.

## Floor area and intensity {#floor-area-and-intensity}

Energy quantities in the screen use kWh/m² with two decimal places.

```text
Intensity [kWh/m²] = selected energy [kWh] / executed scope floor area [m²]
```

Building uses eligible total floor area from the executed model, with the
applicable Zone/List factors. Zone uses that Zone's floor area and applicable
factors. A zero-load plenum can still contribute Building area when the model
includes it in total floor area.

The denominator belongs to the run. Later input edits cannot change it.
Missing or unusable area means unavailable intensity, not zero.

For a 1,000-m² Building, 2,500 kWh displays as 2.50 kWh/m².
For a 100-m² Zone, 500 kWh displays as 5.00 kWh/m².
Do not add intensities across different denominators; sum the compatible
energies and areas first. A simple average of Zone intensities is generally
not the Building intensity.

Ratios, percentages, temperatures, water volumes, and exported absolute energy
totals retain their own units.

## Driver categories and signed pressure {#driver-pressure}

Drivers are classified using the observed variable and the executed model's
surface, zone, and boundary relationships.

| Group | Interpretation |
| --- | --- |
| Exterior walls, roofs, ground, windows | Source-supported surface exchange or window-related heat. |
| Interzone surfaces and air | Exchange between model zones; internal Building transfer is not a new external input. |
| People, lighting, equipment | Reported internal gains. |
| Infiltration, mechanical ventilation | Distinct outdoor-air gain/loss observations. |
| Fan or other HVAC-related heat | Heat entering the thermal balance, separate from consumed fan electricity. |
| Other/storage | Remaining supported load explanation; can include unresolved balance/storage rather than a measured battery. |

A surface can participate in active radiant or pool heat exchange. Its
reported convection is not automatically an isolated passive-envelope effect.
Construction UA alone does not create a simulated driver amount.

Effective signed pressure preserves direction. Positive heat adds cooling
pressure; negative heat adds heating pressure. Native gain/loss aliases can
report positive source energies while representing opposite heat directions.

```text
Cooling pressure P_i = max(signed effective heat H_i, 0)
Heating pressure P_i = max(-H_i, 0)
```

Opposite-sign pressure is retained as offset context. It is not an additional
negative load, an avoided-energy quantity, or a savings estimate.

## Allocate drivers to actual load {#driver-allocation}

For each Zone, month, and service, allocation uses the actual load L and
eligible pressures P:

```text
Allocated driver A_i = L × P_i / sum(P_j)
```

This heat-balance share is a deterministic explanation, not a causal fraction
of the utility bill. Synthetic closure terms do not enter the pressure weights.

Example: cooling load is 120 kWh; wall pressure is +60 kWh, lighting +40 kWh,
and an opposing exchange is -30 kWh.

| Driver | Cooling weight | Allocated cooling contribution |
| --- | ---: | ---: |
| Wall | 60 / 100 | 72 kWh |
| Lighting | 40 / 100 | 48 kWh |
| Opposing exchange | 0 / 100 | 0 kWh |

The -30 kWh remains offset evidence. The raw signed balance is 70 kWh, but
the actual cooling load remains 120 kWh.

Zero actual load gives zero allocation even if pressure exists.
Positive load without positive eligible pressure is assigned to Other/storage
with zero raw pressure. This establishes an unexplained/storage contribution;
it does not invent a physical heat source.

Missing observations cannot be turned into the exact-zero proof needed for
a zero-pressure fallback. Small real signals retain their meaning before
presentation rounding.

## Month-first aggregation and rounding {#aggregation-and-rounding}

Allocation happens before aggregation. Annual and Building sum completed
Zone-month contributions. The app does not redistribute a net annual pressure.

| Month | Source signed heat | Actual service load | Completed contribution |
| --- | ---: | ---: | ---: |
| January | -100 kWh | Heating 80 kWh | Heating 80 kWh |
| July | +100 kWh | Cooling 120 kWh | Cooling 120 kWh |
| Annual evidence | Net 0 kWh | Two separate services | Heating 80 + cooling 120 kWh |

Seasonal cancellation in the source's annual net does not erase gross
directional contributions. An allocation factor is not obtained by dividing
their sum by a zero net source.

Completed simultaneous cooling/heating requires the same Zone-period pair.
Cooling in one Zone and heating in another month do not prove same-time
operation. High-resolution operating evidence has its own observation boundary.

Display accounting commonly uses 0.001-kWh quanta. Proportional branches first
receive their floor quota; the remaining quanta follow largest fractional
remainders, with stable identity for ties. Each branch stays within its own
floor/ceiling quota and the source budget closes.

For a 0.001-kWh budget shared equally by three recipients, one receives
0.001 kWh and two receive 0.000 kWh. Assigning 0.001 to all three would triple
the budget. Three-decimal accounting and two-decimal intensity display are
different rounding steps.

## Direct, allocated, and unassigned consumption {#allocation-bases}

| Basis | Meaning |
| --- | --- |
| Direct/reported | Exact component or Zone observations establish consumption, including a valid zero. |
| Allocated | A supported source budget is distributed using service paths or load shares. |
| Unassigned | Building consumption is observed, but a Zone/service split is not established. |

The ordinary service-path policy uses exact direct evidence first, then
matching HVAC path load share, then matching-service Zone load share.
The remaining energy stays unassigned.

Direct consumption is removed from the same undifferentiated pool before
allocation; it cannot receive that pool's remainder a second time.
A separately observed shared component is a different budget and can
legitimately serve a Zone that also has local equipment.

Fans can use complete related supply-air volume evidence where available,
otherwise supported load shares. Pumps and heat rejection require the
relevant water/condenser paths. An ambiguous path, incomplete recipient
roster, or unquantified non-Zone demand can prevent assignment.

Example: A has 30 kWh of direct consumption. A separately measured 100-kWh
shared component serves A/B with valid weights 60/40. Another 20-kWh source
has no established recipient split.

| Accounting location | Quantity |
| --- | ---: |
| Zone A | 30 direct + 60 allocated = 90 kWh |
| Zone B | 40 allocated kWh |
| Building unassigned | 20 kWh |
| Building total | 150 kWh |

Zone coverage is 130/150 = 86.67%; Building consumption can still close fully.
The 20 kWh is not inserted into either Zone. Unassigned means uncertain
distribution, not missing Building energy.

## Multipliers and ownership {#multipliers}

Eligible representative-Zone observations receive Zone × ZoneGroup factors
once. Facility meters and already-model-total system/component observations
do not receive another factor. Geometry multipliers are not blindly reapplied
to native result energy.

Example: one 24-m² representative Zone has Zone factor 2 and group factor 3.
Its effective area is 144 m². An eligible 10-kWh representative cooling load
becomes 60 kWh. An already-model-total 12-kWh coil observation remains 12 kWh.

The corresponding intensities are 60/144 = 0.4167 and 12/144 = 0.0833 kWh/m²,
displayed as 0.42 and 0.08. Multiplying the coil value by six again would
create false consumption.

A group listing zones is not automatically a multiplier declaration.
Unsupported multiplier applicability remains explicit; leaving a factor at
one conservatively does not prove that one is physically correct.

## Conversion ratios {#ratios}

A reported conversion pairs thermal load with its compatible site consumption.

```text
Ratio = paired thermal load / paired site energy
Annual ratio = sum(paired monthly load) / sum(paired monthly site energy)
```

| Example | Result | Meaning |
| --- | ---: | --- |
| 100 kWh cooling / 25 kWh electricity | 4.00 | COP when boundary and electric equipment evidence support it. |
| 85 kWh heating / 100 kWh fuel | 0.85 | Efficiency when the fuel/equipment boundary supports it. |
| 100 kWh load / 125 kWh district energy | 0.80 | Load / purchased energy, not local equipment COP. |
| 100 kWh load / (25 kWh electricity + 10 kWh gas) | 2.857 | Load / site energy across mixed carriers. |

Honor the stated ratio meaning. Electric heating alone does not prove a
heat-pump COP; radiant or mixed thermal boundaries can require Load / site
energy. Water-loop purchased energy does not establish a chiller's electricity.

Fan/pump consumption is not silently added to a coil-only COP denominator.
If the coil has 25 kWh and fan has 5 kWh, 100/25 = 4 and 100/30 = 3.333 describe
different boundaries. Use the reported paired quantities.

An annual ratio uses only overlapping valid paired periods. It does not
replace a paired load with a larger full-year node total, and it does not
average monthly COP values. A zero/missing denominator or unproved pairing
means unavailable ratio.

## Carriers, generation, and storage {#carriers-and-storage}

Consumption carriers include electricity, gas, district cooling/heating, steam,
propane, fuel oils, and other supported fuels, normalized to site kWh.
Water volume remains m³ context unless an explicit energy-equivalent conversion
is supported. Volume is not inserted into energy closure.

Purchased, produced, sold, and storage-discharge electricity are supply context,
not additional end-use consumption. PV DC output, inverter AC output, loss,
ancillary consumption, and storage transfer have different physical boundaries.

Reviewed native EnergyPlus 25.1 storage charge on supported storage buses is
an unmetered transfer, not another Facility consumer. It can remain visible
as source context without an extra consumption or supply ribbon.
Unresolved storage ownership/version/distribution cannot establish that boundary.

Storage heat is not simply charge minus discharge. State-of-charge changes,
losses, circuit boundaries, and actual reported heat matter. Missing SOC
energy cannot be filled by subtracting two observed electrical values.

Ancillary inverter electricity can belong to a consumed end-use budget.
Its parent subtotal and component observation must not both be added.

## Quality and reconciliation {#quality}

The quality line separates Drivers, Loads, End uses, and Carriers.
Select a stage to inspect its source availability in **Data details**.

| Evidence | Interpretation |
| --- | --- |
| Source availability | Found expected requested groups, relative to applicable planned groups. |
| Driver/load closure | How allocated thermal contributions explain observed loads. |
| End-use/carrier closure | How consumed end uses explain facility carrier totals. |
| Ratio availability | Usable, source-traced conversion pairs. |
| Zone coverage | Building-wide direct + allocated shares versus assignable HVAC budgets. |

Run-level availability does not promise every selected month has every value.
Unknown, not requested, not applicable, missing, and measured zero are different.

```text
Closure [%] = 100 × max(0,
  1 - sum(abs(expected - explained)) / sum(expected))
```

For two 100-kWh expected totals, explanations of 90 and 110 kWh give 90%
closure. Their aggregate total is 200 kWh, but opposing errors do not cancel.

A positive material carrier gap can appear as Unclassified energy. A negative
gap remains overmapping evidence. Adding an unclassified residual to the
display does not convert missing coverage into complete measured end-use
coverage. Supply observations do not enter consumption closure.

Zone-assignment coverage remains Building-wide even while viewing a Zone.
It can exceed 100% if direct/allocated evidence overmaps a budget; that problem
is retained rather than hidden by clamping.

## Other groups and source trace {#other-and-provenance}

Automatic Other grouping simplifies selected small contributions below 1%
within the applicable scope/service/domain. It does not remove their original
source records or alter canonical export accounting. Named carriers, essential
envelope/air drivers, loads, HVAC equipment, and auxiliary lanes remain distinct.

Presentation Other is different from Other/storage load explanation or
unassigned Building consumption. Similar display labels do not prove identical
accounting bases.

For a meaningful trace, retain original output name/key, meter/variable type,
frequency, source unit, normalized unit, aggregation method, and relevant
model ownership. A Monthly source scalar can be an annual aggregate; it cannot
fill an absent month. Matching names alone cannot repair another source's
key, frequency, ownership, or calendar.

## Equipment-specific boundaries {#equipment-boundaries}

These examples describe supported accounting patterns. Exact original
equipment, ports, recipients, versions, and observations still matter.

### District energy {#district-energy}

Purchased cooling/heating water is a site carrier. Zone sensible load can pair
with it as Load / purchased energy. A DOAS coil on both air and water paths
does not multiply Zone delivery. Shared Fans/Pumps need their own budgets and
recipient evidence; a hydronic path alone is not individual pump metering.

### Fan coil {#fan-coil}

Cooling and heating water coils belong to their respective plants. Cross-pairing
a heating coil with the cooling plant is invalid. Broad fan or pump meters
can remain unassigned or explicitly allocated even when Zone delivery is
known. Very small actual load with zero pressure uses preserved native
precision and conservative Other/storage explanation.

### Mixed heating fuels {#mixed-heating}

Local baseboard electricity and central boiler fuel/ancillary electricity are
independent budgets. The same Zone can receive both. Count its paired thermal
load once across contributing carriers. Baseboard Total Heating and active
recipient-surface response are overlapping context, not extra load or energy.

### Pool and Zone multiplier {#pool}

Pool water heating is process demand coupled to a surface, not automatically
Zone-air heating. A mixed pool/space plant without quantified process share
cannot allocate its whole Heating consumption to zones. A separately proved
cooling pump can still allocate its own budget. Pool/floor convection is not
isolated passive envelope exchange.

### PTHP {#pthp}

Packaged heat pumps retain DX cooling, DX heating, defrost, crankcase, and
supplemental fuel as distinct supported consuming cohorts. Heating crankcase
energy remains in its proper denominator. Similar output names on a fuel coil
and DX coil require exact typed ownership. Fans stay outside coil-only ratios.

### PV and storage {#pv-storage}

Production, inverter transfer/loss, storage charge/discharge, and ancillary
consumption retain separate roles. Complete energy accounting need not imply
all thermal driver requests are available. A known zero latent gain does not
make positive gas equipment consumption disappear.

### Radiant {#radiant}

Active-surface fluid heat is not same-period Zone-air delivery. Source heat,
surface storage, and convection can differ. A marked radiant or mixed boundary
uses Load / site energy rather than an invented air-delivery COP. Missing
terminal coils, fans, or component observations are not created from topology.

### VRF {#vrf}

Terminal and outdoor-unit consumption can both serve a Zone. Outdoor main and
auxiliary budgets require complete original recipient loads; an unknown
recipient cannot simply be removed to increase the others' shares.
Cooling/heating and crankcase/defrost cohorts retain their service boundaries.
Partial terminal evidence is not a complete system COP.

### ZoneGroup {#zone-group}

Zone and group factors apply once to eligible representative-zone quantities.
Native local coil/fan/baseboard energy can already be model-total. Overlapping
package totals are not additional consumers. HotWaterEquipment is interior
equipment consumption, not space heating merely because it uses hot water.

## Export and comparison {#exports-and-comparison}

Use **Data details → Data → Export** for HTML, XLSX, or full-run JSON.
Summary uses the selected saved Scope/Period; full-run JSON preserves the run.
Optional trace sheets retain original units, source identities, independently
valued link endpoints, and submitted JSON.

Batch Energy Path comparison uses Building/Annual context. Missing rows are
Missing, not zero. A real zero baseline has no ordinary percentage delta.
Comparisons preserve each side's units, basis, source identities, and quality.

Driver shares and observed ratios do not predict savings from changing a
construction, schedule, or setpoint. To compare such a change, run controlled
cases with the same engine/weather/output plan and compare supported observed
quantities. See [Tools](./tools.en.md) and [Simulation](./simulation.en.md#prepare-a-run).
