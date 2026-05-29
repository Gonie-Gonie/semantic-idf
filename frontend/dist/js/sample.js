export const sampleIDF = `Version,
  24.1;                    !- Version Identifier

SimulationControl,
  Yes,                     !- Do Zone Sizing Calculation
  No,                      !- Do System Sizing Calculation
  No,                      !- Do Plant Sizing Calculation
  No,                      !- Run Simulation for Sizing Periods
  Yes;                     !- Run Simulation for Weather File Run Periods

Building,
  Analyzer Sample Building,!- Name
  0,                       !- North Axis
  Suburbs,                 !- Terrain
  0.04,                    !- Loads Convergence Tolerance Value
  0.4,                     !- Temperature Convergence Tolerance Value
  FullExterior,            !- Solar Distribution
  25,                      !- Maximum Number of Warmup Days
  6;                       !- Minimum Number of Warmup Days

Timestep,
  4;                       !- Number of Timesteps per Hour

GlobalGeometryRules,
  UpperLeftCorner,         !- Starting Vertex Position
  CounterClockWise,        !- Vertex Entry Direction
  World;                   !- Coordinate System

ScheduleTypeLimits,
  Fraction,                !- Name
  0,                       !- Lower Limit Value
  1,                       !- Upper Limit Value
  Continuous;              !- Numeric Type

ScheduleTypeLimits,
  Temperature,             !- Name
  -60,                     !- Lower Limit Value
  200,                     !- Upper Limit Value
  Continuous,              !- Numeric Type
  Temperature;             !- Unit Type

Schedule:Compact,
  AlwaysOn,                !- Name
  Fraction,                !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: AllDays,            !- Field 2
  Until: 24:00,            !- Field 3
  1;                       !- Field 4

Schedule:Compact,
  OfficeOccupancy,         !- Name
  Fraction,                !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: Weekdays,           !- Field 2
  Until: 07:00,            !- Field 3
  0.05,                    !- Field 4
  Until: 18:00,            !- Field 5
  1.0,                     !- Field 6
  Until: 24:00,            !- Field 7
  0.1,                     !- Field 8
  For: Weekends,           !- Field 9
  Until: 24:00,            !- Field 10
  0.15;                    !- Field 11

Schedule:Compact,
  WorkdayLighting,         !- Name
  Fraction,                !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: Weekdays,           !- Field 2
  Until: 06:00,            !- Field 3
  0.05,                    !- Field 4
  Until: 19:00,            !- Field 5
  0.9,                     !- Field 6
  Until: 24:00,            !- Field 7
  0.2,                     !- Field 8
  For: Weekends,           !- Field 9
  Until: 24:00,            !- Field 10
  0.1;                     !- Field 11

Schedule:Compact,
  EquipmentSchedule,       !- Name
  Fraction,                !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: AllDays,            !- Field 2
  Until: 08:00,            !- Field 3
  0.2,                     !- Field 4
  Until: 18:00,            !- Field 5
  0.8,                     !- Field 6
  Until: 24:00,            !- Field 7
  0.3;                     !- Field 8

Schedule:Compact,
  HeatingSetpoint,         !- Name
  Temperature,             !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: AllDays,            !- Field 2
  Until: 06:00,            !- Field 3
  18,                      !- Field 4
  Until: 22:00,            !- Field 5
  21,                      !- Field 6
  Until: 24:00,            !- Field 7
  18;                      !- Field 8

Schedule:Compact,
  CoolingSetpoint,         !- Name
  Temperature,             !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: AllDays,            !- Field 2
  Until: 06:00,            !- Field 3
  28,                      !- Field 4
  Until: 22:00,            !- Field 5
  24,                      !- Field 6
  Until: 24:00,            !- Field 7
  28;                      !- Field 8

Schedule:Compact,
  UnusedNightPurge,        !- Name
  Fraction,                !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: AllDays,            !- Field 2
  Until: 24:00,            !- Field 3
  0;                       !- Field 4

Zone,
  Core Office,             !- Name
  0,                       !- Direction of Relative North
  0,                       !- X Origin
  0,                       !- Y Origin
  0,                       !- Z Origin
  1,                       !- Type
  1,                       !- Multiplier
  autocalculate,           !- Ceiling Height
  autocalculate;           !- Volume

Zone,
  Perimeter Office,        !- Name
  0,                       !- Direction of Relative North
  12,                      !- X Origin
  0,                       !- Y Origin
  0,                       !- Z Origin
  1,                       !- Type
  1,                       !- Multiplier
  autocalculate,           !- Ceiling Height
  autocalculate;           !- Volume

Zone,
  Meeting Room,            !- Name
  0,                       !- Direction of Relative North
  0,                       !- X Origin
  8,                       !- Y Origin
  0,                       !- Z Origin
  1,                       !- Type
  1,                       !- Multiplier
  autocalculate,           !- Ceiling Height
  autocalculate;           !- Volume

BuildingSurface:Detailed,
  Core Floor,              !- Name
  Floor,                   !- Surface Type
  Generic Floor,           !- Construction Name
  Core Office,             !- Zone Name
  Ground,                  !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  NoSun,                   !- Sun Exposure
  NoWind,                  !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0,                       !- Vertex 1 X-coordinate
  0,                       !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  12,                      !- Vertex 2 X-coordinate
  0,                       !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  12,                      !- Vertex 3 X-coordinate
  8,                       !- Vertex 3 Y-coordinate
  0,                       !- Vertex 3 Z-coordinate
  0,                       !- Vertex 4 X-coordinate
  8,                       !- Vertex 4 Y-coordinate
  0;                       !- Vertex 4 Z-coordinate

BuildingSurface:Detailed,
  Perimeter Wall,          !- Name
  Wall,                    !- Surface Type
  Generic Wall,            !- Construction Name
  Perimeter Office,        !- Zone Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0,                       !- Vertex 1 X-coordinate
  0,                       !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  10,                      !- Vertex 2 X-coordinate
  0,                       !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  10,                      !- Vertex 3 X-coordinate
  0,                       !- Vertex 3 Y-coordinate
  3,                       !- Vertex 3 Z-coordinate
  0,                       !- Vertex 4 X-coordinate
  0,                       !- Vertex 4 Y-coordinate
  3;                       !- Vertex 4 Z-coordinate

People,
  Core People,             !- Name
  Core Office,             !- Zone or ZoneList Name
  OfficeOccupancy,         !- Number of People Schedule Name
  People,                  !- Number of People Calculation Method
  18;                      !- Number of People

People,
  Meeting People,          !- Name
  Meeting Room,            !- Zone or ZoneList Name
  OfficeOccupancy,         !- Number of People Schedule Name
  People,                  !- Number of People Calculation Method
  8;                       !- Number of People

Lights,
  Core Lights,             !- Name
  Core Office,             !- Zone or ZoneList Name
  WorkdayLighting,         !- Schedule Name
  LightingLevel,           !- Design Level Calculation Method
  900;                     !- Lighting Level

Lights,
  Meeting Lights,          !- Name
  Meeting Room,            !- Zone or ZoneList Name
  WorkdayLighting,         !- Schedule Name
  LightingLevel,           !- Design Level Calculation Method
  450;                     !- Lighting Level

ElectricEquipment,
  Core Plug Loads,         !- Name
  Core Office,             !- Zone or ZoneList Name
  EquipmentSchedule,       !- Schedule Name
  EquipmentLevel,          !- Design Level Calculation Method
  1200;                    !- Design Level

ThermostatSetpoint:DualSetpoint,
  Office Dual Setpoints,   !- Name
  HeatingSetpoint,         !- Heating Setpoint Temperature Schedule Name
  CoolingSetpoint;         !- Cooling Setpoint Temperature Schedule Name

ZoneControl:Thermostat,
  Core Office Thermostat,  !- Name
  Core Office,             !- Zone or ZoneList Name
  AlwaysOn,                !- Control Type Schedule Name
  ThermostatSetpoint:DualSetpoint, !- Control 1 Object Type
  Office Dual Setpoints;   !- Control 1 Name

ZoneHVAC:IdealLoadsAirSystem,
  Core Ideal Loads,        !- Name
  AlwaysOn,                !- Availability Schedule Name
  Core Office Inlet Node,  !- Zone Supply Air Node Name
  Core Office Exhaust Node,!- Zone Exhaust Air Node Name
  ,                        !- System Inlet Air Node Name
  50,                      !- Maximum Heating Supply Air Temperature
  13,                      !- Minimum Cooling Supply Air Temperature
  0.015,                   !- Maximum Heating Supply Air Humidity Ratio
  0.009;                   !- Minimum Cooling Supply Air Humidity Ratio

Fan:ConstantVolume,
  Supply Fan,              !- Name
  AlwaysOn,                !- Availability Schedule Name
  0.7,                     !- Fan Total Efficiency
  500,                     !- Pressure Rise
  1.2,                     !- Maximum Flow Rate
  0.9,                     !- Motor Efficiency
  1.0,                     !- Motor In Airstream Fraction
  Main Air Inlet Node,     !- Air Inlet Node Name
  Main Air Outlet Node;    !- Air Outlet Node Name

Coil:Heating:Water,
  Main Heating Coil,       !- Name
  AlwaysOn,                !- Availability Schedule Name
  autosize,                !- U-Factor Times Area Value
  autosize,                !- Maximum Water Flow Rate
  Main Water Inlet Node,   !- Water Inlet Node Name
  Main Water Outlet Node,  !- Water Outlet Node Name
  Main Air Outlet Node,    !- Air Inlet Node Name
  Heated Air Node;         !- Air Outlet Node Name
`;
