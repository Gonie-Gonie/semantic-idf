package simulation

// Independent acceptance support. The native compiler proves original/executed/request/SQL rows.
// This consumer independently checks their persisted source records, without
// repairing a candidate or using its scalars to compile an expectation.
import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strings"
)

type epathSQLPoolSourceProof struct {
	Identity epathSQLPoolSourceIdentity
	Weather  epathRealSQLWeather
	Original epathSQLPoolOriginalProof
	Field    string
}

// Root integration adds these to the ordinary check registry and attaches
// Proof to its future PoolSource field. No new selector/zero exemption is used.
type epathSQLPoolSourceRequirement struct {
	Group, Scope, Zone, Period, Key, Unit string
	Quantity                              epathSQLQuantity
	Target                                epathRealOracleTarget
	Proof                                 epathSQLPoolSourceProof
}

func epathSQLPoolSourceProofQuantity(proof epathSQLPoolSourceProof) (epathSQLQuantity, error) {
	if proof.Field != "rawValue" && proof.Field != "effectiveValue" {
		return epathSQLQuantity{}, fmt.Errorf("Pool native source proof permits only raw/effective scalar selectors")
	}
	decoded, err := hex.DecodeString(proof.Original.OriginalSHA256)
	if err != nil || len(decoded) != 32 || len(proof.Original.Owners) != 6 || len(proof.Original.HotWaterDemands) != 8 || len(proof.Original.ChilledWaterDemands) != 2 || len(proof.Original.ServedZones) != 5 || !reflect.DeepEqual(proof.Original.ServedZones, proof.Original.Declaration.ServedZones) {
		return epathSQLQuantity{}, fmt.Errorf("Pool source proof lacks its complete independently compiled original roster/hash")
	}
	specs, err := epathSQLPoolSourceSpecs(proof.Original)
	if err != nil {
		return epathSQLQuantity{}, err
	}
	count := 0
	for _, spec := range specs {
		if spec.ID == proof.Identity.Spec.ID {
			count++
			if !reflect.DeepEqual(spec, proof.Identity.Spec) {
				return epathSQLQuantity{}, fmt.Errorf("Pool source changed its exact original owner/fuel/plant/family")
			}
		}
	}
	if count != 1 {
		return epathSQLQuantity{}, fmt.Errorf("Pool source is outside the finite original native family census")
	}
	native, err := epathSQLPoolSourceQuantity(proof.Identity, proof.Weather)
	if err != nil {
		return epathSQLQuantity{}, err
	}
	// The native source reader integrates unrounded weather observations.
	// normalizeLegacyEnergyDataSources rounds that ANNUAL scalar once, to 3dp.
	// The source JSON wrapper only preserves presence; reopen is idempotent.
	// Hourly chart sample rounding and monthly allocation stages are different
	// quantities and cannot increase this source-scalar error budget.
	lo, hi := native.bounds()
	if !native.valid() || native.Value < 0 {
		return epathSQLQuantity{}, fmt.Errorf("invalid native Pool source quantity")
	}
	round := func(value float64) float64 { return math.Round(math.Max(0, value)*1000) / 1000 }
	return epathSQLBounded(round(native.Value), round(lo), round(hi)), nil
}

func epathSQLPoolSourceRequirements(frame epathSQLPoolSourceFrames) ([]epathSQLPoolSourceRequirement, error) {
	if err := epathSQLValidatePoolSourceFrames(frame); err != nil {
		return nil, err
	}
	var out []epathSQLPoolSourceRequirement
	for _, id := range epathSQLPoolSourceIDs(frame) {
		identity := frame.Sources[id]
		for _, field := range []string{"rawValue", "effectiveValue"} {
			proof := epathSQLPoolSourceProof{Identity: identity, Weather: frame.Weather, Original: frame.Original, Field: field}
			quantity, err := epathSQLPoolSourceProofQuantity(proof)
			if err != nil {
				return nil, err
			}
			s := identity.Source
			out = append(out, epathSQLPoolSourceRequirement{
				Group: identity.Spec.SourceGroup, Scope: "building", Period: "annual", Unit: "kWh",
				Key:      fmt.Sprintf("pool_native_source/%s/%s/%s/%s", identity.Spec.ID, s.Name, s.ReportingFrequency, field),
				Quantity: quantity, Proof: proof,
				Target: epathRealOracleTarget{Collection: "sources", Field: field, SourceName: s.Name, SourceKey: s.KeyValue, SourceUnit: s.SourceUnit, Frequency: s.ReportingFrequency, Unit: "kWh"},
			})
		}
	}
	if len(out) != 56 {
		return nil, fmt.Errorf("Pool native source scalar census must retain all 28 E/R frequency identities, including observed zero")
	}
	return out, nil
}

func epathSQLPoolPersistedSourceValue(value float64, quantity epathSQLQuantity) error {
	if !epathOracleFinite(value) || value < 0 || !quantity.valid() || value != math.Round(value*1000)/1000 {
		return fmt.Errorf("Pool source scalar is not a finite nonnegative 3dp transport value")
	}
	lo, hi := quantity.bounds()
	// Both interval endpoints are themselves quantized transport values.
	// A generic relative comparison slack must not admit another 0.001 grid.
	if value < lo || value > hi {
		return fmt.Errorf("Pool persisted source %.12g lies outside one-stage SQL interval [%.12g, %.12g]", value, lo, hi)
	}
	return nil
}

func epathSQLCheckPoolSource(bundle PurposeResultBundle, proof *epathSQLPoolSourceProof) error {
	if proof == nil {
		return fmt.Errorf("missing independently compiled Pool source proof")
	}
	quantity, err := epathSQLPoolSourceProofQuantity(*proof)
	if err != nil {
		return err
	}
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || result.Scope.ZoneName != "" {
		return fmt.Errorf("Pool native source scalar requires the original Building wrapper")
	}
	s, identity := proof.Identity.Source, proof.Identity
	id := fmt.Sprintf("sql-rdd-%d", s.DictionaryIndex)
	method := "sum_report_data"
	if s.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	sourceMap := map[string]EnergyDataSource{}
	count := 0
	for _, actual := range result.Sources {
		if actual.ID == "" {
			return fmt.Errorf("candidate contains a source without an identity")
		}
		if _, duplicate := sourceMap[actual.ID]; duplicate {
			return fmt.Errorf("candidate repeats a native/derived source identity")
		}
		sourceMap[actual.ID] = actual
		if actual.ID != id && !(strings.EqualFold(actual.Name, s.Name) && strings.EqualFold(actual.KeyValue, s.KeyValue) && actual.ReportingFrequency == s.ReportingFrequency) {
			continue
		}
		count++
		if actual.ID != id || actual.SourceType != "sql_report_data" || actual.IsMeter || actual.Name != s.Name || !strings.EqualFold(actual.KeyValue, s.KeyValue) || actual.SourceUnit != s.SourceUnit || actual.Units != s.SourceUnit || actual.NormalizedUnit != "kWh" || actual.ReportingFrequency != s.ReportingFrequency || actual.AggregationMethod != method || actual.ZoneName != "" || actual.AggregationBasis != "model_total" || actual.EffectiveMultiplier != 1 || actual.MultiplierApplication != "already_model_total" || actual.DriverRole != "context" || actual.InspectorSection != "Context" || len(actual.InputSourceIDs) != 0 || actual.Formula != "" {
			return fmt.Errorf("Pool source changed its native identity, model-total factor-one or nonadditive context semantics")
		}
		// The compiler separately parses original and executed documents. Their
		// indices need not coincide; an Output:Variable opener is not this owner.
		if actual.ObjectIndex == nil || *actual.ObjectIndex != identity.ExecutedOwnerIndex {
			return fmt.Errorf("Pool source lost its independently bound executed component index")
		}
		componentID := fmt.Sprintf("component:%d", identity.ExecutedOwnerIndex)
		componentCount := 0
		for _, related := range actual.RelatedEntityIDs {
			if related == componentID {
				componentCount++
			} else if strings.HasPrefix(related, "component:") {
				return fmt.Errorf("Pool native source borrowed a foreign executed component parent")
			}
		}
		if componentCount != 1 {
			return fmt.Errorf("Pool source lacks one exact executed component reference")
		}
		presence := actual.observedValuePresence
		if actual.inspectorDecodedFromJSON {
			presence = actual.inspectorValuePresence
		}
		if presence&3 != 3 {
			return fmt.Errorf("Pool source raw/effective presence is absent, not an observed or transport-rounded zero")
		}
		if err := epathSQLPoolPersistedSourceValue(actual.RawValue, quantity); err != nil {
			return err
		}
		if err := epathSQLPoolPersistedSourceValue(actual.EffectiveValue, quantity); err != nil {
			return err
		}
		if actual.RawValue != actual.EffectiveValue {
			return fmt.Errorf("Pool model-total source applied another Zone multiplier")
		}
		if actual.AllocationApplied || actual.AllocatedValue != 0 || actual.AllocationFactor != 0 {
			return fmt.Errorf("native Building observation was mislabeled an allocated Zone quantity")
		}
		seenDetails := map[string]bool{}
		for _, detail := range actual.ScopeDetails {
			key := detail.Scope.Kind + "|" + strings.ToLower(detail.Scope.ZoneName)
			if seenDetails[key] {
				return fmt.Errorf("Pool source repeats a scope detail")
			}
			seenDetails[key] = true
			if detail.Scope.Kind == "zone" {
				// Only independently observed canonical CW pump consumption can
				// have a Zone share. Its amount/path/annual closure are checked by
				// the separate source-local pump math consumer, never inferred here.
				served := false
				for _, zone := range proof.Original.ServedZones {
					if strings.EqualFold(zone, detail.Scope.ZoneName) {
						served = true
					}
				}
				if identity.Spec.ID != "pump.cw.electricity" || !identity.BudgetAuthority || !served {
					return fmt.Errorf("unqualified Pool/boiler/HW/companion source acquired a Zone consumption detail")
				}
				exactPresence := detail.inspectorDecodedFromJSON || detail.inspectorScopedValuePresence
				if !exactPresence || detail.inspectorValuePresence&3 != 0 || detail.RawValue != 0 || detail.EffectiveValue != 0 || detail.EffectiveMultiplier != 0 || detail.MultiplierApplication != "" || !detail.AllocationApplied || !epathOracleFinite(detail.AllocatedValue) || detail.AllocatedValue < 0 || !epathOracleFinite(detail.AllocationFactor) || detail.AllocationFactor < 0 || detail.AllocationFactor > 1 {
					return fmt.Errorf("CW pump allocated-only detail claimed measured Zone raw/effective energy or invalid allocation metadata")
				}
			} else {
				if detail.Scope.Kind != "building" || detail.Scope.ZoneName != "" || detail.AllocationApplied || detail.AllocatedValue != 0 || detail.AllocationFactor != 0 {
					return fmt.Errorf("Pool native source has an invalid non-Zone or allocated Building detail")
				}
				if detail.inspectorDecodedFromJSON || detail.inspectorScopedValuePresence {
					if detail.inspectorValuePresence&3 != 3 {
						return fmt.Errorf("Pool Building detail lost its native raw/effective observation")
					}
				}
				if detail.EffectiveMultiplier != 1 || detail.MultiplierApplication != "already_model_total" {
					return fmt.Errorf("Pool Building detail changed its model-total multiplier")
				}
				if err := epathSQLPoolPersistedSourceValue(detail.RawValue, quantity); err != nil {
					return err
				}
				if err := epathSQLPoolPersistedSourceValue(detail.EffectiveValue, quantity); err != nil {
					return err
				}
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("Pool source requires one exact candidate observation even when measured zero")
	}
	if identity.BudgetAuthority {
		return nil
	}
	// Thermal output and the three native E/R companions are context only.
	// A derived source must not launder one of them into an additive quantity.
	var reaches func(string, map[string]bool) bool
	reaches = func(current string, seen map[string]bool) bool {
		if current == id {
			return true
		}
		if seen[current] {
			return false
		}
		seen[current] = true
		for _, parent := range sourceMap[current].InputSourceIDs {
			if reaches(parent, seen) {
				return true
			}
		}
		return false
	}
	reject := func(ids []string) error {
		for _, sourceID := range ids {
			if reaches(sourceID, map[string]bool{}) {
				return fmt.Errorf("nonadditive Pool thermal/companion source became graph accounting provenance")
			}
		}
		return nil
	}
	checkGraph := func(nodes []EnergyExplanationNode, links []EnergyPathLink, rows []EnergyReconciliation) error {
		for _, node := range nodes {
			if err := reject(node.SourceIDs); err != nil {
				return err
			}
			for _, component := range node.LoadBreakdown {
				if err := reject(component.SourceIDs); err != nil {
					return err
				}
			}
			for _, offset := range node.OffsetEffects {
				if err := reject(offset.SourceIDs); err != nil {
					return err
				}
			}
			if node.SimultaneousLoad != nil {
				if err := reject(node.SimultaneousLoad.SourceIDs); err != nil {
					return err
				}
			}
		}
		for _, link := range links {
			if err := reject(link.SourceIDs); err != nil {
				return err
			}
		}
		for _, row := range rows {
			if err := reject(row.SourceIDs); err != nil {
				return err
			}
		}
		return nil
	}
	if err := checkGraph(result.Nodes, result.Links, result.Reconciliation); err != nil {
		return err
	}
	for _, period := range result.Periods {
		if err := checkGraph(period.Nodes, period.Links, period.Reconciliation); err != nil {
			return err
		}
	}
	for _, zone := range result.ZoneResults {
		if err := checkGraph(zone.Nodes, zone.Links, zone.Reconciliation); err != nil {
			return err
		}
		for _, period := range zone.Periods {
			if err := checkGraph(period.Nodes, period.Links, period.Reconciliation); err != nil {
				return err
			}
		}
	}
	return nil
}
