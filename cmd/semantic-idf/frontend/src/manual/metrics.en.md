# Metrics and operational Profile

Metrics summarizes the input model without running EnergyPlus. Profile resolves
design loads and schedules into comparable Zone profiles. These reports answer
what the model declares and how supported inputs are scheduled; simulation
answers what the engine predicts under weather, controls and system dynamics.

## Read a metric correctly {#reading-metrics}

Read the value together with its unit, status and source. The metric's identifier
is more stable than its translated display name. The [catalog](#metric-catalog)
provides the source fields, calculation method, assumptions and missing-data
behavior of every metric available in this build.

| Status | Meaning | Appropriate use |
| --- | --- | --- |
| `ok` | The analyzer resolved the inputs required by that metric's method | Compare after checking its scope and assumptions |
| `partial` | A usable result contains incomplete contributions or an estimate/fallback | Investigate the unresolved inputs before using it as a complete total |
| `missing` / — | No defensible value is available for the selected method | Find the missing source/basis; do not substitute zero |

Inventory counts can legitimately be zero. A computed zero from available
inputs differs from absent data. Rounding can also display a small nonzero
quantity as zero. Confidence, badges and evidence describe the analyzer's
derivation; they are not probabilistic error bars or engine validation.

The six metric families are Model & Inventory, Geometry & Areas, Envelope &
Fenestration, Internal Loads, Schedules & Operation, and HVAC & Conditioning.
Some readiness metrics require heavier analysis and may arrive after the first
quick Metrics report. A pending readiness value should be allowed to refresh.

## Inventory, coverage and conditioning {#inventory-coverage}

Object count includes parsed objects; object-type count counts distinct parsed
type names. Zone and Space are counted separately. Schedule inventory counts
`Schedule:*` objects, excluding `ScheduleTypeLimits`. Construction variants and
opaque/window material families have their own counts. These counts describe
the input, not instantiated equipment or successful simulation results.

Geometry coverage compares usable detailed vertices with geometry-bearing
objects. A supported simple rectangular object can have a calculated area yet
remain uncovered by this detailed-polygon readiness check. Low coverage means
less available geometric verification, not automatically invalid geometry.

Profile coverage measures evaluable *referenced* schedules; unreferenced
schedules are excluded from that readiness ratio. Output readiness checks
recognized request groups. Neither percentage guarantees a successful engine
run or complete Energy Path observations.

Conditioned-zone detection uses references from equipment connections,
ZoneHVAC objects, thermostat controls and SpaceHVAC objects mapped to their
parent Zones. ZoneLists are expanded when resolved. The total deduplicates
Zones; evidence buckets can overlap. Adding their bucket counts can therefore
exceed the conditioned-zone total.

A thermostat is evidence of modeled conditioning, not proof that heating or
cooling capacity is adequate. HVAC object count is deliberately broad and
includes control/topology objects. Node-connection and rule-edge counts describe
resolved graph relationships; they do not count energy flows or solver steps.

## Floor area, volume and multiplier basis {#area-volume}

The analyzer prefers declared numeric Zone floor area where available and
otherwise resolves supported floor geometry. Spaces supply their own target
bases and can support Zone fallback calculations. Check the metric evidence
when declared dimensions and computed geometry disagree.

Gross floor area aggregates the relevant Zone areas. Conditioned floor area
uses only identified conditioned Zones with known areas. Unconditioned area is
the nonnegative difference between gross and conditioned floor area and needs
both quantities. A Zone's **Part of Total Floor Area** setting affects the
building floor-area basis; it should be checked when a total surprises you.

When a numeric Zone area and explicit Space areas both exist, the analyzer
reconciles Space areas proportionally to the declared Zone total. The calculated
basis includes explicit Space areas plus any implicit Zone remainder from floor
surfaces; each explicit Space receives the same reconciliation ratio. For
example, a declared 500 m² Zone containing calculated 100+300 m² Spaces scales
those areas to 125+375 m². Per-area load resolution then uses the reconciled
targets rather than adding another 500 m² on top.

Extensive Metrics quantities incorporate the applicable Zone multiplier and
ZoneGroup's ZoneList multiplier. Apply each factor once. A representative 100 m²
Zone with Zone multiplier 2 and ZoneGroup multiplier 3 contributes 600 m² to
a multiplier-adjusted total. Its polygon still represents 100 m².

Declared numeric Zone Volume has priority. Otherwise, Zone floor area × resolved
height supplies an estimate; resolved Space volumes are a fallback when Zone
dimensions are unavailable. A declared Space volume does not override an
independently known parent Zone volume. Missing or estimated contributions can
produce a partial total. Multipliers must not be applied again to a basis that
already includes them.

Average floor height gives each resolvable Zone one contribution; it is not a
volume-weighted building height. A geometry-derived height is the Zone's
vertical extent, which can differ from a declared ceiling height in sloped or
irregular spaces.

Footprint area sums ground-contact floors, or lowest horizontal floors when
the ground basis is absent. Bounding-box area is XY width × depth across
available detailed vertices. It can include courtyard voids and gaps between
separate wings. Long/short sides and aspect ratio are bounding-box measures,
not a fitted architectural perimeter. A square bounding box has aspect ratio 1.

Envelope/volume and floor-area/volume ratios have unit **1/m**. They indicate
compactness under the resolved area/volume basis, rather than heat-loss rates.

## Envelope area, WWR and orientation {#envelope-wwr}

Gross exterior wall area includes its openings. Gross envelope area combines
the supported exterior walls, roofs and ground-contact floors. Net opaque
envelope subtracts recognized window and opaque-door area; incomplete opening
geometry affects this balance.

```text
WWR [%] = window area / gross exterior wall area × 100
net opaque area = gross envelope area - recognized window and door area
```

Window area includes recognized glass doors; opaque doors have a separate
metric. Skylights are identified through roof base surfaces and compared with
roof area. Read the exact catalog scope before interpreting total WWR as an
exterior-only architectural audit: the numerator follows the analyzer's
recognized window family, while its denominator is exterior wall area.

Example: 200 m² gross wall with 40 m² of applicable windows gives WWR 20%,
not 25% obtained by dividing by 160 m² opaque area. Apply consistent multipliers
to numerator and denominator. A missing/zero wall denominator has no valid WWR;
it is not 0%.

Orientation bins use resolved azimuth after applicable building/Zone rotations:
North [315°,360°) and [0°,45°); East [45°,135°); South [135°,225°); West
[225°,315°). A 45° wall belongs to East. World-coordinate geometry is handled
according to its coordinate declaration; rotations should not be manually
applied again. A window inherits relevant orientation from its base surface.

## Construction U, UA and thermal mass {#construction-thermal}

Construction quantities appear in Topology inspectors rather than as a second
building-wide Metrics U-value catalog. They are static properties derived from
the model's supported material fields.

```text
layer R [m²·K/W] = thickness [m] / conductivity [W/(m·K)]
construction R = sum(resolved layer R)
construction U [W/(m²·K)] = 1 / construction R
areal heat capacity [J/(m²·K)] = sum(thickness × density × specific heat)
UA [W/K] = opaque area × opaque U + sum(opening area × opening U)
```

A direct thermal-resistance field can supply layer R; supported U-factor fields
supply 1/U. The layer sum does not independently add generic indoor/outdoor
surface-film resistances. Simple glazing and specialized C/F-factor constructions
follow their supported direct inputs; an F-factor equivalent uses F × exposed
perimeter / area. This estimate is not a full window assembly or transient
EnergyPlus heat-transfer calculation.

Inspect missing layer fields and construction coverage rather than assuming a
displayed estimate includes every physical effect. SHGC/solar transmittance,
thermal storage, temperature differences and time-varying films are not all
contained in a static U or UA. A 10 m² wall with U=0.4 has UA=4 W/K; multiplying
by an assumed 20 K yields 80 W under that assumption, not observed annual kWh.

See [Topology quantities](./topology.en.md#topology-quantities) for physical versus
multiplier-adjusted totals and reciprocal-interface counting.

## Internal design loads and normalization {#internal-loads}

People supports direct count, People/Area and Area/Person. Lights supports
direct LightingLevel, Watts/Area and Watts/Person. Equipment combines supported
ElectricEquipment, GasEquipment, HotWaterEquipment, SteamEquipment and
OtherEquipment methods: direct level, power per area or power per person.
Specialized gains outside those families are not silently included.

References to Zone/ZoneList/Space/SpaceList must resolve with the relevant
floor-area and people basis. A direct level can be instantiated per target;
an area-based method uses the area of the addressed targets. Missing people
or area can leave a method unresolved even when its numeric coefficient exists.
Method coverage reports resolved/total supported objects and unresolved methods.

```text
people = floor area / area per person
power = watts per area × addressed floor area
power = watts per person × resolved design people
building average density = aggregate design power / selected floor-area basis
people per 100 m² = aggregate people / selected floor area × 100
```

Building lighting/equipment density and people density prefer conditioned floor
area, falling back to gross floor area. This denominator can differ from the
individual object's target area. Partial power or area yields a partial density.
A weighted aggregate is not the arithmetic average of Zone densities.

Example: 100 m² at 10 W/m² and 300 m² at 20 W/m² contain 7000 W total.
The combined density is 17.5 W/m², not the unweighted average 15 W/m². Keep
representative-instance and multiplier-adjusted bases consistent.

These values are design magnitudes. Schedules alter operation; fractions for
radiant/latent/convective gains alter thermal response; HVAC and weather alter
energy demand. Design lighting power is not cooling load, and equipment power
is not automatically total site electricity.

## Schedules and operating hours {#schedule-hours}

The static annual evaluator supports Schedule:Constant and supported
Schedule:Compact rules. Other families remain visible as unsupported rather
than being assigned invented annual operation. Referenced-schedule count is
based on recognized schedule-reference fields and normalized names.

Annual summaries use a fixed Monday-start, non-leap 8760-hour calendar. They do
not reproduce the selected EPW calendar, leap years, engine special-day handling
or every subhourly control effect. Check compact-rule interpretation and any
fallback warning before using a schedule as a detailed operating-calendar audit.

| Quantity | Calculation |
| --- | --- |
| Active/operating hours | Number of represented hours with value > 0 |
| Above-half hours | Number of represented hours with value ≥ 0.5 |
| Equivalent full hours | Sum of hourly schedule values |
| Average | Hourly-value sum / 8760 |
| P95 | Nearest-rank 95th percentile of hourly values |
| Representative model operating hours | Maximum among referenced supported schedules, or all supported schedules if none apply |

A constant fractional schedule of 0.5 has 8760 active hours and 4380 equivalent
full hours. Those quantities answer different questions. Equivalent full hours
has its usual full-load interpretation for fractional multipliers; a temperature
or other dimensional schedule should not be interpreted as load-hours.

The model operating-hours metric is a representative maximum, not the sum of
all schedules and not a resolved building occupancy calendar.

## Profile workflow and comparison basis {#profile-workflow}

Open **Profile** and choose **Profiles** to inspect grouped Zones or **Zones**
to inspect individual Zones. Enable the dimensions relevant to the question:
Occupancy, Lighting, Equipment, Infiltration, Ventilation and Outdoor Air.
Select the rows to compare, then inspect their graphs and fidelity indicators.

Settings select display/grouping metrics, numeric tolerance, schedule comparison,
time view, scaling and apply behavior. The default comparison uses normalized
people/power/airflow metrics, a numeric tolerance of 0.001 and schedule names.
Grouping quantizes each value by `round(value / tolerance) × tolerance`; it is
not a guarantee that every pair of values within the tolerance has one group.

Grouping also includes the actual metric basis, preferred/fallback identity,
status, resolved-item coverage and schedule contribution signature. Two equal
numbers with different units or unresolved contributions are not equivalent.
Schedule comparison by resolved content can identify differently named matching
patterns; comparison by name preserves their declared identities; ignoring
schedules groups design magnitudes without asserting identical operation.

Profile uses representative Zone/Space design bases for comparison. It should
not be added across groups as though every row were a multiplier-adjusted
building total. A group represents comparable members; the graph can consolidate
identical series and identify their member Zones rather than multiply a density
by the number of Zones.

## Profile algorithms and graph meanings {#profile-algorithms}

Load dimensions normalize resolved count, power and airflow to the selected
Zone basis. ACH is `flow [m³/s] × 3600 / volume [m³]`. Per-person flow needs a
resolved design-person count; per-area flow needs the corresponding addressed
area. Leakage area, flow coefficient or opening area can remain the only
defensible source quantity for weather-dependent models.

For several contributing objects, their own schedules are applied to additive
quantities first: people counts, watts or m³/s. The combined series is then
normalized. It does not simply sum each Space's W/m² or borrow the first
object's schedule.

Example: a 100 m² Space has 1000 W lighting at schedule 1 and a 300 m² Space has
6000 W at schedule 0.5. Zone power for that hour is 4000 W; density over
400 m² is 10 W/m². Adding Space densities 10+10 W/m² would be incorrect.
Area/person is an inverse metric and follows scheduled people before taking
the inverse; an unoccupied hour does not establish a finite area/person ratio.

The graph offers representative Day, Week, Month, Year heatmap and Duration
views. Day comprises weekday/Saturday/Sunday profiles; Week has 168 hourly
positions; Month shows monthly averages; the Year heatmap has 365 columns and
24 hour rows; Duration sorts the 8760 values from highest to lowest and discards
chronological order. A duration curve cannot tell you when a peak occurred.

Auto, design-peak, percentile and other selected scale settings change visual
scaling. P95 scaling can de-emphasize rare peaks. Compare units and axes before
comparing colors or line heights. Coincident lines can represent several real
series; their overlap/member presentation avoids implying that one vanished.

Nominal airflow is especially important. Infiltration with weather coefficients,
effective leakage area, flow coefficient and wind/stack opening models depends
on pressure, wind or temperature. DesignSpecification:OutdoorAir can depend on
occupancy and controls. A nominal/scheduled design profile is not simulated
airflow. Unsupported or unresolved schedules can use a disclosed fallback;
their smooth graph is not proof of high fidelity.

Profile hints/outliers flag comparison and operation patterns, not failures of
EnergyPlus convergence or guaranteed design mistakes. Follow source objects
and warnings before applying a change.

## Apply a Profile and export Metrics {#apply-export}

Profile **Apply** starts an explicit model-edit workflow. Choose source and
target Zones/dimensions, then preview changes. Clone mode copies supported
objects for targets; shared mode uses supported shared targets. ZoneList creation
or modification follows the selected settings. Replace/Keep/Duplicate controls
how existing target objects are handled. A duplicate can add an additional load;
it is not merely a second visual reference.

Changing an apply setting or edit invalidates the previous preview. Review the
fresh preview, skipped/unsupported objects and warnings before applying. The
working document changes and analysis reruns; save separately to persist it.
Cloning a Profile does not copy geometric Zones or prove that target HVAC can
serve the resulting loads.

Metrics **Export JSON** preserves categories, metric IDs, values, units and
status/source metadata. **Export CSV** is a two-column `name,value` table;
variable identifiers and units are included in names, with `[-]` for unitless
values. CSV values follow displayed precision. Use JSON when numeric types and
missing/partial status must remain machine-readable. Input filters and visible
Table limits do not define the exported Metrics scope.

For several models, [Batch Metrics](./tools.en.md#batch-metrics) provides comparisons.
Compare status, basis and coverage together with any numerical difference.

## Metric catalog {#metric-catalog}

The catalog below is supplied from the same definitions used by Metrics and
exports. Search by identifier, name, category or source when tracing a value.
Each entry states its input source, method, assumptions and missing-data rule.
Use these entries as the build's scalar reference; the preceding sections explain
how the individual calculations interact and where broader interpretation fails.
