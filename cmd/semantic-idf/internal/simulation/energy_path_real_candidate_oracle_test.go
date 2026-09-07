package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

func epathOracleValidPeriod(period string) bool {
	if period == "annual" {
		return true
	}
	var month int
	_, err := fmt.Sscanf(period, "M%d", &month)
	return err == nil && month >= 1 && month <= 12 && period == fmt.Sprintf("M%d", month)
}

func epathValidateOracleMetricIdentity(metric epathRealOracleMetric) error {
	if (metric.Scope != "building" && metric.Scope != "zone") || (metric.Scope == "zone") != (strings.TrimSpace(metric.Zone) != "") || !epathOracleValidPeriod(metric.Period) || metric.Unit == "" {
		return fmt.Errorf("invalid metric scope/Zone/period/unit %s", metric.Key)
	}
	if (metric.Found == nil) != (metric.Total == nil) {
		return fmt.Errorf("metric %s has unmatched count presence", metric.Key)
	}
	if metric.Found != nil && (*metric.Found < 0 || *metric.Total < 0 || *metric.Found > *metric.Total) {
		return fmt.Errorf("metric %s has invalid counts", metric.Key)
	}
	if metric.Unit == "%" && metric.Value != nil && (!epathOracleFinite(*metric.Value) || *metric.Value < 0 || *metric.Value > 100) {
		return fmt.Errorf("metric %s has invalid percentage", metric.Key)
	}
	return nil
}

func epathValidateOracleGraphRecords(nodes []EnergyExplanationNode, links []EnergyPathLink, rows []EnergyReconciliation, scope, zone, period string, knownZones ...string) error {
	byID := map[string]EnergyExplanationNode{}
	validContext := func(recordPeriod, recordZone string, allocationContext bool) bool {
		if recordPeriod != "" && recordPeriod != period {
			return false
		}
		if recordPeriod == "" && period != "annual" {
			return false
		}
		if scope == "building" {
			if recordZone == "" {
				return true
			}
			// A Building aggregate can retain exact contributing-Zone provenance.
			// That is not a Zone-filtered result; validate its declared owner.
			for _, known := range knownZones {
				if strings.EqualFold(known, recordZone) {
					return true
				}
			}
			return false
		}
		return strings.EqualFold(recordZone, zone) || allocationContext && recordZone == ""
	}
	for _, node := range nodes {
		if node.ID == "" {
			return fmt.Errorf("empty candidate node ID")
		}
		if _, ok := byID[node.ID]; ok {
			return fmt.Errorf("duplicate candidate node ID %q", node.ID)
		}
		if !validContext(node.Period, node.ZoneName, false) {
			return fmt.Errorf("candidate node %s has conflicting scope/period", node.ID)
		}
		byID[node.ID] = node
	}
	seen := map[string]bool{}
	for _, link := range links {
		if link.ID == "" || seen[link.ID] {
			return fmt.Errorf("empty/duplicate candidate link ID %q", link.ID)
		}
		seen[link.ID] = true
		if !validContext(link.Period, link.ZoneName, false) {
			return fmt.Errorf("candidate link %s has conflicting scope/period", link.ID)
		}
		if _, ok := byID[link.FromID]; !ok {
			return fmt.Errorf("candidate link %s has missing source endpoint", link.ID)
		}
		if _, ok := byID[link.ToID]; !ok {
			return fmt.Errorf("candidate link %s has missing target endpoint", link.ID)
		}
	}
	seen = map[string]bool{}
	for _, row := range rows {
		if row.ID == "" || seen[row.ID] {
			return fmt.Errorf("empty/duplicate reconciliation ID %q", row.ID)
		}
		seen[row.ID] = true
		// Allocation rows copied into a Zone describe Building-wide coverage,
		// unlike the Zone's physical graph. Their own period remains exact.
		if !validContext(row.Period, row.ZoneName, row.Level == "allocation") {
			return fmt.Errorf("reconciliation %s has conflicting scope/period", row.ID)
		}
	}
	return nil
}

func epathValidateOracleTarget(target epathRealOracleTarget, unit string) error {
	allowed := map[string]string{
		"nodes":          " id collection field level category service carrier basis kind unit scaleDomain aggregationBasis thermalComponent component aggregate allowPrunedZero ",
		"sources":        " collection field sourceName sourceKey sourceUnit frequency unit ",
		"links":          " id collection field relation service basis fromId toId fromUnit toUnit ratioKind aggregate ",
		"reconciliation": " id collection field level service basis unit allocationMethod status ",
		"quality":        " collection field ",
	}
	fields := map[string]json.RawMessage{}
	data, _ := json.Marshal(target)
	_ = json.Unmarshal(data, &fields)
	for field := range fields {
		if !strings.Contains(allowed[target.Collection], " "+field+" ") {
			return fmt.Errorf("unsupported %s selector property %s", target.Collection, field)
		}
	}
	if target.Aggregate != "" && target.Aggregate != "sum" {
		return fmt.Errorf("unknown candidate aggregate %q", target.Aggregate)
	}
	if target.ID != "" && target.Aggregate != "" {
		return fmt.Errorf("exact candidate ID cannot request aggregation")
	}
	validFields := map[string]string{"nodes": " value rawValue effectiveValue allocatedValue loadBreakdown ", "sources": " rawValue effectiveValue ", "links": " fromValue toValue pairedRatio ", "reconciliation": " expectedValue explainedValue residualValue directValue allocatedValue unassignedValue overmappedValue ", "quality": " drivers loads endUses carriers ratios driverToLoadClosedPct endUseToCarrierClosedPct zoneAllocatedPct unassignedPct "}
	if !strings.Contains(validFields[target.Collection], " "+target.Field+" ") {
		return fmt.Errorf("unknown candidate field %s/%s", target.Collection, target.Field)
	}
	switch target.Collection {
	case "nodes":
		if target.Level == "" || target.Unit == "" || target.Unit != unit || target.ScaleDomain == "" {
			return fmt.Errorf("node target requires explicit matching unit and scale domain")
		}
		specificCategory := target.Category != "" || target.Level == "load" && (target.Service == "cooling" || target.Service == "heating")
		if target.AllowPrunedZero && (target.Field != "value" && target.Field != "allocatedValue" || !specificCategory || target.Basis == "") {
			return fmt.Errorf("pruned zero requires a specific presentation category/basis/contribution field")
		}
		if target.Field == "loadBreakdown" && (target.Level != "load" || target.Component != "sensible" && target.Component != "latent") {
			return fmt.Errorf("exact sensible/latent Load component required")
		}
	case "sources":
		if target.SourceName == "" || target.SourceUnit == "" || target.Frequency == "" || target.Unit == "" || target.Unit != unit {
			return fmt.Errorf("exact original source identity/unit required")
		}
	case "links":
		if target.Relation == "" || target.FromUnit == "" || target.ToUnit == "" {
			return fmt.Errorf("link target requires exact relation and paired units")
		}
		if target.Field == "fromValue" && unit != target.FromUnit || target.Field == "toValue" && unit != target.ToUnit {
			return fmt.Errorf("link metric unit contradicts selected endpoint")
		}
		if target.Field == "pairedRatio" && (target.Relation != "load_to_end_use" || target.RatioKind == "" || target.Service == "" || unit != "ratio") {
			return fmt.Errorf("paired ratio requires conversion relation, service, kind and ratio unit")
		}
	case "reconciliation":
		if target.ID == "" || target.Level == "" || target.Unit == "" || target.Unit != unit {
			return fmt.Errorf("reconciliation target requires exact ID/level/unit")
		}
	case "quality":
		if unit != "%" && unit != "count" {
			return fmt.Errorf("quality unit must be percent or count")
		}
	}
	return nil
}

func epathReadOraclePairedRatio(nodes []EnergyExplanationNode, links []EnergyPathLink, sources []EnergyDataSource, target epathRealOracleTarget) (*float64, error) {
	byID := map[string]EnergyExplanationNode{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	sourceIDs := map[string]bool{}
	for _, source := range sources {
		if source.ID == "" || sourceIDs[source.ID] {
			return nil, fmt.Errorf("ambiguous ratio source identity")
		}
		sourceIDs[source.ID] = true
	}
	numerator, denominator, count := 0.0, 0.0, 0
	for _, link := range links {
		if target.ID != "" && link.ID != target.ID || target.FromID != "" && link.FromID != target.FromID || target.ToID != "" && link.ToID != target.ToID || link.Relation != target.Relation || link.ServiceKind != target.Service || target.Basis != "" && link.Basis != target.Basis {
			continue
		}
		from, to := byID[link.FromID], byID[link.ToID]
		if from.Level != "load" || to.Level != "end_use" || from.ScaleDomain != "thermal" || to.ScaleDomain != "site" || from.ServiceKind != target.Service || to.ServiceKind != target.Service || !strings.EqualFold(from.ZoneName, to.ZoneName) || from.Period != to.Period {
			return nil, fmt.Errorf("invalid paired ratio endpoint context %s", link.ID)
		}
		if link.FromUnit != target.FromUnit || link.ToUnit != target.ToUnit || link.RatioKind != target.RatioKind || from.Unit != link.FromUnit || to.Unit != link.ToUnit {
			return nil, fmt.Errorf("paired ratio unit/kind mismatch %s", link.ID)
		}
		if len(link.SourceIDs) == 0 {
			return nil, fmt.Errorf("ratio lacks source trace")
		}
		for _, id := range link.SourceIDs {
			if !sourceIDs[id] {
				return nil, fmt.Errorf("ratio refers to missing source %s", id)
			}
		}
		if !epathOracleFinite(link.FromValue) || !epathOracleFinite(link.ToValue) || link.FromValue < 0 || link.ToValue < 0 {
			return nil, fmt.Errorf("invalid paired ratio quantities")
		}
		if link.ToValue == 0 {
			return nil, nil
		}
		if err := epathCompareOracleNumber(epathOracleNumber(link.Ratio), epathOracleNumber(link.FromValue/link.ToValue), .00051, 1e-8); err != nil {
			return nil, fmt.Errorf("stored ratio does not match original paired quantities: %w", err)
		}
		numerator += link.FromValue
		denominator += link.ToValue
		count++
	}
	if count == 0 {
		return nil, nil
	}
	if count > 1 && (target.ID != "" || target.Aggregate != "sum") {
		return nil, fmt.Errorf("ambiguous ratio selection")
	}
	if denominator <= 0 {
		return nil, nil
	}
	return epathOracleNumber(numerator / denominator), nil
}

func epathReadOracleSourceCandidate(sources []EnergyDataSource, item epathRealOracleMetricRecipe) (*float64, error) {
	if item.Period != "annual" {
		return nil, fmt.Errorf("root source scalar has no monthly period; annual fallback is forbidden")
	}
	target := item.Target
	var selected *EnergyDataSource
	for _, source := range sources {
		if !strings.EqualFold(source.Name, target.SourceName) || !strings.EqualFold(source.KeyValue, target.SourceKey) || !strings.EqualFold(source.ReportingFrequency, target.Frequency) {
			continue
		}
		if source.SourceUnit != target.SourceUnit || source.NormalizedUnit != target.Unit {
			return nil, fmt.Errorf("source %s/%s has wrong source/normalized units", source.Name, source.KeyValue)
		}
		if selected != nil {
			return nil, fmt.Errorf("ambiguous original source identity")
		}
		copy := source
		selected = &copy
	}
	if selected == nil {
		return nil, nil
	}
	value, bit := selected.RawValue, uint8(1)
	if target.Field == "effectiveValue" {
		value, bit = selected.EffectiveValue, 2
	}
	decoded, presence := selected.inspectorDecodedFromJSON, selected.inspectorValuePresence
	known := selected.DriverRole == "main_flow" && selected.DriverCategory != "" && selected.MultiplierApplication != "" && selected.EffectiveMultiplier > 0
	if item.Scope == "zone" {
		found := false
		for _, detail := range selected.ScopeDetails {
			if !strings.EqualFold(detail.Scope.ZoneName, item.Zone) {
				continue
			}
			if found || detail.Scope.Kind != "zone" {
				return nil, fmt.Errorf("ambiguous source scope detail")
			}
			found = true
			value = detail.RawValue
			if target.Field == "effectiveValue" {
				value = detail.EffectiveValue
			}
			decoded, presence = detail.inspectorDecodedFromJSON, detail.inspectorValuePresence
			known = detail.MultiplierApplication != "" && detail.EffectiveMultiplier > 0
		}
		if !found {
			return nil, nil
		}
	}
	if decoded && presence&bit == 0 || !decoded && value == 0 && !known {
		return nil, nil
	}
	if !epathOracleFinite(value) {
		return nil, fmt.Errorf("invalid original source numeric value")
	}
	return epathOracleNumber(value), nil
}

func epathReadStrictOracleQuality(nodes []EnergyExplanationNode, rows []EnergyReconciliation, quality *EnergyPathQuality, target epathRealOracleTarget, want epathRealOracleMetric) (*float64, error) {
	if quality == nil {
		return nil, nil
	}
	levels := map[string]EnergyCompletenessLevel{"drivers": quality.Drivers, "loads": quality.Loads, "endUses": quality.EndUses, "carriers": quality.Carriers, "ratios": quality.Ratios}
	if level, ok := levels[target.Field]; ok {
		if level.Found < 0 || level.Total < 0 || level.Found > level.Total || !strings.Contains(" complete partial missing unavailable not_requested not_applicable ", " "+level.Status+" ") {
			return nil, fmt.Errorf("invalid quality count/status %s", target.Field)
		}
		if (want.Found == nil) != (want.Total == nil) {
			return nil, fmt.Errorf("incomplete expected quality counts")
		}
		if want.Status != "" && want.Status != level.Status {
			return nil, fmt.Errorf("quality status %s want %s", level.Status, want.Status)
		}
		if want.Found != nil && (*want.Found != level.Found || *want.Total != level.Total) {
			return nil, fmt.Errorf("quality counts %d/%d want %d/%d", level.Found, level.Total, *want.Found, *want.Total)
		}
		if level.Total == 0 || level.Status == "unavailable" || level.Status == "not_requested" || level.Status == "not_applicable" {
			return nil, nil
		}
		if want.Unit == "count" {
			return epathOracleNumber(float64(level.Found)), nil
		}
		return epathOracleNumber(float64(level.Found) * 100 / float64(level.Total)), nil
	}
	value, status, denominator := 0.0, "", 0.0
	switch target.Field {
	case "driverToLoadClosedPct":
		value, status = quality.DriverToLoadClosedPct, quality.DriverToLoadStatus
		for _, node := range nodes {
			if node.Level == "load" && node.ScaleDomain == "thermal" && epathOracleFinite(node.Value) && node.Value > 0 {
				denominator += node.Value
			}
		}
	case "endUseToCarrierClosedPct":
		value, status = quality.EndUseToCarrierClosedPct, quality.EndUseToCarrierStatus
		for _, node := range nodes {
			if node.Level == "carrier" && node.ScaleDomain == "site" && node.ZoneName == "" && (node.MeterHierarchyLevel == "facility_total" || node.Basis == "reported_meter" || node.Basis == "reported_variable" || node.Basis == "integrated_rate") && node.MeterHierarchyLevel != "observed_end_use_subtotal" && node.MeterHierarchyLevel != "zone_direct_subtotal" && epathOracleFinite(node.Value) && node.Value > 0 {
				denominator += node.Value
			}
		}
	case "zoneAllocatedPct", "unassignedPct":
		value, status = quality.ZoneAllocatedPct, quality.ZoneAllocationStatus
		if target.Field == "unassignedPct" {
			value = quality.UnassignedPct
		}
		for _, row := range rows {
			if row.Level == "allocation" && row.Period == want.Period && epathOracleFinite(row.ExpectedValue) && row.ExpectedValue > 0 {
				denominator += row.ExpectedValue
			}
		}
	default:
		return nil, fmt.Errorf("unknown quality metric")
	}
	if !strings.Contains(" complete partial overmapped unavailable not_requested not_applicable ", " "+status+" ") {
		return nil, fmt.Errorf("unknown quality accounting status %q", status)
	}
	if want.Status != "" && want.Status != status {
		return nil, fmt.Errorf("accounting status %s want %s", status, want.Status)
	}
	if denominator <= 0 || status == "unavailable" || status == "not_requested" || status == "not_applicable" {
		return nil, nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		return nil, fmt.Errorf("invalid accounting percentage")
	}
	return epathOracleNumber(value), nil
}
