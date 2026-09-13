const token = (value) => String(value || "").trim().toLowerCase();
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const only = (values) => { const modes = new Set(values.filter(Boolean)); return modes.size === 1 ? [...modes][0] : ""; };

// EnergyPlus uses -999 for an unset node temperature setpoint. Hourly averages
// can move that sentinel a few floating-point steps above -999 (observed in
// eplusout.sql as -998.9999999999999). Do not treat that roundoff as a control.
export function hasHVACTemperatureSetpoint(value) {
  return finite(value) && value > -999 + 1e-6;
}

function equipmentMode(type) {
  const name = token(type);
  if (/^(coil:cooling|chiller:|coolingtower:|evaporativecooler:|districtcooling)/.test(name) || name.includes(":cooling")) return "cooling";
  if (/^(coil:heating|boiler:|waterheater:|districtheating)/.test(name) || name.includes(":heating")) return "heating";
  return "";
}

// Use typed equipment and the current frame's reported operation, never loop
// name substrings. A mixed/off loop without a resolved mode stays unclassified.
export function hvacSetpointMode(loop, node, components) {
  const supply = (loop.supplySide?.branches || []).flatMap((branch) => branch.components || []);
  const supplyOwners = new Set(supply.map((item) => `${token(item.objectType)}|${token(item.objectName)}`));
  const active = components.filter((item) => item.status === "on" && equipmentMode(item.type));
  const ownsNode = (item) => [...(item.nodePorts || []).map((port) => port.nodeName), ...(item.inletNodes || []), ...(item.outletNodes || [])].some((name) => token(name) === token(node));
  const local = only(active.filter(ownsNode).map((item) => equipmentMode(item.type)));
  if (local) return local;
  const activeSupply = active.filter((item) => supplyOwners.has(`${token(item.type)}|${token(item.name)}`) || supplyOwners.has(`${token(item.parentComponentType)}|${token(item.parentComponentName)}`));
  const operating = only(activeSupply.map((item) => equipmentMode(item.type)));
  if (operating) return operating;
  return only(supply.map((item) => equipmentMode(item.objectType)));
}

export function hvacSetpointComparison(temperature, setpoint, mode) {
  if (!finite(temperature) || !hasHVACTemperatureSetpoint(setpoint)) return null;
  const actual = Number(temperature.toFixed(2)), target = Number(setpoint.toFixed(2));
  return { value: setpoint, operator: actual < target ? "<" : actual > target ? ">" : "=",
    state: mode === "cooling" ? actual <= target ? "satisfied" : "unmet" : mode === "heating" ? actual >= target ? "satisfied" : "unmet" : "unknown" };
}
