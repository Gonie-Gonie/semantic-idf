package simulation

// Independent acceptance support. Native hand observations tied to the independently validated
// original topology; no candidate or expected-artifact values are authority.
import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func epathSQLPoolConsumerHandFrame(t *testing.T) epathSQLPoolSourceFrames {
	t.Helper()
	proofs, err := epathSQLValidatePoolOriginal(epathSQLPoolOriginalFixture(t), []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()})
	if err != nil || len(proofs) != 1 {
		t.Fatalf("independent original Pool proof: %v", err)
	}
	return epathSQLPoolSourceHandFramesForOriginal(t, proofs[0])
}

func epathSQLPoolConsumerHandProof(frame epathSQLPoolSourceFrames, family string, companion int) epathSQLPoolSourceProof {
	f := frame.Families[family]
	id := f.CanonicalID
	if companion >= 0 {
		id = f.CompanionIDs[companion]
	}
	return epathSQLPoolSourceProof{Identity: frame.Sources[id], Original: frame.Original, Weather: frame.Weather, Field: "rawValue"}
}

func epathSQLPoolConsumerHandSource(t *testing.T, proof epathSQLPoolSourceProof) EnergyDataSource {
	t.Helper()
	q, err := epathSQLPoolSourceProofQuantity(proof)
	if err != nil {
		t.Fatal(err)
	}
	s := proof.Identity.Source
	method := "sum_report_data"
	if s.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	index := proof.Identity.ExecutedOwnerIndex
	return EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", s.DictionaryIndex), SourceType: "sql_report_data", Name: s.Name, KeyValue: s.KeyValue, Units: s.SourceUnit, SourceUnit: s.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: s.ReportingFrequency, AggregationMethod: method, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", DriverRole: "context", InspectorSection: "Context", RawValue: q.Value, EffectiveValue: q.Value, ObjectIndex: &index, RelatedEntityIDs: []string{fmt.Sprintf("component:%d", index)}, observedValuePresence: 3}
}

func epathSQLPoolConsumerHandBundle(t *testing.T, proof epathSQLPoolSourceProof) PurposeResultBundle {
	t.Helper()
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "effective"}, Sources: []EnergyDataSource{epathSQLPoolConsumerHandSource(t, proof)}}}
}

// Leaf decoding intentionally avoids EnergyExplanationResult's normalizing
// reader. The consumer must see the original scalar/presence, not a repair.
func epathSQLPoolConsumerDecodeSource(t *testing.T, source EnergyDataSource, explicit bool) EnergyDataSource {
	t.Helper()
	var value any = source
	if explicit {
		value = energyPathSourceWire{source}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EnergyDataSource
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestEnergyPathRealSQLPoolSourceConsumerRequirements(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	rows, err := epathSQLPoolSourceRequirements(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 56 {
		t.Fatalf("got %d, need 28 native identities times both source scalars", len(rows))
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	seenSources := map[int]bool{}
	keys := map[string]bool{}
	groups := map[string]int{}
	for _, row := range rows {
		id := row.Proof.Identity.Source.DictionaryIndex
		if !seenSources[id] {
			bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLPoolConsumerHandSource(t, row.Proof))
			seenSources[id] = true
		}
		if keys[row.Key] || row.Key == "" {
			t.Fatal("duplicate/absent independently required source selector")
		}
		keys[row.Key] = true
		groups[row.Group]++
		if row.Scope != "building" || row.Zone != "" || row.Period != "annual" || row.Unit != "kWh" || row.Target.Collection != "sources" || row.Target.Field != row.Proof.Field || row.Target.AllowPrunedZero {
			t.Fatal("native source selector escaped its exact annual/presence contract")
		}
		if err := epathValidateOracleTarget(row.Target, row.Unit); err != nil {
			t.Fatal(err)
		}
	}
	if groups["loads"] != 16 || groups["endUses"] != 40 || len(groups) != 2 {
		t.Fatalf("source-only thermal/purchased groups changed: %v", groups)
	}
	before := append([]EnergyDataSource(nil), bundle.EnergyExplanation.Sources...)
	for _, row := range rows {
		if err := epathSQLCheckPoolSource(bundle, &row.Proof); err != nil {
			t.Fatalf("%s: %v", row.Key, err)
		}
	}
	if !reflect.DeepEqual(before, bundle.EnergyExplanation.Sources) {
		t.Fatal("source consumer repaired/mutated candidate observations")
	}
	delete(frame.Sources, frame.Families["boiler.ancillary_electricity"].CanonicalID)
	if _, err := epathSQLPoolSourceRequirements(frame); err == nil {
		t.Fatal("missing measured-zero identity weakened required coverage")
	}
}

func TestEnergyPathRealSQLPoolSourceConsumerIdentityAndPresence(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	base := epathSQLPoolConsumerHandProof(frame, "boiler.natural_gas", -1)
	for _, name := range []string{"raw missing", "effective missing", "wrong ID", "duplicate ID", "identity collision", "wrong key", "wrong native unit", "wrong frequency", "wrong method", "zone multiplier", "wrong owner index", "output opener as owner", "missing owner reference", "duplicate owner reference", "foreign component parent", "zone owner", "passive driver", "thermal formula", "derived source", "allocated Building", "missing request", "invalid original hash", "wrong original plant", "incomplete original roster", "native raw sum", "native dictionary", "unsupported field", "wrong scope"} {
		t.Run(name, func(t *testing.T) {
			proof := base
			bundle := epathSQLPoolConsumerHandBundle(t, proof)
			s := &bundle.EnergyExplanation.Sources[0]
			switch name {
			case "raw missing":
				s.observedValuePresence = 2
			case "effective missing":
				s.observedValuePresence = 1
			case "wrong ID":
				s.ID = "sql-rdd-999999"
			case "duplicate ID":
				bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, *s)
			case "identity collision":
				copy := *s
				copy.ID = "sql-rdd-999999"
				bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, copy)
			case "wrong key":
				s.KeyValue = "Another Boiler"
			case "wrong native unit":
				s.SourceUnit = "kWh"
			case "wrong frequency":
				s.ReportingFrequency = "Hourly"
			case "wrong method":
				s.AggregationMethod = "average_report_data"
			case "zone multiplier":
				s.EffectiveMultiplier = 10
				s.EffectiveValue *= 10
			case "wrong owner index":
				index := *s.ObjectIndex + 1
				s.ObjectIndex = &index
			case "output opener as owner":
				opener := *s.ObjectIndex + 100
				proof.Identity.OutputObjectIndex = &opener
				s.ObjectIndex = &opener
			case "missing owner reference":
				s.RelatedEntityIDs = nil
			case "duplicate owner reference":
				s.RelatedEntityIDs = append(s.RelatedEntityIDs, s.RelatedEntityIDs[0])
			case "foreign component parent":
				s.RelatedEntityIDs = append(s.RelatedEntityIDs, "component:999999")
			case "zone owner":
				s.ZoneName = proof.Original.Declaration.ZoneName
			case "passive driver":
				s.DriverRole = "driver"
				s.InspectorSection = "Drivers"
			case "thermal formula":
				s.Formula = "Boiler Heating Energy / efficiency"
			case "derived source":
				s.InputSourceIDs = []string{"derived-thermal-input"}
			case "allocated Building":
				s.AllocationApplied = true
				s.AllocatedValue = s.EffectiveValue
			case "missing request":
				proof.Identity.RequestBound = false
			case "invalid original hash":
				proof.Original.OriginalSHA256 = "not-an-original-hash"
			case "wrong original plant":
				proof.Identity.Spec.Owner.PlantLoopName = "Other Loop"
			case "incomplete original roster":
				proof.Original.HotWaterDemands = nil
			case "native raw sum":
				wrong := *proof.Identity.Source.RawSum + 1e9
				proof.Identity.Source.RawSum = &wrong
			case "native dictionary":
				proof.Identity.Dictionary.Type = "Avg"
			case "unsupported field":
				proof.Field = "allocatedValue"
			case "wrong scope":
				bundle.EnergyExplanation.Scope.Kind = "zone"
				bundle.EnergyExplanation.Scope.ZoneName = "SPACE1-1"
			}
			if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

func epathSQLPoolConsumerPower(proof epathSQLPoolSourceProof, powerKW float64) epathSQLPoolSourceProof {
	s := proof.Identity.Source
	proof.Identity.Source, proof.Identity.Rows = epathSQLPoolSourceHandObservation(s.DictionaryIndex, s.Name, s.KeyValue, s.SourceUnit, s.ReportingFrequency, false, powerKW)
	for m, bucket := range proof.Identity.Source.Months {
		proof.Identity.NativeRawMonthly[m], proof.Identity.NativeEnergyMonthly[m] = *bucket.RawSum, *bucket.EnergyKWh
	}
	return proof
}

func TestEnergyPathRealSQLPoolSourceConsumerOneScalarRoundingStage(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	for _, selection := range []int{-1, 0, 1, 2} {
		proof := epathSQLPoolConsumerPower(epathSQLPoolConsumerHandProof(frame, "pool.water_heating", selection), 0.00049)
		native, err := epathSQLPoolSourceQuantity(proof.Identity, proof.Weather)
		if err != nil {
			t.Fatal(err)
		}
		q, err := epathSQLPoolSourceProofQuantity(proof)
		if err != nil {
			t.Fatal(err)
		}
		if !epathSQLPoolSameNative(native.Value, 0.00049*8760) || q.Value != 4.292 {
			t.Fatalf("native and persisted quantity conflated: native=%g wire=%g", native.Value, q.Value)
		}
		bundle := epathSQLPoolConsumerHandBundle(t, proof)
		for reload := 0; reload < 2; reload++ {
			bundle.EnergyExplanation.Sources[0] = epathSQLPoolConsumerDecodeSource(t, bundle.EnergyExplanation.Sources[0], true)
			if err := epathSQLCheckPoolSource(bundle, &proof); err != nil {
				t.Fatalf("selection %d reopen %d: %v", selection, reload, err)
			}
		}
		for _, value := range []float64{native.Value, 4.291, 4.293, 0, math.NaN(), math.Inf(1), -1} {
			bundle.EnergyExplanation.Sources[0].RawValue, bundle.EnergyExplanation.Sources[0].EffectiveValue = value, value
			if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
				t.Fatalf("selection %d accepted extra stage/per-hour rounding/raw native scalar %g", selection, value)
			}
		}
	}
	// A generic 1e-8 relative comparison may span several 0.001 grid points
	// here. The persisted source contract admits only the one quantized result.
	proof := epathSQLPoolConsumerHandProof(frame, "boiler.heating_output", -1)
	bundle := epathSQLPoolConsumerHandBundle(t, proof)
	bundle.EnergyExplanation.Sources[0].RawValue += 0.001
	bundle.EnergyExplanation.Sources[0].EffectiveValue += 0.001
	if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
		t.Fatal("generic relative slack admitted an adjacent wrong source transport grid")
	}
}

func TestEnergyPathRealSQLPoolSourceConsumerZeroAndTinyKnownWire(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	for _, proof := range []epathSQLPoolSourceProof{
		epathSQLPoolConsumerHandProof(frame, "boiler.ancillary_electricity", -1),
		epathSQLPoolConsumerPower(epathSQLPoolConsumerHandProof(frame, "pool.water_heating", -1), 1e-8),
	} {
		bundle := epathSQLPoolConsumerHandBundle(t, proof)
		if bundle.EnergyExplanation.Sources[0].RawValue != 0 {
			t.Fatal("hand zero/tiny native value does not land on wire zero")
		}
		for reload := 0; reload < 2; reload++ {
			bundle.EnergyExplanation.Sources[0] = epathSQLPoolConsumerDecodeSource(t, bundle.EnergyExplanation.Sources[0], true)
			if err := epathSQLCheckPoolSource(bundle, &proof); err != nil {
				t.Fatal(err)
			}
		}
		// Generic leaf omitempty is not the real source-wire wrapper: explicit
		// zero disappears. The independent decoder must retain that absence.
		bundle.EnergyExplanation.Sources[0] = epathSQLPoolConsumerDecodeSource(t, bundle.EnergyExplanation.Sources[0], false)
		if bundle.EnergyExplanation.Sources[0].inspectorValuePresence&3 != 0 {
			t.Fatal("legacy leaf fixture unexpectedly retained scalar presence")
		}
		if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
			t.Fatal("omitted zero fields were treated as measured observations")
		}
		bundle.EnergyExplanation.Sources = nil
		if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
			t.Fatal("missing whole observed-zero source was pruned")
		}
		for _, field := range []string{"rawValue", "effectiveValue"} {
			source := epathSQLPoolConsumerHandSource(t, proof)
			raw, err := json.Marshal(energyPathSourceWire{source})
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			fields[field] = json.RawMessage("null")
			raw, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			var decoded EnergyDataSource
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			bundle.EnergyExplanation.Sources = []EnergyDataSource{decoded}
			if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
				t.Fatalf("explicit null %s was treated as observed zero", field)
			}
		}
	}
}

func TestEnergyPathRealSQLPoolSourceConsumerHalfGridUsesNativeIntervalOnly(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	proof := epathSQLPoolConsumerPower(epathSQLPoolConsumerHandProof(frame, "pool.water_heating", -1), 1.2345/8760)
	q, err := epathSQLPoolSourceProofQuantity(proof)
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := q.bounds()
	if lo != 1.234 || hi != 1.235 {
		t.Fatalf("half-grid native integration uncertainty not mapped once: [%g,%g]", lo, hi)
	}
	for _, value := range []float64{lo, hi} {
		if err := epathSQLPoolPersistedSourceValue(value, q); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []float64{1.233, 1.2345, 1.236} {
		if err := epathSQLPoolPersistedSourceValue(value, q); err == nil {
			t.Fatalf("half-grid proof widened to a second rounding stage/raw scalar: %g", value)
		}
	}
}

func TestEnergyPathRealSQLPoolSourceConsumerExecutedOwnerNotOriginalOffset(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	proof := epathSQLPoolConsumerHandProof(frame, "pool.water_heating", -1)
	// Hand executed document has a deliberately non-unit index displacement.
	// The prior native compiler proves actual fields in each document; this
	// consumer must use that explicit executed binding, not infer original+1.
	proof.Identity.ExecutedOwnerIndex += 17
	opener := proof.Identity.ExecutedOwnerIndex + 47
	proof.Identity.OutputObjectIndex = &opener
	bundle := epathSQLPoolConsumerHandBundle(t, proof)
	if err := epathSQLCheckPoolSource(bundle, &proof); err != nil {
		t.Fatal(err)
	}
	wrong := proof.Identity.Spec.Owner.ObjectIndex + 1
	bundle.EnergyExplanation.Sources[0].ObjectIndex = &wrong
	if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
		t.Fatal("original index + 1 replaced independently parsed executed owner")
	}
}

func TestEnergyPathRealSQLPoolSourceConsumerContextCannotBecomeAccounting(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	for _, proof := range []epathSQLPoolSourceProof{
		epathSQLPoolConsumerHandProof(frame, "pool.water_heating", -1),
		epathSQLPoolConsumerHandProof(frame, "boiler.heating_output", -1),
		epathSQLPoolConsumerHandProof(frame, "boiler.natural_gas", 0),
		epathSQLPoolConsumerHandProof(frame, "pump.cw.electricity", 1),
	} {
		for _, location := range []string{"node", "link", "load breakdown", "offset", "simultaneous", "ledger", "monthly", "annual alias", "zone", "zone month", "derived ancestry"} {
			bundle := epathSQLPoolConsumerHandBundle(t, proof)
			id := bundle.EnergyExplanation.Sources[0].ID
			node := EnergyExplanationNode{ID: "hand-load", Level: "load", Unit: "kWh_th", Value: 1, SourceIDs: []string{id}}
			switch location {
			case "node":
				bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{node}
			case "link":
				bundle.EnergyExplanation.Links = []EnergyPathLink{{SourceIDs: []string{id}}}
			case "load breakdown":
				node.SourceIDs = nil
				node.LoadBreakdown = []EnergyExplanationLoadComponent{{SourceIDs: []string{id}}}
				bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{node}
			case "offset":
				node.SourceIDs = nil
				node.OffsetEffects = []EnergyExplanationOffsetEffect{{SourceIDs: []string{id}}}
				bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{node}
			case "simultaneous":
				node.SourceIDs = nil
				node.SimultaneousLoad = &EnergyExplanationSimultaneousLoad{SourceIDs: []string{id}}
				bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{node}
			case "ledger":
				bundle.EnergyExplanation.Reconciliation = []EnergyReconciliation{{SourceIDs: []string{id}}}
			case "monthly":
				bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{node}}}
			case "annual alias":
				bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: "annual", Nodes: []EnergyExplanationNode{node}}}
			case "zone":
				bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Nodes: []EnergyExplanationNode{node}}}
			case "zone month":
				bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{node}}}}}
			case "derived ancestry":
				bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: "derived-a", InputSourceIDs: []string{"derived-b"}}, EnergyDataSource{ID: "derived-b", InputSourceIDs: []string{"derived-a", id}})
				node.SourceIDs = []string{"derived-a"}
				bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{node}
			}
			if err := epathSQLCheckPoolSource(bundle, &proof); err == nil {
				t.Fatalf("%s/%s acquired additive provenance", proof.Identity.Spec.ID, location)
			}
		}
		bundle := epathSQLPoolConsumerHandBundle(t, proof)
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: "unrelated-native-cooling"})
		bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "cooling", SourceIDs: []string{"unrelated-native-cooling"}}}
		if err := epathSQLCheckPoolSource(bundle, &proof); err != nil {
			t.Fatalf("unrelated cooling was suppressed: %v", err)
		}
	}
}

func TestEnergyPathRealSQLPoolSourceConsumerAllocatedOnlyCWBoundary(t *testing.T) {
	frame := epathSQLPoolConsumerHandFrame(t)
	for _, family := range []string{"pool.water_heating", "boiler.heating_output", "boiler.natural_gas", "boiler.ancillary_natural_gas", "boiler.ancillary_electricity", "pump.hw.electricity", "pump.cw.electricity"} {
		for _, companion := range []int{-1, 0, 1, 2} {
			proof := epathSQLPoolConsumerHandProof(frame, family, companion)
			bundle := epathSQLPoolConsumerHandBundle(t, proof)
			bundle.EnergyExplanation.Sources[0].ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1-1"}, AllocationApplied: true, AllocatedValue: 12, AllocationFactor: 0.1, inspectorScopedValuePresence: true}}
			err := epathSQLCheckPoolSource(bundle, &proof)
			if family != "pump.cw.electricity" || companion != -1 {
				if err == nil {
					t.Fatalf("%s/%d borrowed a measured or allocated Zone scope", family, companion)
				}
				continue
			}
			if err != nil {
				t.Fatalf("canonical CW allocation-only metadata was rejected before its separate math proof: %v", err)
			}
			bundle.EnergyExplanation.Sources[0] = epathSQLPoolConsumerDecodeSource(t, bundle.EnergyExplanation.Sources[0], true)
			if err := epathSQLCheckPoolSource(bundle, &proof); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range []string{"known zero", "raw amount", "plenum", "other zone", "native multiplier", "missing exact presence", "negative allocation", "factor outside budget", "duplicate zone"} {
				copy := bundle
				copy.EnergyExplanation.Sources = append([]EnergyDataSource(nil), bundle.EnergyExplanation.Sources...)
				copy.EnergyExplanation.Sources[0].ScopeDetails = append([]EnergyDataSourceScopeDetail(nil), bundle.EnergyExplanation.Sources[0].ScopeDetails...)
				detail := &copy.EnergyExplanation.Sources[0].ScopeDetails[0]
				switch mutation {
				case "known zero":
					detail.inspectorValuePresence = 3
				case "raw amount":
					detail.RawValue = 12
				case "plenum":
					detail.Scope.ZoneName = proof.Original.Declaration.ReturnPlenumZoneName
				case "other zone":
					detail.Scope.ZoneName = "Another Zone"
				case "native multiplier":
					detail.EffectiveMultiplier = 1
					detail.MultiplierApplication = "already_model_total"
				case "missing exact presence":
					detail.inspectorDecodedFromJSON = false
					detail.inspectorScopedValuePresence = false
				case "negative allocation":
					detail.AllocatedValue = -1
				case "factor outside budget":
					detail.AllocationFactor = 1.01
				case "duplicate zone":
					copy.EnergyExplanation.Sources[0].ScopeDetails = append(copy.EnergyExplanation.Sources[0].ScopeDetails, *detail)
				}
				if err := epathSQLCheckPoolSource(copy, &proof); err == nil {
					t.Fatalf("accepted CW detail %s", mutation)
				}
			}
		}
	}
}
