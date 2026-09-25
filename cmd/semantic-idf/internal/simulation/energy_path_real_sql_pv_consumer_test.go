package simulation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

func epathSQLPVPersistedSourceValue(value float64, quantity epathSQLQuantity) error {
	if !epathOracleFinite(value) || !quantity.valid() || value != math.Round(value*1000)/1000 {
		return fmt.Errorf("PV source scalar is not a finite signed3dp observation")
	}
	lo, hi := quantity.bounds()
	// Quantized endpoints: generic relative slack must not admit an extra .001.
	if value < lo || value > hi {
		return fmt.Errorf("PV source %.12g outside native once-serialized interval [%.12g, %.12g]", value, lo, hi)
	}
	return nil
}

func epathSQLCheckPVSourceConsumer(bundle PurposeResultBundle, check epathSQLModelCheck, prepared epathSQLPVValidatedSources) error {
	if check.PVSource == nil {
		return fmt.Errorf("required PV source has no independent typed proof")
	}
	canonical, found := prepared.canonical[check.Want.Key]
	if !found || !reflect.DeepEqual(check, canonical) {
		return fmt.Errorf("PV source row escaped its exact native selector/proof/quantity")
	}
	identity, found := prepared.identities[check.PVSource.DictionaryIndex]
	if !found {
		return fmt.Errorf("PV source lost its boundary-validated actual dictionary")
	}
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || result.Scope.ZoneName != "" {
		return fmt.Errorf("PV native source scalar requires the original Building wrapper")
	}
	id := fmt.Sprintf("sql-rdd-%d", identity.DictionaryIndex)
	seen := map[string]bool{}
	count := 0
	for _, actual := range result.Sources {
		if actual.ID == "" || seen[actual.ID] {
			return fmt.Errorf("original source list has an empty or duplicated actual ID")
		}
		seen[actual.ID] = true
		sameSelector := strings.EqualFold(actual.Name, identity.Name) && strings.EqualFold(actual.KeyValue, identity.Key) && strings.EqualFold(actual.ReportingFrequency, identity.Frequency)
		if actual.ID != id && !sameSelector {
			continue
		}
		count++
		if actual.ID != id || actual.SourceType != "sql_report_data" || actual.Name != identity.Name || actual.KeyValue != identity.Key || actual.ReportingFrequency != identity.Frequency || actual.IsMeter != identity.IsMeter ||
			actual.Units != "J" || actual.SourceUnit != "J" || actual.NormalizedUnit != "kWh" || actual.IndexGroup != identity.IndexGroup || actual.AggregationMethod != "sum_report_data" ||
			actual.ZoneName != "" || actual.AggregationBasis != "model_total" || actual.EffectiveMultiplier != 1 || actual.MultiplierApplication != "already_model_total" ||
			len(actual.InputSourceIDs) != 0 || actual.Formula != "" {
			return fmt.Errorf("PV source changed its exact native ID/metadata/model-total factor-one observation")
		}
		// Request navigation is distinct from separately bound physical owners.
		if !reflect.DeepEqual(actual.ObjectIndex, identity.OutputObjectIndex) {
			return fmt.Errorf("PV source borrowed a physical/executed or wrong-frequency Output index")
		}
		presence := actual.observedValuePresence
		if actual.inspectorDecodedFromJSON {
			presence = actual.inspectorValuePresence
		}
		if presence&3 != 3 {
			return fmt.Errorf("PV raw/effective observation is missing/null, not known or rounded zero")
		}
		if err := epathSQLPVPersistedSourceValue(actual.RawValue, identity.Quantity); err != nil {
			return err
		}
		if err := epathSQLPVPersistedSourceValue(actual.EffectiveValue, identity.Quantity); err != nil {
			return err
		}
		if actual.RawValue != actual.EffectiveValue {
			return fmt.Errorf("PV native model-total source applied an extra multiplier")
		}
	}
	if count != 1 {
		return fmt.Errorf("PV native source requires exactly one persisted observation including measured zero")
	}
	// This source-only consumer does not infer flow/Zone allocation permission,
	// electrical balance, chart samples, or ancillary parent closure from Role.
	return nil
}
