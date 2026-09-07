const METER_OUTPUT_TYPES = new Set([
  "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly",
]);

const METER_CARRIERS = Object.freeze({
  electricity: "electricity", naturalgas: "natural_gas", gas: "natural_gas",
  districtcooling: "district_cooling", districtheating: "district_heating", districtheatingwater: "district_heating",
  districtheatingsteam: "steam", steam: "steam", propane: "propane", coal: "coal", diesel: "diesel", gasoline: "gasoline",
  fueloilno1: "fuel_oil_1", fueloil1: "fuel_oil_1", fueloilno2: "fuel_oil_2", fueloil2: "fuel_oil_2",
  otherfuel1: "other_fuel_1", otherfuel2: "other_fuel_2", water: "water",
});

function text(value) {
  return String(value ?? "").trim();
}

function token(value) {
  return text(value).toLowerCase().replace(/\s+/g, " ");
}

function compact(value) {
  return token(value).replace(/[^a-z0-9]+/g, "");
}

function optionalIndex(value) {
  if (value === null || value === undefined || text(value) === "") return null;
  if (typeof value !== "number" && typeof value !== "string") return null;
  const index = Number(value);
  return Number.isInteger(index) && index >= 0 ? index : null;
}

function requestField(object, property, ...fieldNames) {
  if (text(object?.[property])) return text(object[property]);
  const fields = Array.isArray(object?.fields) ? object.fields : [];
  return text(fields.find((field) => fieldNames.some((name) => token(field?.name) === token(name)))?.value);
}

/** Display and matching read the same typed-or-field-only request identity. */
export function energyPathOutputRequestFields(object) {
  return {
    objectType: text(object?.objectType),
    keyValue: requestField(object, "keyValue", "Key Value", "Key Name"),
    variableName: requestField(object, "variableName", "Variable Name"),
    reportingFrequency: requestField(object, "reportingFrequency", "Reporting Frequency"),
  };
}

function requestIdentity(object) {
  const fields = energyPathOutputRequestFields(object);
  return {
    type: token(fields.objectType),
    key: token(fields.keyValue),
    name: token(fields.variableName),
    // Spaces differ in stored SQL dictionary labels such as Run Period; no
    // annual/monthly or detailed/timestep equivalence is inferred.
    frequency: token(fields.reportingFrequency).replace(/\s+/g, ""),
  };
}

function meterGroup(value) {
  const special = {
    electricityproducedfacility: "electricity|generators",
    generatorselectricityproduced: "electricity|generators",
  };
  if (special[compact(value)]) return special[compact(value)];
  const parts = text(value).split(":").map(compact);
  if (parts.length !== 2 || parts.some((part) => !part)) return "";
  const left = METER_CARRIERS[parts[0]];
  const right = METER_CARRIERS[parts[1]];
  const endUse = left ? parts[1] : right ? parts[0] : "";
  if (!endUse || (left && right)) return "";
  const aliases = { facility: "total", dhw: "watersystems", humidifier: "humidification", interiorlighting: "interiorlights", exteriorlighting: "exteriorlights" };
  return `${left || right}|${aliases[endUse] || endUse}`;
}

function meterNamesMatch(left, right) {
  if (!text(left) || !text(right)) return false;
  if (token(left) === token(right)) return true;
  const group = meterGroup(left);
  return group !== "" && group === meterGroup(right);
}

function resolution(status, matches = []) {
  const exact = status === "exact" ? matches[0] : null;
  return {
    status,
    request: exact?.object ?? null,
    requestIndex: exact?.index ?? -1,
    candidates: matches.map((match) => match.object),
  };
}

/**
 * Resolve an Energy Path observation to an output-plan request without selecting
 * a plausible-looking first match. objectIndex is a hint, never an identity
 * bypass: old stored sources can have an index from a different frequency.
 * Derived and tabular observations intentionally do not acquire direct links.
 */
export function resolveEnergyPathOutputRequest(source, outputObjects = [], sources = []) {
  if (typeof source === "string") {
    source = (Array.isArray(sources) ? sources : []).find((candidate) => candidate?.id === source);
  }
  if (!source || typeof source !== "object") return resolution("unavailable");
  const sourceType = token(source.sourceType);
  if (sourceType === "sql_tabular" || sourceType === "tabular" || token(source.aggregationMethod) === "tabular_annual_value") {
    return resolution("tabular");
  }
  if (sourceType.startsWith("derived") || (Array.isArray(source.inputSourceIds) && source.inputSourceIds.length > 0)) {
    return resolution("derived");
  }

  const explicitType = token(source.objectType);
  const isMeter = source.isMeter === true || METER_OUTPUT_TYPES.has(explicitType) || sourceType === "sql_meter";
  if (explicitType && !METER_OUTPUT_TYPES.has(explicitType) && explicitType !== "output:variable") return resolution("unavailable");
  if (explicitType === "output:variable" && isMeter) return resolution("unavailable");
  if ((sourceType === "sql_variable" && isMeter) || (METER_OUTPUT_TYPES.has(explicitType) && source.isMeter === false)) return resolution("unavailable");
  const sourceName = token(isMeter ? source.keyValue || source.name : source.name || source.variableName);
  const sourceKey = token(source.keyValue);
  const frequency = token(source.reportingFrequency).replace(/\s+/g, "");
  if (!sourceName) return resolution("unavailable");

  const matches = [];
  for (const [index, object] of (Array.isArray(outputObjects) ? outputObjects : []).entries()) {
    const identity = requestIdentity(object);
    if (explicitType && identity.type !== explicitType) continue;
    if (isMeter ? !METER_OUTPUT_TYPES.has(identity.type) : identity.type !== "output:variable") continue;
    if (frequency && identity.frequency && frequency !== identity.frequency) continue;
    let rank = 1;
    if (isMeter) {
      if (!meterNamesMatch(identity.key, sourceName)) continue;
    } else {
      if (identity.name !== sourceName) continue;
      if (sourceKey && identity.key !== sourceKey && identity.key !== "*" && identity.key !== "") continue;
      rank = sourceKey && identity.key === sourceKey ? 2 : 1;
    }
    matches.push({ object, index, rank, complete: Boolean(frequency && identity.frequency && (isMeter || (sourceKey && identity.key))) });
  }
  if (matches.length === 0) return resolution("unavailable");
  const bestRank = Math.max(...matches.map((match) => match.rank));
  const best = matches.filter((match) => match.rank === bestRank);
  // Incomplete identity cannot become exact merely because the current plan
  // happens to contain only one compatible row.
  if (best.some((match) => !match.complete)) return resolution(best.length > 1 ? "ambiguous" : "unavailable", best);
  const sourceIndex = optionalIndex(source.objectIndex);
  const indexed = sourceIndex === null ? [] : best.filter((match) => optionalIndex(match.object?.objectIndex) === sourceIndex);
  if (indexed.length === 1) return resolution("exact", indexed);
  return resolution(best.length === 1 ? "exact" : "ambiguous", best);
}

/** Stable within the output plan, including duplicate requests and no IDF index. */
export function energyPathOutputRequestKey(object, index) {
  const identity = requestIdentity(object);
  const parts = [identity.type, identity.key, identity.name, identity.frequency, text(object?.signature)];
  // Array position guarantees distinct keys for duplicate rows. Encoding the
  // complete identity avoids hash collisions and keeps the token DOM-safe.
  const identityKey = Array.from(JSON.stringify(parts), (character) => character.codePointAt(0).toString(16)).join("-");
  return `energy-path-output-request-${optionalIndex(index) ?? "unknown"}-${identityKey}`;
}
