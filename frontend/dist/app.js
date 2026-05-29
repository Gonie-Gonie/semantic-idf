const sampleIDF = `Version,
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

const state = {
  report: null,
  model: null,
  epjsonText: "",
  activeTab: "summary",
  activeInputView: "text",
  lastAnalyzedText: "",
  tableOrientation: "objects",
  tableGroupOrientations: new Map(),
};

const elements = {
  runtimeStatus: document.querySelector("#runtimeStatus"),
  fileInput: document.querySelector("#fileInput"),
  analyzeButton: document.querySelector("#analyzeButton"),
  removeUnusedButton: document.querySelector("#removeUnusedButton"),
  toIDFButton: document.querySelector("#toIDFButton"),
  toEPJSONButton: document.querySelector("#toEPJSONButton"),
  downloadButton: document.querySelector("#downloadButton"),
  guideButton: document.querySelector("#guideButton"),
  idfInput: document.querySelector("#idfInput"),
  jsonTextInput: document.querySelector("#jsonTextInput"),
  applyJSONButton: document.querySelector("#applyJSONButton"),
  textStats: document.querySelector("#textStats"),
  fieldStats: document.querySelector("#fieldStats"),
  textObjectView: document.querySelector("#textObjectView"),
  jsonStructuredView: document.querySelector("#jsonStructuredView"),
  fieldFilter: document.querySelector("#fieldFilter"),
  fieldTable: document.querySelector("#fieldTable"),
  tableOrientationButtons: document.querySelectorAll(".orientation-button"),
  workspace: document.querySelector(".workspace"),
  workspaceSplitter: document.querySelector("#workspaceSplitter"),
  inputViewButtons: document.querySelectorAll(".view-tab"),
  inputViews: document.querySelectorAll(".input-view"),
  objectCount: document.querySelector("#objectCount"),
  typeCount: document.querySelector("#typeCount"),
  scheduleCount: document.querySelector("#scheduleCount"),
  unusedCount: document.querySelector("#unusedCount"),
  typeList: document.querySelector("#typeList"),
  zoneViz: document.querySelector("#zoneViz"),
  systemViz: document.querySelector("#systemViz"),
  objectTable: document.querySelector("#objectTable"),
  objectFilter: document.querySelector("#objectFilter"),
  scheduleList: document.querySelector("#scheduleList"),
  unusedList: document.querySelector("#unusedList"),
  connectionList: document.querySelector("#connectionList"),
  tabs: document.querySelectorAll(".tab"),
  panes: document.querySelectorAll(".tab-pane"),
};

function backend() {
  return window.go && window.go.main && window.go.main.App;
}

function setStatus(message, tone = "muted") {
  elements.runtimeStatus.textContent = message;
  const colors = {
    muted: "#60707c",
    ok: "#246b44",
    warn: "#a85f00",
    error: "#b3261e",
  };
  elements.runtimeStatus.style.color = colors[tone] || colors.muted;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

async function analyze() {
  const api = backend();
  updateTextStats();
  if (!api) {
    setStatus("Run with Go/Wails to enable IDF or epJSON analysis", "warn");
    renderEmpty();
    return;
  }

  try {
    const text = elements.idfInput.value;
    const result =
      typeof api.AnalyzeInputText === "function"
        ? await api.AnalyzeInputText(text)
        : { report: await api.AnalyzeIDFText(text), model: null, epjson: "" };
    state.report = result.report;
    state.model = result.model || null;
    state.epjsonText = result.epjson || "";
    state.lastAnalyzedText = text;
    renderReport();
    setStatus("Analysis complete", "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

async function removeUnused() {
  const api = backend();
  if (!api) {
    setStatus("Backend unavailable", "warn");
    return;
  }

  try {
    const result = await api.RemoveUnusedObjectsText(elements.idfInput.value);
    elements.idfInput.value = result.text;
    updateTextStats();
    await analyze();
    setStatus("Unused objects removed", "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

async function convertInput(targetFormat) {
  const api = backend();
  if (!api || typeof api.ConvertInputText !== "function") {
    setStatus("Backend unavailable", "warn");
    return;
  }

  try {
    const result = await api.ConvertInputText(elements.idfInput.value, targetFormat);
    elements.idfInput.value = result.text;
    updateTextStats();
    await analyze();
    const label = targetFormat === "epjson" ? "epJSON" : "IDF";
    setStatus(`Converted to ${label}`, "ok");
  } catch (error) {
    setStatus(error.message || String(error), "error");
  }
}

function updateTextStats() {
  const text = elements.idfInput.value;
  const lines = text.length === 0 ? 0 : text.split(/\r\n|\r|\n/).length;
  elements.textStats.textContent = `${lines} lines`;
}

function renderReport() {
  const report = state.report;
  if (!report) {
    renderEmpty();
    return;
  }

  elements.objectCount.textContent = report.objectCount ?? 0;
  elements.typeCount.textContent = report.typeCounts?.length ?? 0;
  elements.scheduleCount.textContent = report.schedules?.length ?? 0;
  elements.unusedCount.textContent = report.unusedObjects?.length ?? 0;

  renderTypeList(report.typeCounts || []);
  renderZoneViz(report.zones || []);
  renderObjectTable(report.objects || []);
  renderScheduleList(report.schedules || []);
  renderUnusedList(report.unusedObjects || []);
  renderSystemViz(report.hvacConnections || []);
  renderConnectionList(report.hvacConnections || []);
  renderInputViews();
}

function renderEmpty() {
  elements.objectCount.textContent = "0";
  elements.typeCount.textContent = "0";
  elements.scheduleCount.textContent = "0";
  elements.unusedCount.textContent = "0";
  elements.typeList.innerHTML = `<div class="empty">No analysis yet</div>`;
  elements.objectTable.innerHTML = `<div class="empty">No objects yet</div>`;
  elements.scheduleList.innerHTML = `<div class="empty">No schedules yet</div>`;
  elements.unusedList.innerHTML = `<div class="empty">No unused objects yet</div>`;
  elements.connectionList.innerHTML = `<div class="empty">No connections yet</div>`;
  elements.zoneViz.innerHTML = "";
  elements.systemViz.innerHTML = "";
  elements.textObjectView.innerHTML = `<div class="empty">No formatted input yet</div>`;
  elements.jsonStructuredView.innerHTML = `<div class="empty">No structured input yet</div>`;
  elements.jsonTextInput.value = "";
  elements.fieldTable.innerHTML = `<div class="empty">No field table yet</div>`;
  elements.fieldStats.textContent = "0 tables";
}

function renderTypeList(typeCounts) {
  elements.typeList.innerHTML = typeCounts.length
    ? typeCounts
        .map(
          (item) => `
            <div class="list-row">
              <span class="row-main" title="${escapeHTML(item.type)}">${escapeHTML(item.type)}</span>
              <span class="badge">${escapeHTML(item.count)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No object types</div>`;
}

function renderObjectTable(objects) {
  const filter = elements.objectFilter.value.trim().toLowerCase();
  const filtered = objects.filter((object) => {
    const haystack = `${object.index} ${object.type} ${object.name || ""}`.toLowerCase();
    return haystack.includes(filter);
  });

  const rows = filtered
    .map(
      (object) => `
        <div class="table-row">
          <span class="row-sub">#${escapeHTML(object.index)}</span>
          <span class="row-main" title="${escapeHTML(object.type)}">${escapeHTML(object.type)}</span>
          <span class="row-main" title="${escapeHTML(object.name || "")}">${escapeHTML(object.name || "-")}</span>
          <span class="badge">${escapeHTML(object.fieldCount)}</span>
        </div>`,
    )
    .join("");

  elements.objectTable.innerHTML = `
    <div class="table-row table-head">
      <span>Index</span>
      <span>Type</span>
      <span>Name</span>
      <span>Fields</span>
    </div>
    ${rows || `<div class="empty">No matching objects</div>`}
  `;
}

function renderScheduleList(schedules) {
  elements.scheduleList.innerHTML = schedules.length
    ? schedules
        .map(
          (schedule) => `
            <div class="list-row">
              <span>
                <span class="row-main" title="${escapeHTML(schedule.name)}">${escapeHTML(schedule.name)}</span>
                <span class="row-sub">${escapeHTML(schedule.type)}</span>
              </span>
              <span class="badge">#${escapeHTML(schedule.index)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No schedules</div>`;
}

function renderUnusedList(unusedObjects) {
  elements.unusedList.innerHTML = unusedObjects.length
    ? unusedObjects
        .map(
          (object) => `
            <div class="list-row">
              <span>
                <span class="row-main" title="${escapeHTML(object.name)}">${escapeHTML(object.name)}</span>
                <span class="row-sub">${escapeHTML(object.type)}</span>
              </span>
              <span class="badge">#${escapeHTML(object.index)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No unused named objects</div>`;
}

function renderConnectionList(connections) {
  elements.connectionList.innerHTML = connections.length
    ? connections
        .map(
          (connection) => `
            <div class="list-row">
              <span>
                <span class="row-main">${escapeHTML(connection.fromNode)} -> ${escapeHTML(connection.toNode)}</span>
                <span class="row-sub">${escapeHTML(connection.objectType)} ${escapeHTML(connection.objectName || "")}</span>
              </span>
              <span class="badge">#${escapeHTML(connection.objectIndex)}</span>
            </div>`,
        )
        .join("")
    : `<div class="empty">No node-to-node connections</div>`;
}

function renderInputViews() {
  if (state.activeInputView === "text") {
    renderFormattedTextView();
  }
  if (state.activeInputView === "json") {
    renderJSONView();
  }
  if (state.activeInputView === "table") {
    renderFieldTable();
  }
}

function groupedObjectsFromModel() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    return [];
  }

  const groups = [];
  const byType = new Map();
  model.objects.forEach((object) => {
    if (!byType.has(object.type)) {
      const group = { type: object.type, objects: [] };
      groups.push(group);
      byType.set(object.type, group);
    }
    byType.get(object.type).objects.push(object);
  });
  return groups;
}

function renderFormattedTextView() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    elements.textObjectView.innerHTML = `<div class="empty">Analyze input to build formatted text view</div>`;
    return;
  }

  const versionLabel = model.version?.raw || "unknown";
  const groups = groupedObjectsFromModel();
  elements.textObjectView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(model.objects.length)} objects</span>
    </div>
    <div class="json-groups">
      ${groups
        .map(
          (group, index) => `
            <details class="json-group" ${index < 4 ? "open" : ""}>
              <summary>
                <span>${escapeHTML(group.type)}</span>
                <span class="badge">${escapeHTML(group.objects.length)}</span>
              </summary>
              ${group.objects.map(renderFormattedObject).join("")}
            </details>`,
        )
        .join("")}
    </div>
  `;
}

function renderJSONView() {
  const model = state.model;
  if (!model || !Array.isArray(model.objects)) {
    elements.jsonStructuredView.innerHTML = `<div class="empty">Analyze input to build JSON view</div>`;
    elements.jsonTextInput.value = "";
    return;
  }

  const versionLabel = model.version?.raw || "unknown";
  if (document.activeElement !== elements.jsonTextInput) {
    elements.jsonTextInput.value = state.epjsonText || "";
  }

  elements.jsonStructuredView.innerHTML = `
    <div class="json-meta">
      <span class="badge">${escapeHTML(model.format || "unknown")}</span>
      <span class="badge">Version ${escapeHTML(versionLabel)}</span>
      <span class="badge">${escapeHTML(model.objects.length)} objects</span>
    </div>
    <div class="json-tree primary-tree">${renderJSONTree(model)}</div>
  `;
}

function renderFormattedObject(object) {
  const fields = object.fields || [];
  return `
    <section class="json-object">
      <div class="json-object-head">
        <strong title="${escapeHTML(object.name || "")}">${escapeHTML(object.name || "(unnamed)")}</strong>
        <span class="row-sub">#${escapeHTML(object.sourceIndex ?? "")}</span>
      </div>
      <dl>
        ${fields
          .map(
            (field) => `
              <dt title="${escapeHTML(field.key || field.comment || "")}">${escapeHTML(field.key || field.comment || "field")}</dt>
              <dd>${renderJSONFieldValue(field.value)}</dd>`,
          )
          .join("")}
      </dl>
    </section>
  `;
}

function formatJSONValue(value) {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

function renderJSONFieldValue(value) {
  if (value && typeof value === "object") {
    return `<div class="json-inline-tree">${renderJSONTree(value)}</div>`;
  }
  return `<span title="${escapeHTML(formatJSONValue(value))}">${escapeHTML(formatJSONValue(value))}</span>`;
}

function renderJSONTree(value, depth = 0) {
  const openAttr = depth < 2 ? "open" : "";
  if (Array.isArray(value)) {
    if (!value.length) {
      return `<span class="json-primitive">[]</span>`;
    }
    return `
      <details class="json-node" ${openAttr}>
        <summary>Array <span class="badge">${escapeHTML(value.length)}</span></summary>
        <ol>
          ${value.map((item, index) => `<li><span class="json-key">${escapeHTML(index)}</span>${renderJSONTree(item, depth + 1)}</li>`).join("")}
        </ol>
      </details>`;
  }

  if (value && typeof value === "object") {
    const entries = Object.entries(value);
    if (!entries.length) {
      return `<span class="json-primitive">{}</span>`;
    }
    return `
      <details class="json-node" ${openAttr}>
        <summary>Object <span class="badge">${escapeHTML(entries.length)}</span></summary>
        <ol>
          ${entries
            .map(([key, child]) => `<li><span class="json-key">${escapeHTML(key)}</span>${renderJSONTree(child, depth + 1)}</li>`)
            .join("")}
        </ol>
      </details>`;
  }

  return `<span class="json-primitive">${escapeHTML(formatJSONValue(value))}</span>`;
}

function renderFieldTable() {
  const report = state.report;
  if (!report || !Array.isArray(report.objects)) {
    elements.fieldTable.innerHTML = `<div class="empty">Analyze input to build table view</div>`;
    elements.fieldStats.textContent = "0 tables";
    return;
  }

  const filter = elements.fieldFilter.value.trim().toLowerCase();
  const groups = [];
  const byType = new Map();
  report.objects.forEach((object) => {
    const haystack = [
      object.index,
      object.type,
      object.name || "",
      ...(object.fields || []).flatMap((field) => [field.comment || "", field.value || ""]),
    ]
      .join(" ")
      .toLowerCase();
    if (filter && !haystack.includes(filter)) {
      return;
    }

    if (!byType.has(object.type)) {
      const group = { type: object.type, objects: [] };
      groups.push(group);
      byType.set(object.type, group);
    }
    byType.get(object.type).objects.push(object);
  });

  const objectCount = groups.reduce((sum, group) => sum + group.objects.length, 0);
  const orientationLabel = state.tableOrientation === "fields" ? "fields as rows" : "objects as rows";
  elements.fieldStats.textContent = `${groups.length} tables, ${objectCount} objects, ${orientationLabel}`;
  if (!groups.length) {
    elements.fieldTable.innerHTML = `<div class="empty">No matching object tables</div>`;
    return;
  }

  elements.fieldTable.innerHTML = `
    ${groups.map((group, index) => renderObjectTypeTable(group, index)).join("")}
  `;

  elements.fieldTable.querySelectorAll(".field-value-input").forEach((input) => {
    input.addEventListener("blur", () => applyTableValue(input));
    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        input.blur();
      }
      if (event.key === "Escape") {
        input.value = input.dataset.original || "";
        input.blur();
      }
    });
  });
  elements.fieldTable.querySelectorAll(".object-orientation-button").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      state.tableGroupOrientations.set(button.dataset.objectType, button.dataset.nextOrientation);
      renderFieldTable();
    });
  });
}

function renderObjectTypeTable(group, groupIndex) {
  const orientation = state.tableGroupOrientations.get(group.type) || state.tableOrientation;
  const columns = buildObjectTypeColumns(group.objects);
  const nextOrientation = orientation === "objects" ? "fields" : "objects";
  return `
    <details class="object-table-group" ${groupIndex < 5 ? "open" : ""}>
      <summary>
        <span>${escapeHTML(group.type)}</span>
        <span class="object-table-actions">
          <button class="object-orientation-button" data-object-type="${escapeHTML(group.type)}" data-next-orientation="${escapeHTML(nextOrientation)}" type="button">
            ${orientation === "objects" ? "Fields as rows" : "Objects as rows"}
          </button>
          <span class="badge">${escapeHTML(group.objects.length)} objects</span>
        </span>
      </summary>
      <div class="object-type-table-scroll">
        ${orientation === "objects" ? renderObjectsAsRowsTable(group, columns) : renderFieldsAsRowsTable(group, columns)}
      </div>
    </details>
  `;
}

function renderObjectsAsRowsTable(group, columns) {
  return `
    <table>
      <thead>
        <tr>
          <th class="sticky-col">Obj</th>
          <th>Name</th>
          ${columns.map((column) => `<th title="${escapeHTML(column.label)}">${escapeHTML(column.label)}</th>`).join("")}
        </tr>
      </thead>
      <tbody>
        ${group.objects
          .map(
            (object) => `
              <tr>
                <td class="sticky-col">#${escapeHTML(object.index)}</td>
                <td title="${escapeHTML(object.name || "")}">${escapeHTML(object.name || "-")}</td>
                ${columns.map((column) => renderObjectTypeCell(object, column.index)).join("")}
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>
  `;
}

function renderFieldsAsRowsTable(group, columns) {
  return `
    <table>
      <thead>
        <tr>
          <th class="sticky-col">Field</th>
          ${group.objects
            .map(
              (object) => `
                <th title="${escapeHTML(object.name || "")}">
                  #${escapeHTML(object.index)} ${escapeHTML(object.name || "-")}
                </th>`,
            )
            .join("")}
        </tr>
      </thead>
      <tbody>
        ${columns
          .map(
            (column) => `
              <tr>
                <td class="sticky-col" title="${escapeHTML(column.label)}">${escapeHTML(column.label)}</td>
                ${group.objects.map((object) => renderObjectTypeCell(object, column.index)).join("")}
              </tr>`,
          )
          .join("")}
      </tbody>
    </table>
  `;
}

function buildObjectTypeColumns(objects) {
  const maxFields = Math.max(...objects.map((object) => (object.fields || []).length), 0);
  return Array.from({ length: maxFields }, (_, index) => {
    const fieldWithComment = objects
      .map((object) => (object.fields || [])[index])
      .find((field) => field && field.comment);
    return {
      index,
      label: fieldWithComment?.comment || `Field ${index + 1}`,
    };
  });
}

function renderObjectTypeCell(object, fieldIndex) {
  const field = (object.fields || [])[fieldIndex];
  if (!field) {
    return `<td class="empty-cell"></td>`;
  }

  const value = field.value || "";
  const label = field.comment || `Field ${fieldIndex + 1}`;
  return `
    <td title="${escapeHTML(label)}">
      <input class="field-value-input" data-object-index="${escapeHTML(object.index)}"
        data-field-index="${escapeHTML(fieldIndex)}" data-original="${escapeHTML(value)}"
        value="${escapeHTML(value)}" />
    </td>`;
}

async function applyTableValue(input) {
  const nextValue = input.value;
  if (nextValue === input.dataset.original) {
    return;
  }

  const api = backend();
  if (!api || typeof api.UpdateFieldText !== "function") {
    setStatus("Backend unavailable", "warn");
    input.value = input.dataset.original || "";
    return;
  }

  try {
    const result = await api.UpdateFieldText(
      elements.idfInput.value,
      Number(input.dataset.objectIndex),
      Number(input.dataset.fieldIndex),
      nextValue,
    );
    elements.idfInput.value = result.text;
    updateTextStats();
    await analyze();
    setStatus("Field updated", "ok");
  } catch (error) {
    input.value = input.dataset.original || "";
    setStatus(error.message || String(error), "error");
  }
}

function renderZoneViz(zones) {
  const svg = elements.zoneViz;
  const width = 560;
  const height = 260;
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);

  if (!zones.length) {
    svg.innerHTML = `<text x="24" y="48" fill="#60707c" font-size="14">No zones</text>`;
    return;
  }

  const columns = Math.min(3, zones.length);
  const cellWidth = (width - 48) / columns;
  const cellHeight = 78;
  const content = zones
    .map((zone, index) => {
      const col = index % columns;
      const row = Math.floor(index / columns);
      const x = 24 + col * cellWidth;
      const y = 28 + row * (cellHeight + 18);
      const surfaceText = `${zone.surfaceCount || 0} surfaces`;
      return `
        <g>
          <rect x="${x}" y="${y}" width="${cellWidth - 14}" height="${cellHeight}" rx="6" fill="#e9f5f6" stroke="#007c89" />
          <text x="${x + 12}" y="${y + 30}" fill="#18222b" font-size="14" font-weight="700">${escapeHTML(zone.name)}</text>
          <text x="${x + 12}" y="${y + 54}" fill="#60707c" font-size="12">${escapeHTML(surfaceText)}</text>
        </g>`;
    })
    .join("");
  svg.innerHTML = content;
}

function renderSystemViz(connections) {
  const svg = elements.systemViz;
  const width = 800;
  const height = 260;
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);

  if (!connections.length) {
    svg.innerHTML = `<text x="24" y="48" fill="#60707c" font-size="14">No HVAC connections</text>`;
    return;
  }

  const nodes = [...new Set(connections.flatMap((item) => [item.fromNode, item.toNode]))].slice(0, 9);
  const spacing = (width - 100) / Math.max(nodes.length - 1, 1);
  const y = 112;
  const nodeX = new Map(nodes.map((node, index) => [node, 50 + index * spacing]));

  const paths = connections
    .filter((connection) => nodeX.has(connection.fromNode) && nodeX.has(connection.toNode))
    .map((connection) => {
      const x1 = nodeX.get(connection.fromNode);
      const x2 = nodeX.get(connection.toNode);
      const mid = (x1 + x2) / 2;
      return `
        <path d="M ${x1} ${y} C ${mid} ${y - 52}, ${mid} ${y - 52}, ${x2} ${y}"
          fill="none" stroke="#a85f00" stroke-width="2" marker-end="url(#arrow)" />`;
    })
    .join("");

  const nodeMarks = nodes
    .map((node) => {
      const x = nodeX.get(node);
      const label = node.length > 18 ? `${node.slice(0, 16)}...` : node;
      return `
        <g>
          <circle cx="${x}" cy="${y}" r="15" fill="#ffffff" stroke="#007c89" stroke-width="2" />
          <text x="${x}" y="${y + 36}" text-anchor="middle" fill="#18222b" font-size="12">${escapeHTML(label)}</text>
        </g>`;
    })
    .join("");

  svg.innerHTML = `
    <defs>
      <marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
        <path d="M 0 0 L 8 4 L 0 8 z" fill="#a85f00"></path>
      </marker>
    </defs>
    ${paths}
    ${nodeMarks}
  `;
}

function downloadText() {
  const blob = new Blob([elements.idfInput.value], { type: "text/plain" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = state.model?.format === "epjson" ? "model.epJSON" : "model.idf";
  link.click();
  URL.revokeObjectURL(url);
}

function openGuide() {
  window.location.assign("./guide.html");
}

function switchTab(tabName) {
  state.activeTab = tabName;
  elements.tabs.forEach((tab) => {
    tab.classList.toggle("active", tab.dataset.tab === tabName);
  });
  elements.panes.forEach((pane) => {
    pane.classList.toggle("active", pane.id === `${tabName}Pane`);
  });
}

async function switchInputView(viewName) {
  state.activeInputView = viewName;
  elements.inputViewButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.inputView === viewName);
  });
  elements.inputViews.forEach((view) => {
    view.classList.toggle("active", view.id === `${viewName}InputView`);
  });

  if (viewName !== "text" && state.lastAnalyzedText !== elements.idfInput.value) {
    await analyze();
    return;
  }
  renderInputViews();
}

async function applyJSONText() {
  elements.idfInput.value = elements.jsonTextInput.value;
  updateTextStats();
  state.lastAnalyzedText = "";
  await analyze();
  setStatus("JSON applied", "ok");
}

function setTableOrientation(orientation) {
  state.tableOrientation = orientation;
  state.tableGroupOrientations.clear();
  elements.tableOrientationButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.tableOrientation === orientation);
  });
  renderFieldTable();
}

function initializeWorkspaceSplitter() {
  const savedWidth = localStorage.getItem("idfAnalyzer.editorWidth");
  if (savedWidth) {
    elements.workspace.style.setProperty("--editor-width", savedWidth);
  }

  let dragging = false;
  elements.workspaceSplitter.addEventListener("pointerdown", (event) => {
    dragging = true;
    elements.workspaceSplitter.setPointerCapture(event.pointerId);
    document.body.classList.add("resizing-workspace");
  });

  elements.workspaceSplitter.addEventListener("pointermove", (event) => {
    if (!dragging) {
      return;
    }
    const rect = elements.workspace.getBoundingClientRect();
    const splitterWidth = elements.workspaceSplitter.getBoundingClientRect().width;
    const minLeft = 420;
    const minRight = 420;
    const nextWidth = Math.min(
      Math.max(event.clientX - rect.left, minLeft),
      rect.width - splitterWidth - minRight,
    );
    const value = `${Math.round(nextWidth)}px`;
    elements.workspace.style.setProperty("--editor-width", value);
    localStorage.setItem("idfAnalyzer.editorWidth", value);
  });

  function stopDrag(event) {
    if (!dragging) {
      return;
    }
    dragging = false;
    if (event.pointerId !== undefined) {
      try {
        elements.workspaceSplitter.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture may already be released by the browser.
      }
    }
    document.body.classList.remove("resizing-workspace");
  }

  elements.workspaceSplitter.addEventListener("pointerup", stopDrag);
  elements.workspaceSplitter.addEventListener("pointercancel", stopDrag);
}

elements.fileInput.addEventListener("change", async (event) => {
  const [file] = event.target.files || [];
  if (!file) {
    return;
  }
  elements.idfInput.value = await file.text();
  updateTextStats();
  await analyze();
});

elements.analyzeButton.addEventListener("click", analyze);
elements.removeUnusedButton.addEventListener("click", removeUnused);
elements.toIDFButton.addEventListener("click", () => convertInput("idf"));
elements.toEPJSONButton.addEventListener("click", () => convertInput("epjson"));
elements.downloadButton.addEventListener("click", downloadText);
elements.guideButton.addEventListener("click", openGuide);
elements.applyJSONButton.addEventListener("click", applyJSONText);
elements.idfInput.addEventListener("input", () => {
  updateTextStats();
  state.lastAnalyzedText = "";
});
elements.jsonTextInput.addEventListener("input", () => {
  state.lastAnalyzedText = "";
});
elements.objectFilter.addEventListener("input", () => {
  if (state.report) {
    renderObjectTable(state.report.objects || []);
  }
});
elements.fieldFilter.addEventListener("input", renderFieldTable);
elements.tabs.forEach((tab) => {
  tab.addEventListener("click", () => switchTab(tab.dataset.tab));
});
elements.inputViewButtons.forEach((button) => {
  button.addEventListener("click", () => switchInputView(button.dataset.inputView));
});
elements.tableOrientationButtons.forEach((button) => {
  button.addEventListener("click", () => setTableOrientation(button.dataset.tableOrientation));
});

initializeWorkspaceSplitter();
elements.idfInput.value = sampleIDF;
updateTextStats();
renderEmpty();
analyze();
