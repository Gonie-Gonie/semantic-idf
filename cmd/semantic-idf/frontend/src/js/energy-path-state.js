const record = (value) => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const supplied = (value, key) => Object.hasOwn(value, key) && value[key] !== undefined;
const text = (value) => typeof value === "string" ? value.trim() : "";
const token = (value) => text(value).toLowerCase();

function first(entries, fallback) {
  for (const [value, key] of entries) if (supplied(value, key)) return value[key];
  return fallback;
}

/**
 * Data-independent workspace/history migration. Only the six primary fields
 * are returned; obsolete rendering/focus settings cannot become active state.
 * Unknown Zone/selection/source IDs wait for validation against the restored
 * result, rather than selecting a first Zone or discarding a pending context.
 */
export function migrateEnergyPathState(input = {}) {
  const source = record(input), nested = supplied(source, "primary") ? record(source.primary) : {};
  const read = (suffix, fallback) => first([
    [source, `simulationEnergy${suffix}`], [nested, `simulationEnergy${suffix}`], [source, `energy${suffix}`],
  ], fallback);
  const legacy = (suffix, fallback = "") => first([[source, `simulationEnergy${suffix}`], [source, `energy${suffix}`]], fallback);
  const oldZoneScope = token(legacy("FocusMode")) === "zone";
  const scope = token(read("ScopeKind", oldZoneScope ? "zone" : "building"));
  const zoneName = text(read("ZoneName", legacy("ZoneFocus")));
  const period = token(read("Period", "annual"));
  const service = token(read("Service", "all"));
  const open = read("DetailsOpen", ["sources", "reconciliation"].includes(token(legacy("View"))));
  const primary = {
    simulationEnergyScopeKind: scope === "zone" && zoneName ? "zone" : "building",
    simulationEnergyZoneName: zoneName,
    simulationEnergyPeriod: /^m([1-9]|1[0-2])$/.test(period) ? period.toUpperCase() : "annual",
    simulationEnergyService: ["all", "cooling", "heating"].includes(service) ? service : "all",
    simulationEnergySelection: text(read("Selection", "")),
    simulationEnergyDetailsOpen: typeof open === "boolean" ? open : false,
  };
  let drawerSource;
  if (supplied(source, "energyDrawer")) drawerSource = record(source.energyDrawer);
  else if (supplied(source, "drawer")) drawerSource = record(source.drawer);
  else drawerSource = {
    tab: legacy("DetailsTab", "data"),
    stage: legacy("DetailsStage"),
    outputSource: legacy("OutputSource"),
  };
  const stages = { drivers: "drivers", loads: "loads", enduses: "endUses", carriers: "carriers" };
  const drawerValue = (key) => first([[drawerSource, key]], "");
  const stage = token(drawerValue("stage"));
  return {
    primary,
    drawer: {
      tab: token(drawerValue("tab")) === "output" ? "output" : "data",
      stage: Object.hasOwn(stages, stage) ? stages[stage] : "",
      outputSource: text(drawerValue("outputSource")),
    },
  };
}
