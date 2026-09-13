const token = (value) => String(value || "").trim().toLowerCase();
const positiveArea = (value) => typeof value === "number" && Number.isFinite(value) && value > 0 ? value : null;

// Areas belong to the executed model snapshot, never the currently edited model.
// The same denominator is used throughout the selected scope and period.
export function energyPathDisplayContext(explanation = {}, viewState = {}) {
  let scope = explanation.scope;
  if (token(viewState.simulationEnergyScopeKind || scope?.kind) === "zone") {
    const zone = token(viewState.simulationEnergyZoneName || scope?.zoneName);
    const zones = Array.isArray(explanation.zoneResults) ? explanation.zoneResults : [];
    scope = zones.find((item) => token(item?.scope?.zoneName) === zone)?.scope ||
      (token(scope?.kind) === "zone" && token(scope?.zoneName) === zone ? scope : null);
  }
  return Object.freeze({ perArea: true, areaM2: positiveArea(scope?.floorAreaM2) });
}

function energyUnit(unit) {
  const match = /^(j|kj|mj|gj|wh|kwh|mwh)(?:\s+(thermal|site))?$/.exec(token(unit));
  if (!match) return null;
  const factor = { j: 1 / 3600000, kj: 1 / 3600, mj: 1 / 3.6, gj: 1000 / 3.6, wh: 1 / 1000, kwh: 1, mwh: 1000 }[match[1]];
  return { factor, suffix: match[2] ? ` ${match[2]}` : "" };
}

export function energyPathDisplayUnit(unit = "", display) {
  const energy = display?.perArea && energyUnit(unit);
  return energy ? `kWh/m²${energy.suffix}` : unit;
}

export function formatEnergyPathDisplayValue(value, unit = "", display, { includeUnit = true } = {}) {
  const energy = display?.perArea && energyUnit(unit);
  let number = typeof value === "number" && Number.isFinite(value) ? value : null;
  if (energy) {
    const area = positiveArea(display.areaM2);
    number = number !== null && area !== null ? number * energy.factor / area : null;
  }
  if (number !== null && !Number.isFinite(number)) number = null;
  // Rounded negative zero is not a negative energy contribution.
  if (number !== null && Math.abs(number) < 0.005) number = 0;
  const formatted = number === null ? "—" : number.toLocaleString(undefined, {
    minimumFractionDigits: energy ? 2 : 0,
    maximumFractionDigits: 2,
  });
  return [formatted, includeUnit ? energyPathDisplayUnit(unit, display) : ""].filter(Boolean).join(" ");
}
