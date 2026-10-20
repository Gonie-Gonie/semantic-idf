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

Select equipment in the loop diagram to open three panels: **Availability**,
**On / Off decisions**, and **Parameter control**. Shared
equipment is inspected in the selected loop's
context, so another circuit's setpoint is not automatically its control target.

Missing or unsupported configuration remains unresolved. Older saved analysis
may need reanalysis of its source input for the updated availability/control views.

### Availability {#availability-definitions}

The leftmost label identifies the object that owns each availability condition.
Related schedules are stacked as **ON / OFF** bands, with **Configured availability**
at the bottom. A source schedule is ON when positive and OFF otherwise. The final
row combines the minimum configured static conditions, including supported
scheduled-manager overrides; it is not a simple AND of every source flag.

NightCycle decisions, runtime temperatures/loads and actual equipment dispatch decisions
are excluded from this view. They do not turn the static result into Unknown.
Missing or ambiguous references and unresolved configuration remain Unknown.
When the equipment has no separate availability schedule, its own row shows
**No separate schedule** with an unrestricted band. This does not remove loop or
plant-operation
activation conditions. Configured availability describes the static configuration;
actual runtime control and whether the equipment ran belong to simulation.

For example, Chiller:Electric:ReformulatedEIR has no own Availability Schedule
field. Its unrestricted own row is expected; the PlantEquipmentOperationSchemes
activation schedule and exact equipment-list membership still restrict its
configured availability. See the [EnergyPlus chiller reference](https://bigladdersoftware.com/epx/docs/24-2/input-output-reference/group-plant-equipment.html#chillerelectricreformulatedeir)
and [plant operation controls](https://bigladdersoftware.com/epx/docs/24-2/input-output-reference/group-plant-condenser-control.html).

Choose **Rep. day**, **Rep. week** or **Annual**. Representative views start on
January 1 of the current year, using Monday as the representative starting day;
Annual follows that year's regular Gregorian weekdays. The calendar does not
apply simulation holidays, daylight saving or runtime overrides.

Use the wheel to zoom the time axis and drag to pan. All rows share the time
range and cursor; hover to compare their statuses at the same time. Source
minute transitions and known OFF versus unknown intervals are preserved.
The detailed decision logic is shown in **On / Off decisions**.

### On / Off decisions {#on-off-decisions}

Start with **Run permission**: the full gate for allowing this equipment to
operate, including manager overrides and runtime plant-equipment selection.
This is broader than the minimum static conditions in **Configured availability**.
**On command** combines run permission with the equipment's activation condition,
for example `Run permission AND airflow request`. **AND** requires every
condition; **OR** requires at least one. Schedule comparisons retain the original
schedule name. These expressions describe control logic, not simulated ON/OFF
results.

Repeated manager rules have one named definition, referenced from the conditions
that use them. Read the manager's source object and inputs there rather than
expanding the same rule at every reference. A NightCycle definition, for example,
shows its declared applicability schedule, fan schedule, control type, tolerance
and cycling settings when present. Its response also depends on runtime zone
temperatures, thermostat conditions and cycle timing, so schedules alone cannot
determine the resulting status. **Runtime manager decision**, **Dispatch
selection**, and **Runtime control decision** identify where runtime inputs are
needed. Missing references, unsupported definitions and reanalysis requirements
have separate names. **Airflow path unresolved** means that the analysis cannot
establish the connection; it is not an ordinary runtime input.

**Manager permission** converts a manager status into equipment permission;
**Fan permission** also applies the fan's schedule and manager override. Read
the manager status as follows:

| Manager status | Permission |
| --- | --- |
| ForceOff | Blocked |
| CycleOn | Allowed |
| CycleOnZoneFansOnly | Zone fans allowed; central fan blocked |
| NoAction | Follow the fan's own schedule; a manager-only permission condition imposes no further restriction |

NoAction means that the manager does not intervene. A fan override can therefore
allow a fan despite its own schedule, or block it despite an ON schedule. Other
permission conditions still apply. Air/zone manager selection uses the priority
**ForceOff → CycleOnZoneFansOnly → CycleOn → NoAction**. Plant manager selection
uses the first non-NoAction result in the declared list order. Named rules keep
these meanings; they are not ordinary AND/OR combinations of manager statuses.

For supported active equipment, the activation condition names its source and
dependencies. Connected load, thermostat and dispatch objects are inputs to that
decision, not automatically separate AND conditions. Continuous/default pump
mode does not turn “no flow request” into a proven OFF; passive equipment has no
active ON/OFF command. Unknown inputs stay unresolved, and missing expression
metadata requires reanalysis. The Availability timeline above continues to show
only the minimum static conditions.

### Parameter control {#parameter-control}

Follow the arrows through four groups: **Inputs / targets**, **Comparison /
controllers**, **Control commands**, and **Equipment response / outputs**.
Each compact block represents a source object. Blocks share a width sized to
their labels. Full labels and source names wrap when needed, and block heights
grow to keep the text visible. Once the blocks have enough width, a wider panel
adds space around and between them for the connecting paths. Nearby connections
use short, smooth paths, with the main controller, command and response aligned
where their connections allow. In the wide layout, longer direct inputs use upper lanes;
long returning references and feedback use lower lanes with broad rounded turns.
Hover or focus a path or block to emphasize its connections and endpoints. Select
a block to inspect its original fields, ports, members and connections below the
diagram. A schedule summary in the inspector describes the representative day,
not its annual range.

The legend distinguishes **Signal**, **Reference** and **Measured feedback**.
Measured feedback includes a measurement entering a supported comparison with
**+**. Backward measurement or setpoint references remain references unless the
native relationship identifies feedback. A **?** marks an unknown polarity, not
a missing connection or proven positive feedback.

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

The **Control variable** role means a manipulated command. EnergyPlus
Controller:WaterCoil uses the source field **Control Variable** for the sensed
or regulated quantity, such as outlet air temperature. The raw field retains
that native meaning; **Actuator Variable** identifies the manipulated flow.

Known **+ / −** signs appear only where a supported rule establishes them. Read the
relation label together with the sign: a comparison sign and the controller's
Normal/Reverse Action describe different parts of the control. An unknown sign
does not imply positive feedback. Feedback references retain their source and
meaning without repeatedly expanding a cycle; diagram order is not simulation
execution order.
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
