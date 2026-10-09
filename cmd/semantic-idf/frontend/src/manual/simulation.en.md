# Simulation

Simulation executes the current EnergyPlus model and reads the resulting
observations. Static analysis describes the input; simulation describes one
particular run, engine, weather file, output plan, and period.

Use [Energy Path](./energy-path.en.md#reading-the-four-stages) for energy accounting,
[HVAC](./hvac.en.md) for the static service diagrams, and
[Tools](./tools.en.md) for multiple-file runs.

## Prepare a run {#prepare-a-run}

1. Open the intended input and finish any edits.
2. In Settings, register a usable EnergyPlus installation.
3. Open Simulation and choose the weather file required by the model.
4. Select the purposes needed for this investigation.
5. Choose **Run & Inspect**.
6. Inspect the run status and ERR messages before interpreting the results.

The main view detects the declared EnergyPlus version in IDF or epJSON and
selects a compatible registered installation. A missing compatible engine or
unverifiable installation version prevents that run. Registration alone does
not prove version compatibility.

Weather supplies boundary conditions, including outdoor temperature, humidity,
solar radiation, and wind. Choosing a different EPW is a different experiment.
A model can also contain design days or run-period settings: the visible weather
choice does not replace those input objects.

For comparisons, hold the engine, weather, run period, schedules, and output
frequencies constant unless one is the intended change.

## Run copy and source preservation {#run-copy}

Purpose output requests are added to a copy used for execution. Running does
not silently add those requests to the editable source document.

| Item | Interpretation |
| --- | --- |
| Editable input | The document you view and save. |
| Executed input | The actual model copied into the run directory. |
| Temporary output additions | Requests needed by the selected purposes. |
| Run plan | What this run requested, including existing and temporary requests. |
| Result | Observations produced by that executed input. |

The source may continue changing after a run. Its saved result still describes
the executed copy. A new editor area, construction, or HVAC connection does not
retroactively change the result.

Permanent output editing is a separate explicit operation available to
automation. See [Automation](./automation.en.md). Do not confuse source editing
with the temporary output preparation performed by Run & Inspect.

## Choose purposes {#purposes}

| Purpose | Main question | Typical observations |
| --- | --- | --- |
| Basic Energy | What drives thermal load and site consumption? | Monthly accounting and Hourly component sources for Energy Path. |
| Zone Heat Flow | How does the zone heat balance change with time? | Heat-balance rates, zone temperatures, and detailed surface outputs. |
| HVAC Loop Check | What happened along an executed loop? | Node flow, temperature, humidity, setpoints, and component operation. |
| Comfort | What thermal conditions and reported discomfort occurred? | Zone/People temperatures, PMV, PPD, humidity, and unmet time. |

The main view uses all zones, all loops, and the full run period. Result
Scope/Period selections are display choices applied after execution.
They do not make the engine run only one zone or one month.

The standard energy allocation uses service paths and load shares where
measurements do not establish direct zone consumption. This is explained in
[Direct, allocated, and unassigned](./energy-path.en.md#allocation-bases).

Integrity and custom output capabilities exist in automation; they are not
additional purpose controls in the main view. Batch Simulation has Basic Energy,
Zone Heat Flow, and HVAC Loop Check purposes.

## Output plan and reporting frequency {#output-plan}

An output request identifies a meter or variable, its key, and reporting
frequency. Similar names or the same physical component at different
frequencies do not automatically describe the same observation.

| Output state | Meaning |
| --- | --- |
| Existing | Requested in the source model already. |
| Temporary | Added for this run only. |
| Planned permanent output | Part of an explicit permanent apply operation. |
| Conflict | A similar target has a different frequency or field set. |

The energy plan keeps Monthly accounting observations and requests native
Hourly observations for component charts. A purpose can reuse a matching
request from another purpose. Existing requests retain their frequency.

Monthly energy is an interval total, not a value for the last hour of a month.
A Run Period total remains a run total even if its SQL row is attached to the
final month's time record.

Output availability can vary by engine version and model type. A requested
variable that EnergyPlus cannot report is missing evidence, not zero.
Post-run variable and meter dictionaries help identify available names and
aliases; their existence does not establish measured values.

## Result sources and partial results {#result-sources}

The reading preference is SQL, then CSV, then ESO.

| Source | Useful information | Boundary |
| --- | --- | --- |
| SQL | Dictionary keys, units, frequencies, time metadata, tabular reports. | A dictionary entry without valid observations proves no value. |
| CSV | Reported columns and timestamp labels. | Some metadata is less explicit than SQL. |
| ESO | Retained fallback heat-balance observations. | Availability depends on the requested outputs. |
| ERR | Engine warnings, Severe/Fatal issues, and completion evidence. | Messages explain the run; they are not energy measurements. |

Sections can have different available sources. A failed or absent Heat Flow
section does not erase successfully read Series, and conversely. Inspect the
source and completeness of the section you are using.

A fallback can restore an unavailable section, but it does not authorize
inventing outputs. A file named eplusout.sql alone proves neither a complete
run nor a complete energy explanation.

## Calendar, warmup, and sampling {#calendar-and-sampling}

Use the actual observation calendar. Do not assume every run has exactly
8,760 hours: leap years, partial periods, and model settings can change it.
EnergyPlus interval-end labels can include 24:00; read the complete label
rather than treating it as an ordinary hour-of-day index.

For Hourly HVAC and Comfort plots, SQL weather-run metadata is used when
available to exclude sizing days and warmup. Older SQL schemas and CSV retain
their available axes. Missing metadata is not permission to manufacture a
weather calendar.

Some aggregate SQL rows have a NULL warmup flag. NULL alone does not make a
valid Monthly or Run Period observation a warmup row. Interpretation also
depends on reporting frequency and environment metadata.

Generic Series are a preview: SQL is limited to 256 columns, CSV to 16, and
long previews can be sampled. Requested HVAC/Comfort observations and Energy
Path source traces are read separately beyond those preview limits.
A small preview does not prove the original run has few observations.

Heat Flow can also report sampled displayed frames and the original frame
count. A playback frame is not necessarily every native reporting interval.
Preserve this distinction when interpreting peaks or exporting trace data.

## Energy, rates, and units {#energy-and-rates}

Energy and power answer different questions.

| Quantity | Meaning | Common display unit |
| --- | --- | --- |
| Energy | Amount over an interval. | kWh |
| Power or heat-transfer rate | Rate at an observation interval. | W or kW |
| Temperature | State at the reported observation. | °C |
| Mass flow | Mass passing per second. | kg/s |
| Humidity ratio | Water vapour per mass of dry air. | g/kg |
| Relative humidity | Relative saturation measure. | % |

Reported energy takes precedence when the same target also has a rate
fallback. The app does not add energy and its integrated rate as two consumers.

```text
Energy [kWh] = reported joules / 3,600,000
Energy from a rate [kWh] = sum(rate [W] × interval [hours]) / 1,000
```

For example, 2,000 W over 15 minutes contributes 0.5 kWh.
Summing four such W readings gives 8,000 W, not 2 kWh; interval integration is
required. Missing intervals cannot be treated as observed operation at zero.

The Energy Path display divides thermal and site energy by executed-model
floor area. See [Intensity](./energy-path.en.md#floor-area-and-intensity).
HVAC rate and Comfort state plots keep their own units.

## Missing, zero, and unavailable {#missing-data}

| Display/evidence | What it means |
| --- | --- |
| Measured zero | A valid report contains zero. |
| Missing or unavailable | The necessary observation or denominator is absent or invalid. |
| Not requested | This output group was outside the run's selected plan. |
| Not applicable | The model/plan has no applicable observation group. |
| Ambiguous | Multiple identities or conflicting observations prevent a unique value. |

A flat-looking chart can contain rounded small values. Conversely, no line or
an unavailable chart is not a zero-energy run. Missing observations should not
be replaced with nearby times, another zone, or the Building value.

## Heat-Flow Ledger {#heat-flow-ledger}

Select a zone number in the compact floor plans to inspect the current timestamp
in the right-hand ledger. Zone numbers are unique across the model. A zone with
several floor pieces, such as a plenum, has one number and one local stack.
The plans fit their cards automatically and stay fixed; wheel, drag and
double-click do not change their scale or position.
Expand **Zone values** below a plan for each zone's local gains, local losses
and total Net; the tables and reading guide start closed to keep the plans clear.
The ledger displays signed rates in kW to 0.001 kW and temperature in °C.
Hover a rate to read the W value retained in the result. Positive transfer adds
heat to zone air; negative transfer removes it. Gains and losses remain visible
separately, even when their Net is small.
The local bars and Net colours use fixed scales across all retained frames, so
their sizes and colours remain comparable during playback. The floor-plan colour
legend shows the selected Net or temperature range.
These zone-air rates describe one modelled zone; Zone/ZoneGroup multipliers
are not applied to the reported ledger.

| Group | Included transfer terms | Meaning |
| --- | --- | --- |
| Local gains / losses | Internal convective gains | Heat from people, lights and equipment to zone air. |
| Local gains / losses | HVAC system air transfer | Heat delivered or removed through HVAC supply air. |
| Local gains / losses | HVAC/system convective gains | Direct convective heat from non-air HVAC equipment. |
| Boundary exchange | Surface convection | Combined transfer between all zone surfaces and zone air. |
| Boundary exchange | Interzone air transfer | Combined transfer caused by air exchange with other zones. |
| Boundary exchange | Outdoor air transfer | Combined transfer from outside air, including infiltration. |

```text
Net = sum of the six transfer terms
Balance residual = Net − air energy storage
```

**Air energy storage** and **reported balance deviation** are shown separately
from transfers. Positive storage means the air stores heat as it warms;
negative storage means it releases heat as it cools. They are not extra gains
or losses to add to Net. The computed residual is available only when all six
transfer terms and storage are observed. The independently reported deviation
keeps its reported sign. This follows the
[EnergyPlus zone-air balance](https://github.com/NREL/EnergyPlus/blob/v25.1.0/src/EnergyPlus/ZoneTempPredictorCorrector.cc#L4981).

A `*` marks a partial Net or local subtotal: it sums the available terms without treating missing
terms as zero. An unavailable number or chart gap differs from a measured zero.
New results record availability for each zone/category/time and temperature.
Older saved results retain their original value arrays, whose padded zeros may
not distinguish absent observations; rerunning provides that evidence.

**Exchange arrows** point into or out of the selected zone. The air-transfer and
surface-convection rows show aggregate zone rates in kW; they do not identify a
particular neighbouring zone. The separate measured surface arrows identify a
zone or external boundary from the executed model. They require verified Hourly
source observations, an exact timestamp match and explicit supported rate or
energy units. Rate-based energy needs a verified one-hour integration interval.
Reported Hourly J/kWh energy can remain usable at aligned observed timestamps
even when other hours are absent. Reciprocal interzone surfaces represent one
interface and are counted once, while their source identities
remain available. Incoming and outgoing interval energies are shown separately
in **kWh per reported interval**. Surface conduction and window exchange differ
from the zone-air convection ledger and must not be added to its kW total.
Missing or unverified pairs have no measured arrow. Older results need a new
Surface-detail run to provide the executed geometry and frame evidence.

The floor grid draws one net surface arrow per visible zone pair, directly
between their number badges, including pairs on different visible levels.
All observed outdoor orientations combine into one **Outside** arrow; Ground
and Adiabatic boundaries are not outdoor exchange. Hidden zones are not replaced
with external labels. Direction follows incoming minus outgoing interval energy;
balanced nonzero exchange uses one grey two-headed arrow. Expand **Measured
surface exchange** for the original separate incoming/outgoing amounts. The
combined arrow totals only verified observations, never unreported boundaries.

**Heat-flow history** uses the full result width for three panels: local
transfers, boundary exchange, and reported storage/balance deviation. The panels
share a signed scale; the diagnostics retain their reported signs. Axis units
are labelled. Fonts follow the UI and **Settings → Graph label font size**, with
a readable minimum and horizontal scrolling on narrow panels. Click a
chart to select a frame; scroll to zoom, Shift-scroll to pan, or double-click
to restore the complete range. Legends retain the selected frame's values.
Increasing valid timestamps use elapsed-time spacing. Backward, duplicate or
unavailable dates use an explicitly labelled recorded-frame sequence instead.
Sparse observations do not fill the unobserved intervals.
**24 hours**, **7 days** and **30 days** select windows around
the selected timestamp. They stay within one continuous calendar sequence;
unusable dates or a backward date boundary do not fall back to frame counts.

The history renders every supplied frame without additional stride sampling
or averaging. Long runs are already sampled by the backend around its 720-frame
limit; the retained last frame can affect that count. Check shown/original
frame counts and timestamps. Twenty-four displayed frames need not mean one
day, and adding sampled W readings cannot produce annual kWh. Use actual
timestamp ranges and the integrated energy results for those quantities.

Static Topology UA is conductance in W/K. It has no weather, operating time,
or temperature difference. For illustration, UA = 100 W/K and a steady
10 K difference imply 1,000 W in a simple steady calculation, but that is
not the model's simulated hourly surface flow. Storage, solar gains,
schedules, and changing boundary conditions remain relevant.

See [Topology](./topology.en.md) for static connectivity and construction
coverage. Simulation results do not become a static Topology metric.

## HVAC frame inspection {#hvac-frames}

Select an available executed loop and move through its observation frames.
The diagram retains equipment and node topology from that run, even if the
current editor model has changed.

Node annotations can include mass flow, temperature, humidity, and defined
setpoints. Equipment properties can include power, load, operating fraction,
energy, or reported COP where those outputs exist.

- An available zero flow is a measurement.
- An absent reading stays unavailable; the previous frame is not reused.
- The EnergyPlus unset temperature setpoint (-999, including tiny roundoff)
  is excluded from setpoint display and deviation calculations.
- A valid zero or negative temperature setpoint is not an unset flag.
- A missing setpoint alone does not establish a control fault.
- Older saved results without executed topology can retain observations
  while their loop diagram remains unavailable.

Narrow schematic panes can scroll horizontally. Dense measurement labels add
space around the loop rather than changing the physical connection meaning.
The schematic annotates only its actual circuit nodes and equipment. A typed
child can annotate its uniquely identified parent equipment. Observed readings
without a drawn circuit position remain available in the graph controls.
Ports of another circuit are not annotated, and detached labels are omitted.

## HVAC time-series and scatter {#hvac-charts}

The basic charts show node flow, temperature, and humidity. Use the per-node
controls to compare selected traces. Y ranges and Auto reset apply independently
to different units. HVAC and Comfort plots use the app font and the graph
label size in Settings. Larger labels keep a readable chart width; narrow
panes scroll horizontally.

Custom charts choose an equipment/node first, then a property. A line chart
can use at most two units on separate axes. Scatter uses exactly two
properties at matching observation times.

Primary observations align by source row identity. A secondary file can join
by a timestamp label only when that label is unique in both files. Duplicate
or ambiguous observations become gaps rather than an arbitrary match.

For example, a temperature measured at 12:00 cannot be paired with a flow
measured only at 12:15 to establish a scatter point. An empty intersection
means unavailable paired evidence, not proof of no relationship.

Humidity uses reported relative humidity where available; otherwise it can
use humidity ratio in g/kg. These units are different quantities. A shared
visual range does not convert one into the other.

## Comfort interpretation {#comfort}

Comfort is an observation view, not an energy-accounting view.

| Trace/indicator | Interpretation |
| --- | --- |
| T | Zone air temperature. |
| Top | Reported operative temperature, when available. |
| Tmrt | Reported mean radiant temperature. |
| Tset,h / Tset,c | Heating/cooling temperature setpoints. |
| PMV | Predicted Mean Vote from the reported comfort model. |
| PPD | Reported Predicted Percentage Dissatisfied, in %. |
| RH / w | Relative humidity / humidity ratio. |
| Unmet time | Reported setpoint or comfort unmet duration, in hours. |

PMV/PPD can be keyed to People objects. Those keys are mapped to their actual
zones; different People groups remain separate. Their assumptions depend on
the input comfort model, activity, clothing, schedules, and environmental
conditions. The app does not infer an unreported People-group comfort result.

Indicators use available observation averages/ranges or reported unmet hours.
A time-series average is an average of available observations, not necessarily
an occupancy-weighted exposure measure.

Building indicators require actual facility observations. Zone PMV is not
averaged into a Building PMV, and zone unmet hours are not summed into an
invented facility total. Concurrent unmet hours in different zones can overlap.

For a custom period used by automation, full-run tabular unmet hours are
omitted because they do not describe that selected interval. The main view
uses the full run. A zone with tabular hours but no trend can still have an
indicator; it does not acquire a fabricated hourly chart.

## Execution, receiving, and errors {#run-status}

The stages distinguish engine execution from reading, result construction,
receiving, and display. Progress shows actual work when its total is known.
SQL reads report processed observations without rescanning the database just
to count rows. An animated bar with no percentage means the total is unknown;
elapsed time and the current activity still update.

After a successful matching run in this app session, the app can show an
approximate `≈` percentage and a remaining-time range. The estimate uses the
same input, purpose/detail/scope, engine, weather and execution configuration.
One measurement produces a wide initial range; later matching measurements
refine it. It estimates backend processing, including reading and building
results, and does not promise a response-transfer or display deadline.
Changed inputs begin without an estimate. If a run exceeds the measured range,
remaining time returns to unknown instead of showing zero while work continues.

Engine Warmup, sizing and simulated dates identify actual engine activity.
A simulated date is not a wall-time percentage: design days, multiple run
periods and Warmup can have very different costs. Receiving progress uses actual
bytes when the response size is known; result decoding/display is a separate
step. Overall completion reaches 100% after the result is installed for display;
an individual step can finish earlier and is explicitly labeled as that step.

Timing measurements are bounded and kept only in memory. They create no
additional run-history files and reset when the app restarts. Batch progress
shows completed files, active/queued workers and an active file's current stage;
different file sizes and concurrent workloads prevent a reliable file-count ETA.

Engine completion does not mean every result is already displayed. A large
run can take additional time to read SQL, build accounting, transfer data,
and prepare graphs. A receiving or display problem must be investigated
separately from an engine error.

Review ERR severity and the completion state together. Warnings can identify
unavailable requests or modeling concerns. Severe/Fatal issues and incomplete
execution require attention before using quantities as a complete experiment.
A result section can still be partial; its presence is not a clean-run verdict.

## Saved results, transport, and exports {#saved-results}

Saved results are bound to the executed model, not the current editor.
Restoring or exporting a captured result does not rerun EnergyPlus.

Large desktop responses can share identical timelines and numeric columns.
This preserves observation values and order; it is not chart downsampling.
Unopened HVAC loops can remain compact until inspected.

Energy Path **Data details → Data → Export** offers HTML, XLSX, and full-run
JSON. Summaries retain the selected Scope/Period; full-run JSON retains the
captured run. Optional XLSX trace sheets retain source references, units,
link endpoint quantities, and the original submitted data.

Retain the executed input, run plan, ERR, SQL or fallback files, and weather
identity when keeping evidence for a comparison. Exported totals alone
cannot explain which outputs, units, or assumptions produced them.

## Troubleshooting and batch work {#troubleshooting}

| Symptom | First checks |
| --- | --- |
| Run unavailable | Compatible engine registration, detected input version, weather requirements. |
| Engine completed but view is still busy | Current reading/building/receiving stage; do not assume the engine is still executing. |
| Energy component lacks Hourly chart | Actual Hourly request, exact key/frequency, complete calendar, usable floor area. |
| HVAC property is absent | Whether the executed run reported it and whether values align with frames. |
| Comfort Building is empty | Actual facility observations; zone data is not a substitute. |
| Comparison differs unexpectedly | Executed inputs, engine, weather, periods, frequencies, allocation, and missing versus zero. |

Batch workers run independent inputs with bounded concurrency. Each file
needs its own compatible engine and weather mapping; one incompatible input
does not make compatible files fail. Worker count is a throughput choice,
not a change to physical quantities. See [Batch workflows](./tools.en.md).
