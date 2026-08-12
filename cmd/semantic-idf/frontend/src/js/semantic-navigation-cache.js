const EMPTY_NAVIGATION = Object.freeze({
  entities: Object.freeze([]),
  occurrences: Object.freeze([]),
  byEntityId: Object.freeze({}),
  byObjectId: Object.freeze({}),
  byObjectIndex: Object.freeze({}),
  byViewTarget: Object.freeze({}),
});
const EMPTY_LIST = Object.freeze([]);
const MAX_SHARED_CACHE_ENTRIES = 6;

let cacheBucketsByNavigation = new WeakMap();
const sharedCachesByKey = new Map();
let cacheBuildCount = 0;
let identityHitCount = 0;
let sharedHitCount = 0;

/**
 * Returns the immutable lookup cache for a semantic projection/navigation
 * payload. Live projections use a WeakMap identity fast path. Equivalent
 * payloads can also reuse a bounded text-hash + analyzer-version entry.
 */
export function getSemanticNavigationCache(source = null, metadata = {}) {
  const navigation = rawNavigation(source);
  const resolvedMetadata = navigationCacheMetadata(source, navigation, metadata);
  const identityKey = metadataIdentityKey(resolvedMetadata);
  let bucket = cacheBucketsByNavigation.get(navigation);
  const identityCached = bucket?.get(identityKey);
  if (identityCached) {
    identityHitCount += 1;
    touchSharedCache(identityCached.cacheKey);
    return identityCached;
  }

  const cacheKey = sharedCacheKey(navigation, resolvedMetadata);
  const sharedCached = resolvedMetadata.textHash ? sharedCachesByKey.get(cacheKey) : null;
  if (sharedCached) {
    sharedHitCount += 1;
    touchSharedCache(cacheKey);
    rememberIdentityCache(navigation, identityKey, sharedCached, bucket);
    return sharedCached;
  }

  const built = buildSemanticNavigationCache(navigation, resolvedMetadata, cacheKey);
  cacheBuildCount += 1;
  rememberIdentityCache(navigation, identityKey, built, bucket);
  if (resolvedMetadata.textHash) {
    sharedCachesByKey.set(cacheKey, built);
    trimSharedCaches();
  }
  return built;
}

/** Exposes bounded counters for performance regression tests and diagnostics. */
export function getSemanticNavigationCacheStats() {
  return Object.freeze({
    builds: cacheBuildCount,
    identityHits: identityHitCount,
    sharedHits: sharedHitCount,
    sharedEntries: sharedCachesByKey.size,
    sharedEntryLimit: MAX_SHARED_CACHE_ENTRIES,
  });
}

/** Test-only reset kept explicit so runtime code never clears a live cache. */
export function resetSemanticNavigationCacheForTests() {
  cacheBucketsByNavigation = new WeakMap();
  sharedCachesByKey.clear();
  cacheBuildCount = 0;
  identityHitCount = 0;
  sharedHitCount = 0;
}

function buildSemanticNavigationCache(navigation, metadata, cacheKey) {
  const entities = Array.isArray(navigation.entities) ? navigation.entities : EMPTY_LIST;
  const occurrences = Array.isArray(navigation.occurrences) ? navigation.occurrences : EMPTY_LIST;
  const entityById = recordsByID(entities, "id");
  const occurrenceById = recordsByID(occurrences, "occurrenceId");
  const occurrenceIdsByEntityId = reverseIndexMap(navigation.byEntityId);
  const occurrenceIdsByObjectId = reverseIndexMap(navigation.byObjectId);
  const occurrenceIdsByObjectIndex = reverseIndexMap(navigation.byObjectIndex);
  const occurrenceIdsByViewTarget = reverseIndexMap(navigation.byViewTarget);
  const occurrencesByPath = new Map();
  const occurrencesByObjectType = new Map();
  const occurrencesByObjectName = new Map();
  const occurrencesByFieldName = new Map();
  const occurrencesByFieldIndex = new Map();

  for (const occurrence of occurrences) {
    const path = normalizedPath(occurrence?.path);
    if (path) {
      appendMapValue(occurrencesByPath, path, occurrence);
    }
    const anchor = occurrence?.sourceAnchor || {};
    appendNormalizedValue(occurrencesByObjectType, anchor.objectType, occurrence);
    appendNormalizedValue(occurrencesByObjectName, anchor.objectName, occurrence);
    appendNormalizedValue(occurrencesByFieldName, anchor.fieldName, occurrence);
    if (hasIndex(anchor.fieldIndex)) {
      appendMapValue(occurrencesByFieldIndex, normalizedIndex(anchor.fieldIndex), occurrence);
    }
  }
  freezeMapArrays(occurrencesByPath);
  freezeMapArrays(occurrencesByObjectType);
  freezeMapArrays(occurrencesByObjectName);
  freezeMapArrays(occurrencesByFieldName);
  freezeMapArrays(occurrencesByFieldIndex);

  return Object.freeze({
    cacheKey,
    textHash: metadata.textHash,
    analysisKey: metadata.textHash,
    analyzerVersion: metadata.analyzerVersion,
    schemaVersion: metadata.schemaVersion,
    entityById,
    occurrenceById,
    occurrenceIdsByEntityId,
    occurrenceIdsByObjectId,
    occurrenceIdsByObjectIndex,
    occurrenceIdsByViewTarget,
    occurrencesByPath,
    entity(entityId) {
      return entityById.get(String(entityId || "")) || null;
    },
    occurrence(occurrenceId) {
      return occurrenceById.get(String(occurrenceId || "")) || null;
    },
    occurrenceIDs(indexName, key) {
      const index = reverseIndexForName({
        occurrenceIdsByEntityId,
        occurrenceIdsByObjectId,
        occurrenceIdsByObjectIndex,
        occurrenceIdsByViewTarget,
      }, indexName);
      return index?.get(String(key ?? "")) || EMPTY_LIST;
    },
    occurrencesForIDs(ids) {
      return (Array.isArray(ids) ? ids : EMPTY_LIST)
        .map((id) => occurrenceById.get(String(id || "")))
        .filter(Boolean);
    },
    sourceIdentityCandidates(anchor = {}) {
      return sourceIdentityCandidates(anchor, {
        occurrencesByObjectType,
        occurrencesByObjectName,
        occurrencesByFieldName,
        occurrencesByFieldIndex,
      });
    },
    nearestParentOccurrence(path) {
      return nearestParentOccurrence(path, occurrencesByPath);
    },
  });
}

function rawNavigation(source) {
  if (!source || typeof source !== "object") {
    return EMPTY_NAVIGATION;
  }
  const nested = source.navigation || source.Navigation;
  if (nested && typeof nested === "object") {
    return nested;
  }
  if (
    Array.isArray(source.entities) ||
    Array.isArray(source.occurrences) ||
    source.byEntityId ||
    source.byObjectId ||
    source.byObjectIndex ||
    source.byViewTarget
  ) {
    return source;
  }
  return EMPTY_NAVIGATION;
}

function navigationCacheMetadata(source, navigation, metadata) {
  const projection = source && typeof source === "object" && source !== navigation ? source : {};
  return Object.freeze({
    textHash: String(
      metadata.textHash || metadata.analysisKey || projection.textHash || projection.analysisKey || navigation.textHash || "",
    ),
    analyzerVersion: String(
      metadata.analyzerVersion || projection.analyzerVersion || navigation.analyzerVersion || "unknown",
    ),
    schemaVersion: String(
      metadata.schemaVersion || projection.schemaVersion || projection.schema || navigation.schemaVersion || navigation.schema || "",
    ),
  });
}

function metadataIdentityKey(metadata) {
  return `${metadata.textHash}\u0000${metadata.analyzerVersion}\u0000${metadata.schemaVersion}`;
}

function sharedCacheKey(navigation, metadata) {
  const entities = Array.isArray(navigation.entities) ? navigation.entities : EMPTY_LIST;
  const occurrences = Array.isArray(navigation.occurrences) ? navigation.occurrences : EMPTY_LIST;
  const firstEntity = entities[0]?.id || "";
  const lastEntity = entities[entities.length - 1]?.id || "";
  const firstOccurrence = occurrences[0]?.occurrenceId || "";
  const lastOccurrence = occurrences[occurrences.length - 1]?.occurrenceId || "";
  return [
    metadata.textHash,
    metadata.analyzerVersion,
    metadata.schemaVersion,
    entities.length,
    occurrences.length,
    firstEntity,
    lastEntity,
    firstOccurrence,
    lastOccurrence,
  ].join("\u0000");
}

function rememberIdentityCache(navigation, identityKey, cache, existingBucket = null) {
  const bucket = existingBucket || new Map();
  bucket.set(identityKey, cache);
  while (bucket.size > 3) {
    bucket.delete(bucket.keys().next().value);
  }
  if (!existingBucket) {
    cacheBucketsByNavigation.set(navigation, bucket);
  }
}

function touchSharedCache(cacheKey) {
  if (!sharedCachesByKey.has(cacheKey)) {
    return;
  }
  const cache = sharedCachesByKey.get(cacheKey);
  sharedCachesByKey.delete(cacheKey);
  sharedCachesByKey.set(cacheKey, cache);
}

function trimSharedCaches() {
  while (sharedCachesByKey.size > MAX_SHARED_CACHE_ENTRIES) {
    sharedCachesByKey.delete(sharedCachesByKey.keys().next().value);
  }
}

function recordsByID(records, key) {
  const result = new Map();
  for (const record of records) {
    const id = String(record?.[key] || "");
    if (id && !result.has(id)) {
      result.set(id, record);
    }
  }
  return result;
}

function reverseIndexMap(rawIndex) {
  const result = new Map();
  if (!rawIndex || typeof rawIndex !== "object") {
    return result;
  }
  for (const [key, rawIDs] of Object.entries(rawIndex)) {
    result.set(String(key), Object.freeze(
      (Array.isArray(rawIDs) ? rawIDs : EMPTY_LIST).map((id) => String(id || "")).filter(Boolean),
    ));
  }
  return result;
}

function reverseIndexForName(indexes, name) {
  switch (name) {
    case "entity": return indexes.occurrenceIdsByEntityId;
    case "object-id": return indexes.occurrenceIdsByObjectId;
    case "object-index": return indexes.occurrenceIdsByObjectIndex;
    case "view-target": return indexes.occurrenceIdsByViewTarget;
    default: return null;
  }
}

function appendNormalizedValue(map, rawKey, value) {
  const key = normalizeIdentityText(rawKey);
  if (key) {
    appendMapValue(map, key, value);
  }
}

function appendMapValue(map, key, value) {
  const values = map.get(key);
  if (values) {
    values.push(value);
  } else {
    map.set(key, [value]);
  }
}

function freezeMapArrays(map) {
  for (const [key, values] of map) {
    map.set(key, Object.freeze(values));
  }
}

function sourceIdentityCandidates(anchor, indexes) {
  const buckets = [];
  const constraints = [
    [indexes.occurrencesByObjectType, normalizeIdentityText(anchor.objectType)],
    [indexes.occurrencesByObjectName, normalizeIdentityText(anchor.objectName)],
    [indexes.occurrencesByFieldName, normalizeIdentityText(anchor.fieldName)],
  ];
  if (!anchor.fieldName && hasIndex(anchor.fieldIndex)) {
    constraints.push([indexes.occurrencesByFieldIndex, normalizedIndex(anchor.fieldIndex)]);
  }
  for (const [index, key] of constraints) {
    if (!key) {
      continue;
    }
    const bucket = index.get(key);
    if (!bucket) {
      return EMPTY_LIST;
    }
    buckets.push(bucket);
  }
  if (!buckets.length) {
    return EMPTY_LIST;
  }
  return buckets.reduce((smallest, bucket) => bucket.length < smallest.length ? bucket : smallest);
}

function nearestParentOccurrence(rawPath, occurrencesByPath) {
  let path = normalizedPath(rawPath);
  while (path) {
    const separator = path.lastIndexOf("/");
    if (separator < 0) {
      return null;
    }
    path = path.slice(0, separator);
    const candidates = occurrencesByPath.get(path);
    if (candidates?.length) {
      return [...candidates].sort((left, right) => (
        String(left?.occurrenceId || "").localeCompare(String(right?.occurrenceId || ""))
      ))[0] || null;
    }
  }
  return null;
}

function normalizedPath(value) {
  return String(value || "").replace(/\/+$/, "");
}

function normalizeIdentityText(value) {
  return String(value || "").trim().toLowerCase().replace(/\s+/g, " ");
}

function normalizedIndex(value) {
  const number = Number(value);
  return Number.isFinite(number) ? String(number) : String(value ?? "");
}

function hasIndex(value) {
  return value !== undefined && value !== null && String(value) !== "";
}
