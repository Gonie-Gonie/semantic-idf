package simulation

// Supplemental native global-observation shape, not a graph/supply permission.
// Call only with independently bound original/native RDD identity. This helper
// neither recognizes a source from its presentation marker nor repairs it.
import (
	"fmt"
	"strings"
)

func epathSQLCheckPVNativeSourceShape(bundle PurposeResultBundle, native epathRealSQLSource) error {
	if native.DictionaryIndex <= 0 || native.Name == "" || native.SourceUnit != "J" || native.ReportingFrequency != "Monthly" && native.ReportingFrequency != "Hourly" ||
		native.IsMeter && native.KeyValue != "" || !native.IsMeter && strings.TrimSpace(native.KeyValue) == "" {
		return fmt.Errorf("PV source shape lacks its exact independent native M/H identity")
	}
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || result.Scope.ZoneName != "" {
		return fmt.Errorf("PV source shape requires original global Building wrapper")
	}
	id := fmt.Sprintf("sql-rdd-%d", native.DictionaryIndex)
	seen := map[string]bool{}
	count := 0
	for _, actual := range result.Sources {
		if actual.ID == "" || seen[actual.ID] {
			return fmt.Errorf("PV source shape encountered missing/duplicate native ID")
		}
		seen[actual.ID] = true
		sameSelector := strings.EqualFold(actual.Name, native.Name) && strings.EqualFold(actual.KeyValue, native.KeyValue) && strings.EqualFold(actual.ReportingFrequency, native.ReportingFrequency)
		if actual.ID != id && !sameSelector {
			continue
		}
		count++
		if actual.ID != id || actual.SourceType != "sql_report_data" || actual.Name != native.Name || actual.KeyValue != native.KeyValue || actual.IsMeter != native.IsMeter || actual.ReportingFrequency != native.ReportingFrequency ||
			actual.SourceUnit != "J" || actual.Units != "J" || actual.NormalizedUnit != "kWh" || actual.AggregationMethod != "sum_report_data" || actual.IndexGroup != native.IndexGroup {
			return fmt.Errorf("PV source shape borrowed another native dictionary or metadata")
		}
		// Native component totals, Facility meters and the independently proved
		// Cogeneration parent are global observations, not representative Zones,
		// derived allocation sources, or per-scope restatements of that quantity.
		if actual.ZoneName != "" || len(actual.ScopeDetails) != 0 || actual.AggregationBasis != "model_total" || actual.EffectiveMultiplier != 1 || actual.MultiplierApplication != "already_model_total" {
			return fmt.Errorf("PV native global source claimed Zone/scope or incorrect model-total basis/multiplier")
		}
		if actual.AllocationApplied || actual.AllocatedValue != 0 || actual.AllocationFactor != 0 || actual.AllocationExplanation != "" || actual.AllocationFormula != "" {
			return fmt.Errorf("PV native global observation claimed an allocation instead of its measured quantity")
		}
		if actual.Formula != "" || len(actual.InputSourceIDs) != 0 || len(actual.RelatedEntityIDs) != 0 {
			return fmt.Errorf("PV native global source claimed derived/related ownership outside its independent identity")
		}
	}
	if count != 1 {
		return fmt.Errorf("PV native global source shape requires exactly one original source")
	}
	// Raw/effective presence, native scalar bounds, Output request navigation,
	// native calendar and electrical/Zone flow are separate mandatory proofs.
	return nil
}

// Core adapter: retain the frozen scalar/identity consumer, then add shape.
// This replaces its call at evaluate AND coverage entry; it adds no metric,
// registry exception, native row rescan or source-only graph authorization.
func epathSQLCheckPVSourceConsumerWithShape(bundle PurposeResultBundle, check epathSQLModelCheck, prepared epathSQLPVValidatedSources) error {
	if err := epathSQLCheckPVSourceConsumer(bundle, check, prepared); err != nil {
		return err
	}
	identity, found := prepared.identities[check.PVSource.DictionaryIndex]
	if !found {
		return fmt.Errorf("PV source shape lost the already validated core identity")
	}
	native := epathRealSQLSource{DictionaryIndex: identity.DictionaryIndex, Name: identity.Name, KeyValue: identity.Key, IsMeter: identity.IsMeter, SourceUnit: "J", ReportingFrequency: identity.Frequency, IndexGroup: identity.IndexGroup}
	return epathSQLCheckPVNativeSourceShape(bundle, native)
}
