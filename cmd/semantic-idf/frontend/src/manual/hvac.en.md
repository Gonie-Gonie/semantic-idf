# Static HVAC systems and service paths

The **HVAC** tab interprets equipment, loop and node references in the input.
It builds a static service model: which modeled systems condition a Zone or
Space, which components deliver that service, and how supported loops couple.
It does not run a hydraulic, airflow or thermodynamic solver.

## Investigate a served Zone {#hvac-workflow}

1. Open HVAC after current analysis is ready and start at **Zone services**.
2. Choose a Zone/Space and inspect its cooling, heating, ventilation, exhaust
   or direct-service paths.
3. Use **Service**, **Path** and **Medium** filters to isolate the relationship.
4. Select a delivery component or conditioning source and read its local details.
5. Follow a related Air Loop, Plant Loop or other loop to inspect circuit structure.
6. Review unresolved references/warnings and compare with the source input.
7. For measured operation, use Simulation HVAC inspection after a suitable run.

The loop pickers distinguish Air Loops, Plant Loops and Other Loops. The latter
includes supported loop roles outside the two principal groups. Fit changes
the diagram extent. Zone services returns to the overview; Clear focus removes
the local focus; Back/Forward restore local HVAC navigation.

HVAC selection is independent of the common Metrics/Profile/Topology selection.
Selecting a component does not automatically reveal its input or switch other
result tabs. Inspect the displayed object type/name and locate it through an
input filter when source editing or reference verification is needed.

## Service, path and medium {#service-model}

| Concept | Meaning |
| --- | --- |
| Served subject | Zone or Space addressed by the service relationship |
| Service kind | Cooling, heating, ventilation, exhaust or direct service, with supported specialized variants |
| Path type | How conditioning reaches the served subject: central air, direct Zone, hydronic, refrigerant, radiant or local |
| Conditioning component | Equipment that supplies a conditioning role along the path |
| Delivery component | Terminal or Zone equipment that delivers the service to the served subject |
| Delivery wrapper | Referencing object such as AirDistributionUnit, retained separately from its terminal |
| Coupling | Supported relationship between systems, loops or different physical media |

Medium filters include air, chilled water (CHW), hot water (HW), condenser water
(CW), refrigerant, electricity, fuel and service water. A path can involve more
than one medium. A water coil transfers heat between air and water; those
streams are not merged into one node merely because the same equipment has
both roles.

A path records source systems and any resolved AirLoop, PlantLoop, CondenserLoop
or refrigerant system. It retains conditioning, delivery and supporting coupling
references separately. A blank upstream relationship is not permission to
invent a central source for otherwise local equipment.

Service classification follows supported object roles and references, not
display-name keywords alone. Similar names such as “Cooling Pump” do not by
themselves establish cooling service or a physical connection.

## How loop structure is reconstructed {#loop-algorithm}

Loop objects identify supply/demand inlet/outlet nodes, BranchLists and
ConnectorLists. BranchLists resolve to Branch objects; each Branch expands
ordered component type/name/inlet/outlet declarations. Typed component lookup
then checks the actual referenced object and its supported node fields.

Splitter and mixer declarations identify parallel branches and convergence.
The renderer projects lead-in, parallel and lead-out branches using those
declarations; its compact routing is not an inferred engineering piping layout.
Diagram positions and connector lengths have no physical distance meaning.

AirLoop demand reconstruction follows SupplyPath/ReturnPath components,
ZoneSplitters/Mixers and supported plenums. Zone/Space equipment connections
resolve EquipmentLists, Zone air/inlet/exhaust/return nodes and NodeList
expansion. Equipment order and cooling/heating sequence metadata are retained
where supported; they are not operating-hour fractions.

An AirDistributionUnit references its terminal. The analyzer checks the terminal
outlet against the ADU outlet and Zone inlet, and whether its inlet belongs to
the AirLoop demand path. Listing a terminal in an EquipmentList alone is weaker
evidence than a complete matching node chain.

Cross-loop relationships use evidence such as the same water coil occurring
in air and plant contexts, or a chiller connecting its chilled-water circuit to
a condenser circuit. Shared equipment identity is preserved across occurrences.
One coil drawn in two loop contexts is not two separate installed coils.

Rule-derived relationships retain source object/field and rule traces. Missing
or inconsistent declarations remain warnings; the analyzer does not silently
repair node names or add guessed branches to make the graph connected.

## Equipment-family interpretation {#equipment-families}

| Family | Static relationship to inspect | Important boundary |
| --- | --- | --- |
| Central air with water coils | Zone terminal/ADU → AirLoop conditioning → coil's PlantLoop | Air delivery and water-source circuit are separate roles |
| Fan coil or hydronic Zone equipment | Local fan/coil delivery and resolved water loops | A local fan coil can have plant support without an AirLoop |
| PTAC/PTHP, window AC, unit heater | Equipment wrapper, internal coils/fan and direct Zone service | Do not invent a remote plant for self-contained equipment |
| VRF terminal | Terminal → terminal list → refrigerant source/system | Shared outdoor system identity matters across recipient Zones |
| Water-to-air/central heat pump | Supported native ports, loops and recipient references | Heating/cooling/recovery roles depend on actual ports and topology |
| Radiant or radiant-convective equipment | Served Zone, active surfaces/surface groups and applicable water/electric source | Surface-mediated delivery differs from direct Zone-air output |
| District heating/cooling | Purchased thermal source and its supported loop/service | A district source is not onsite combustion or onsite compressor electricity |
| IdealLoads | Declared direct ideal conditioning service | Ideal delivered load does not establish a real equipment energy conversion |
| Ventilation/exhaust/ERV | Air nodes, outdoor/exhaust paths and related fan or recovery device | Ventilation can exist without a cooling/heating service |

Coverage is object- and version-specific. Recognizing one family does not prove
that every variant, extensible list, control strategy or new engine object is
fully resolved. Read warnings at the path, component and loop levels. Unserved
Zones/plenums may be intentional; a missing modeled service needs investigation
rather than a guessed assignment.

## Read equipment availability and control {#component-values}

Select equipment in the loop diagram to open three graphs: **Availability**,
**On / Off decisions**, and **Parameter control**. Shared
equipment is inspected in the selected loop's
context, so another circuit's setpoint is not automatically its control target.

Missing, unsupported or runtime-dependent inputs remain unresolved. Older saved
analysis may need reanalysis of its source input to show the new control graphs.

### Availability {#availability-definitions}

Related availability schedules are stacked as **ON / OFF** time bands, with
**Final availability** at the bottom. A source schedule is ON when positive and
OFF otherwise; unavailable inputs remain distinct. These source flags are not
individual manager decisions. The final row applies the modeled manager rules
and overrides, so it is not a simple AND of the rows. For example, a CycleOn
manager can override a fan's own OFF schedule. Final ON permits operation and
Final OFF blocks it; neither establishes whether the equipment actually ran.

Choose **Rep. day**, **Rep. week** or **Annual**. Representative views start on
January 1 of the current year, using Monday as the representative starting day;
Annual follows that year's regular Gregorian weekdays. The calendar does not
apply simulation holidays, daylight saving or runtime overrides. Managers that
need unknown temperatures, loads or external inputs stay unresolved.

Use the wheel to zoom the time axis and drag to pan. All rows share the time
range and cursor; hover to compare their statuses at the same time. Source
minute transitions and known OFF versus unknown intervals are preserved.
The detailed decision logic is shown in **On / Off decisions**.

### On / Off decisions {#on-off-decisions}

Follow the configured permission and equipment request through decision nodes
to ON, OFF or an unresolved branch. Branch labels describe the condition being
tested. Availability alone does not establish a load request, dispatch selection
or actual operation. A decision that needs runtime temperature, load or another
unavailable input stays unresolved.

The graph preserves alternatives and combination rules. Air/Zone manager
precedence and Plant manager assignment priority are different rules; several
manager references do not imply a sequence of independent AND conditions.
Limits and dispatch ranges use the declared source fields. They do not supply
invented thresholds or a simulated decision for the inspection period.

### Parameter control {#parameter-control}

Trace how an equipment command is determined and what states result from it:

| Role | Meaning and examples |
| --- | --- |
| External input | A demand, schedule, setpoint or inlet condition supplied from outside the selected equipment |
| Controller / setpoint | The referenced object that compares a sensed quantity, supplies a target or applies a supported control rule |
| Comparison | An explicitly defined residual or comparison, separate from the actuator's response direction |
| Control variable | A manipulated command, such as requested water/air flow, speed or capacity command |
| State variable | A resulting outlet condition, heat-transfer rate or power, determined by the command, inlet state and equipment model |
| External output | A command or physical outlet quantity passed beyond the selected equipment's boundary |

External ports use the selected equipment/circuit context and retain the
declared node/field identity when resolved. Unresolved occurrence ports do not
justify borrowing another circuit's ports. A physical flow path is not by itself
evidence of a feedback controller. Performance curves
and configured limits explain how states are determined; their presence does
not make every calculated state a separately controlled variable.

The graph's **Control variable** role means a manipulated command. EnergyPlus
Controller:WaterCoil uses the source field **Control Variable** for the sensed
or regulated quantity, such as outlet air temperature. The raw field retains
that native meaning; **Actuator Variable** identifies the manipulated flow.

Signed links appear only where a supported rule establishes the sign. Read the
relation label together with the sign: a comparison sign and the controller's
Normal/Reverse Action describe different parts of the control. An unknown sign
does not imply positive feedback. Feedback arrows describe dependencies and
may return to a sensor; their drawing order is not a simulation execution order.
Use Simulation HVAC and actual output variables to inspect numerical operating
states and convergence.

For a supported Controller:WaterCoil temperature comparison, EnergyPlus uses
`residual = sensed value − setpoint`: the measurement enters with **+**, the
target with **−**. These are residual signs, not the actuator's response sign.
The native **Action** field separately describes the response to increasing
actuated flow: Normal for a heating response and Reverse for a cooling response.
See the [EnergyPlus controller reference](https://bigladdersoftware.com/epx/docs/24-2/input-output-reference/group-controllers.html#field-action)
and [native residual calculation](https://github.com/NREL/EnergyPlus/blob/v24.2.0/src/EnergyPlus/HVACControllers.cc#L1072).

TemperatureAndHumidityRatio control keeps its temperature and maximum humidity
ratio targets separate. Resolving one provider does not establish the other;
a missing target stays an explicit unresolved external input. Supported humidity
setpoint-manager types identify their target quantity even when they have no
Control Variable field. Humidity and combined-mode dependencies retain unknown
comparison signs where their detailed comparison/selection is not resolved.

### Example: trace a chilled-water service {#chilled-water-example}

Start with a Zone VAV terminal and verify its ADU/outlet-to-Zone inlet evidence.
Follow the AirLoop to its cooling water coil. Find that coil's resolved demand
branch on the chilled-water PlantLoop, then the supply chiller and any condenser
relationship. This establishes modeled connectivity. It does not establish
the coil's annual load, the pump's electricity or whether every hour has enough
flow. Those require outputs from an actual run.

## Missing or misleading graph relationships {#hvac-problems}

| Symptom | Check |
| --- | --- |
| Equipment appears but no Zone service path | EquipmentConnections/List target, terminal wrapper and inlet/outlet chain |
| Missing branch or connector | Exact BranchList/ConnectorList names and referenced object existence |
| Plant relationship is absent | Actual water ports and demand/supply branch occurrence, not just object naming |
| A Zone is linked to several systems | Distinct valid paths, service roles, sequences and shared component occurrences |
| Same component appears several times | Whether occurrences refer to one definition in different contexts |
| Direct Zone equipment has no AirLoop | This may be correct for local/hydronic/refrigerant delivery |
| Graph is visually connected but simulation fails | Engine node rules, controls, sizing, schedules and error output still need checking |

Warnings describe declared-reference failures, supported-rule checks or analyzer
limitations. A visually incomplete graph may reflect unsupported modeling;
a complete-looking graph is not proof of a successful engine solve.
Use Diagnose to distinguish rule issues from limitations and inspect the
underlying source object before replacing a relation.

## Safe field edits and output monitors {#hvac-edits}

Where a component exposes an Edit action, the editor accepts the supported safe
field set rather than arbitrary object restructuring. Availability schedules,
supported capacities/control fields and other exposed fields retain their
object/field identity. Validate the unit and allowable numeric or Autosize form
for that specific field.

Run **Preview** before **Apply**. Preview reports source changes and warnings;
invalid targets, out-of-range fields and unsafe fields prevent application.
Changing the form invalidates the previous preview. After applying, analysis
refreshes for the changed working document; **Save** persists it separately.

Node/component **Add monitor** actions create explicit Output:Variable requests
where supported. Check key value, variable name, reporting frequency and the
optional schedule. An existing matching request can already cover the monitor.
Adding a monitor changes the input for a subsequent run; it does not create
observations in an old result bundle.

The available monitor suggestions are contextual conveniences. Engine versions,
equipment and valid output-variable dictionaries govern whether a request is
actually produced. Inspect the run's available output sources before diagnosing
a missing reading as zero operation.

## Simulation comparison and machine exports {#hvac-exports}

Static HVAC and Simulation HVAC inspection answer different questions. Static
analysis supplies declared loops, branches, equipment and service recipients.
Simulation inspection overlays reported operation, temperatures/flows where
available, loads, electricity/fuel and reported ratios for the selected period.
Match the source model and scope before comparing them.

For reproducible machine analysis, `hvac-graph` exports rule, service or coupling
navigation JSON using `semantic-idf.hvac.graph.v1`. For example:

```text
semantic-idf hvac-graph --graph service --format json model.idf
semantic-idf hvac-graph --graph rule --format json model.idf
```

The rule graph explains source-derived relations; the service graph expresses
delivery to recipients; the coupling graph expresses supported system/medium
links. Optional Debug facilities expose analysis graphs when enabled, but are
not required for the normal Zone-services/loop workflow.

Preserve object identity, occurrence context, medium and role when joining these
exports. Graph connectivity is not an energy allocation algorithm; shared
energy pools and conversion quantities follow Simulation/Energy Path rules.
