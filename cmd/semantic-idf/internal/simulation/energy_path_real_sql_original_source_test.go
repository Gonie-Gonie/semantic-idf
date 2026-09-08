package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// These are disjoint original SQL identities. A Tabular cell never receives a
// fabricated ReportDataDictionary index or a synthetic monthly observation.
type epathSQLOriginalSource struct {
	RDD        *epathRealSQLSource
	Tabular    *epathSQLTabularObservation
	Name, Key  string // Independent reviewed wire alias, only for Tabular.
	site       epathRealSQLSite
	LoadDetail *epathSQLLoadDetailIdentity       // Non-additive, exact equipment-to-Zone context only.
	DirectHVAC *epathSQLDirectHVACSourceIdentity // Additive model-total coil observation, exact reviewed owner.
	NativeVRF  *epathSQLVRFSourceIdentity        // Original native local/shared constituent, not a PTAC/PTHP cohort.
}

func epathSQLOriginalRDD(source epathRealSQLSource) epathSQLOriginalSource {
	return epathSQLOriginalSource{RDD: &source}
}

func epathSQLOriginalLoadDetail(detail epathSQLLoadDetailIdentity) (epathSQLOriginalSource, error) {
	if err := epathSQLValidateLoadDetailIdentity(detail); err != nil {
		return epathSQLOriginalSource{}, err
	}
	proof := epathSQLOriginalRDD(detail.Source)
	proof.LoadDetail = &detail
	return proof, nil
}

func epathSQLOriginalDirectHVAC(identity epathSQLDirectHVACSourceIdentity) (epathSQLOriginalSource, error) {
	if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
		return epathSQLOriginalSource{}, err
	}
	proof := epathSQLOriginalRDD(identity.Source)
	proof.DirectHVAC = &identity
	return proof, nil
}

func epathSQLOriginalVRF(identity epathSQLVRFSourceIdentity) (epathSQLOriginalSource, error) {
	if err := epathSQLValidateVRFSourceIdentity(identity); err != nil {
		return epathSQLOriginalSource{}, err
	}
	proof := epathSQLOriginalRDD(identity.Observation.Source)
	proof.NativeVRF = &identity
	return proof, nil
}

// Literal reviewed taxonomy, not the production meter classifier. The exact
// original column remains part of identity even when a carrier has aliases.
func epathSQLTabularSiteAlias(site epathRealSQLSite) (string, error) {
	if site.Tabular == nil || epathValidateSQLSiteSource(site) != nil || site.ID == "" {
		return "", fmt.Errorf("Tabular alias requires one reviewed site declaration")
	}
	selector := *site.Tabular
	columns := map[string]struct{ carrier, meter string }{
		"Electricity":            {"electricity", "Electricity"},
		"Natural Gas":            {"natural_gas", "NaturalGas"},
		"District Cooling":       {"district_cooling", "DistrictCooling"},
		"District Heating Water": {"district_heating", "DistrictHeating"},
	}
	column, ok := columns[selector.ColumnName]
	if !ok || column.carrier != site.Carrier {
		return "", fmt.Errorf("unreviewed or contradictory annual site carrier column")
	}
	if site.Facility {
		if site.EndUse != "" || selector.RowName != "Total End Uses" {
			return "", fmt.Errorf("facility Tabular source is not the exact total row")
		}
		return column.meter + ":Facility", nil
	}
	rows := map[string]struct{ endUse, meter string }{
		"Cooling": {"cooling", "Cooling"}, "Heating": {"heating", "Heating"},
		"Interior Lighting": {"lighting", "InteriorLights"}, "Interior Equipment": {"equipment", "InteriorEquipment"},
	}
	row, ok := rows[selector.RowName]
	if !ok || row.endUse != site.EndUse {
		return "", fmt.Errorf("unreviewed or contradictory annual site end-use row")
	}
	return row.meter + ":" + column.meter, nil
}

func epathSQLOriginalTabular(site epathRealSQLSite, observation epathSQLTabularObservation) (epathSQLOriginalSource, error) {
	alias, err := epathSQLTabularSiteAlias(site)
	if err != nil {
		return epathSQLOriginalSource{}, err
	}
	if *site.Tabular != observation.Selector {
		return epathSQLOriginalSource{}, fmt.Errorf("Tabular observation is not the declared original cell")
	}
	proof := epathSQLOriginalSource{Tabular: &observation, Name: alias, Key: alias, site: site}
	if _, err := epathSQLOriginalKey(proof); err != nil {
		return epathSQLOriginalSource{}, err
	}
	return proof, nil
}

func epathSQLOriginalKey(proof epathSQLOriginalSource) (string, error) {
	if (proof.RDD == nil) == (proof.Tabular == nil) {
		return "", fmt.Errorf("original source must be exactly one RDD or Tabular identity")
	}
	if source := proof.RDD; source != nil {
		if source.DictionaryIndex <= 0 || source.Name == "" || source.SourceUnit == "" || source.ReportingFrequency == "" || proof.Name != "" || proof.Key != "" {
			return "", fmt.Errorf("invalid original RDD identity")
		}
		if proof.LoadDetail != nil && (epathSQLValidateLoadDetailIdentity(*proof.LoadDetail) != nil || !reflect.DeepEqual(*source, proof.LoadDetail.Source)) {
			return "", fmt.Errorf("non-additive load detail lost its exact original RDD binding")
		}
		if proof.DirectHVAC != nil && (proof.LoadDetail != nil || epathSQLValidateDirectHVACSourceIdentity(*proof.DirectHVAC) != nil || !reflect.DeepEqual(*source, proof.DirectHVAC.Source)) {
			return "", fmt.Errorf("direct HVAC source lost its exact original RDD/model-total owner binding")
		}
		if proof.NativeVRF != nil && (proof.LoadDetail != nil || proof.DirectHVAC != nil || epathSQLValidateVRFSourceIdentity(*proof.NativeVRF) != nil || !reflect.DeepEqual(*source, proof.NativeVRF.Observation.Source)) {
			return "", fmt.Errorf("native VRF source lost its exact original RDD/shared-system owner binding")
		}
		return fmt.Sprintf("sql-rdd-%d", source.DictionaryIndex), nil
	}
	if proof.LoadDetail != nil || proof.DirectHVAC != nil || proof.NativeVRF != nil {
		return "", fmt.Errorf("a Tabular cell cannot claim equipment load-detail identity")
	}
	observation := proof.Tabular
	if err := epathValidateSQLTabularObservation(*observation); err != nil {
		return "", err
	}
	alias, err := epathSQLTabularSiteAlias(proof.site)
	if err != nil || proof.site.Tabular == nil || *proof.site.Tabular != observation.Selector || proof.Name != alias || proof.Key != alias {
		return "", fmt.Errorf("missing exact reviewed Tabular wire alias")
	}
	selector := observation.Selector
	// Report/facility/index are independently verified by the original reader;
	// they are not invented as fields on the narrower canonical wire source.
	identity, _ := json.Marshal([]string{selector.ReportName, selector.ReportForString, selector.TableName, selector.RowName, selector.ColumnName, selector.Unit})
	return "sql-tabular:" + string(identity), nil
}

func epathSQLOriginalSourceMatches(source EnergyDataSource, proof epathSQLOriginalSource, period string) bool {
	if !epathOracleValidPeriod(period) {
		return false
	}
	if _, err := epathSQLOriginalKey(proof); err != nil {
		return false
	}
	if original := proof.RDD; original != nil {
		if proof.NativeVRF != nil {
			return epathSQLMatchVRFSource(source, proof.NativeVRF.Observation) == nil
		}
		if proof.DirectHVAC != nil {
			return epathSQLMatchDirectHVACSource(source, *proof.DirectHVAC) == nil
		}
		if proof.LoadDetail != nil {
			return epathSQLLoadDetailSourceMatches(source, *proof.LoadDetail)
		}
		return source.SourceType == "sql_report_data" && source.IsMeter == original.IsMeter && strings.EqualFold(source.Name, original.Name) && strings.EqualFold(source.KeyValue, original.KeyValue) && source.SourceUnit == original.SourceUnit && source.NormalizedUnit == "kWh" && strings.EqualFold(source.ReportingFrequency, original.ReportingFrequency)
	}
	selector := proof.Tabular.Selector
	return period == "annual" && source.SourceType == "sql_tabular" && source.IsMeter && source.Name == proof.Name && source.KeyValue == proof.Key && source.Units == selector.Unit && source.SourceUnit == selector.Unit && source.NormalizedUnit == "kWh" && source.ReportingFrequency == "Annual" && source.AggregationMethod == "tabular_annual_value" && source.AggregationBasis == "model_total" && source.ZoneName == "" && source.TableName == selector.TableName && source.RowName == selector.RowName && source.ColumnName == selector.ColumnName+" ["+selector.Unit+"]"
}

func epathSQLValidateOriginalSources(allowed map[string]epathSQLOriginalSource) error {
	for key, proof := range allowed {
		identity, err := epathSQLOriginalKey(proof)
		if err != nil || identity != key {
			return fmt.Errorf("invalid independently allowed original source %s: %v", key, err)
		}
	}
	return nil
}

// Recursive path preserves the previous quality proof's metadata-based RDD
// matching. Original keys, not candidate-derived names/IDs, are returned.
func epathSQLOriginalSourceLeaves(ids []string, actual map[string]EnergyDataSource, allowed map[string]epathSQLOriginalSource, period string) (map[string]bool, error) {
	if len(ids) == 0 || len(allowed) == 0 || !epathOracleValidPeriod(period) {
		return nil, fmt.Errorf("trace lacks original source/context evidence")
	}
	if err := epathSQLValidateOriginalSources(allowed); err != nil {
		return nil, err
	}
	leaves, visiting, visited := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("cyclic original source inputs")
		}
		if visited[id] {
			return nil
		}
		source, exists := actual[id]
		if !exists || id == "" || source.ID != id {
			return fmt.Errorf("missing original source reference %s", id)
		}
		if original, bound := allowed[id]; bound && (original.LoadDetail != nil || original.DirectHVAC != nil || original.NativeVRF != nil) && !epathSQLOriginalSourceMatches(source, original, period) {
			return fmt.Errorf("original equipment source cannot masquerade as a derived or ordinary source")
		}
		for _, original := range allowed {
			if original.NativeVRF != nil && strings.EqualFold(source.Name, original.RDD.Name) && strings.EqualFold(source.KeyValue, original.RDD.KeyValue) &&
				strings.EqualFold(source.ReportingFrequency, original.RDD.ReportingFrequency) && !epathSQLOriginalSourceMatches(source, original, period) {
				return fmt.Errorf("original VRF observation cannot acquire a different ID or derived wrapper")
			}
		}
		visiting[id] = true
		if len(source.InputSourceIDs) > 0 {
			// An original annual cell cannot masquerade as a derived monthly
			// wrapper to evade the exact temporal/source-type contract.
			if source.SourceType == "sql_tabular" {
				return fmt.Errorf("original Tabular source has derived inputs")
			}
			for _, input := range source.InputSourceIDs {
				if err := visit(input); err != nil {
					return err
				}
			}
		} else {
			count := 0
			for key, proof := range allowed {
				if epathSQLOriginalSourceMatches(source, proof, period) {
					leaves[key], count = true, count+1
				}
			}
			if count != 1 {
				return fmt.Errorf("source %s has %d permitted original identities", id, count)
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return leaves, nil
}

// Direct node/flow traces retain the stricter existing sql-rdd-N wire identity
// requirement. A Tabular wire ID is opaque; its exact original tuple is not.
func epathSQLVerifyOriginalSources(ids []string, actual map[string]EnergyDataSource, allowed map[string]epathSQLOriginalSource, required map[string]bool, period string) error {
	if !epathOracleValidPeriod(period) {
		return fmt.Errorf("invalid original source context")
	}
	if err := epathSQLValidateOriginalSources(allowed); err != nil {
		return err
	}
	seen, observed := map[string]bool{}, map[string]bool{}
	for _, id := range ids {
		source, exists := actual[id]
		if !exists || id == "" || source.ID != id || seen[id] || len(source.InputSourceIDs) != 0 {
			return fmt.Errorf("missing/duplicate/non-original direct SQL source %s", id)
		}
		seen[id] = true
		count, selected := 0, ""
		for key, proof := range allowed {
			if proof.RDD != nil && id != key {
				continue
			}
			if epathSQLOriginalSourceMatches(source, proof, period) {
				count++
				selected = key
			}
		}
		if count != 1 || observed[selected] {
			return fmt.Errorf("direct source %s has ambiguous/duplicate original ownership", id)
		}
		observed[selected] = true
	}
	for key, mustExist := range required {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("required original source has no independent proof")
		}
		if mustExist && !observed[key] {
			return fmt.Errorf("direct trace lost required original SQL source %s", key)
		}
	}
	return nil
}
