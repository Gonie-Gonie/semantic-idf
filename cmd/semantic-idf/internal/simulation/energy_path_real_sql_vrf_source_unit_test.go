package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func epathVRFSourceUnitFixture(t *testing.T) (epathSQLFrames, epathSQLModelChecks, map[string]any) {
	t.Helper()
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	return epathVRFSourceUnitSystemFixture(t, frames, system)
}

func epathVRFSourceUnitSystemFixture(t *testing.T, frames epathSQLFrames, system epathSQLVRFSystemFrame) (epathSQLFrames, epathSQLModelChecks, map[string]any) {
	t.Helper()
	allocation, err := epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	frames.NativeVRFSystems = []epathSQLVRFSystemFrame{system}
	frames.NativeVRFAllocations = []epathSQLVRFAllocationProof{allocation}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelVRFSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	rows := []any{}
	for _, source := range system.Sources {
		annual := math.Round(*source.Source.EnergyKWh*1000) / 1000
		row := map[string]any{"id": fmt.Sprintf("sql-rdd-%d", source.Source.DictionaryIndex), "sourceType": "sql_report_data", "name": source.Source.Name, "keyValue": source.Source.KeyValue, "zoneName": source.ZoneName,
			"units": "J", "sourceUnit": "J", "normalizedUnit": "kWh", "reportingFrequency": "Monthly", "aggregationMethod": "sum", "aggregationBasis": "model_total", "effectiveMultiplier": 1, "multiplierApplication": "already_model_total",
			"rawValue": annual, "effectiveValue": annual, "allocatedValue": annual, "allocationApplied": false}
		if source.RequestObjectIndex != nil {
			row["objectIndex"] = *source.RequestObjectIndex
		}
		details := []any{}
		for _, terminal := range system.Declaration.Terminals {
			zone := terminal.ZoneName
			if !source.Shared && zone != source.ZoneName {
				continue
			}
			detail := map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": zone, "aggregationBasis": "model_total"}, "aggregationBasis": "model_total"}
			if source.Shared {
				detail["allocatedValue"] = float64(allocation.Annual.Sources[source.Source.DictionaryIndex].Shares[zone].DisplayMilliKWh) / 1000
				detail["allocationApplied"] = true
			} else {
				detail["rawValue"], detail["effectiveValue"], detail["allocatedValue"] = annual, annual, annual
				detail["effectiveMultiplier"], detail["multiplierApplication"], detail["allocationApplied"] = 1, "already_model_total", false
			}
			details = append(details, detail)
		}
		row["scopeDetails"] = details
		rows = append(rows, row)
	}
	return frames, checks, map[string]any{"energyExplanation": map[string]any{"schema": "semantic-idf.energy-explanation/v2", "scope": map[string]any{"kind": "building", "aggregationBasis": "model_total"}, "sources": rows}}
}

func epathVRFSourceUnitDecode(t *testing.T, wire map[string]any) PurposeResultBundle {
	t.Helper()
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func epathVRFSourceUnitRow(t *testing.T, wire map[string]any, shared bool) map[string]any {
	t.Helper()
	for _, item := range wire["energyExplanation"].(map[string]any)["sources"].([]any) {
		row := item.(map[string]any)
		if (row["zoneName"] == "") == shared {
			return row
		}
	}
	t.Fatal("source row missing")
	return nil
}

func TestEnergyPathRealSQLVRFSourceScalarContexts(t *testing.T) {
	_, checks, wire := epathVRFSourceUnitFixture(t)
	bundle := epathVRFSourceUnitDecode(t, wire)
	if len(checks.Rows) != 132 {
		t.Fatalf("source obligations=%d, want132", len(checks.Rows))
	}
	unknown, zeros := 0, 0
	for _, check := range checks.Rows {
		if check.Quantity == nil {
			unknown++
		} else if check.Quantity.Value == 0 {
			zeros++
		}
		if err := epathCheckSQLVRFSource(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Item.Key, err)
		}
	}
	if unknown != 40 || zeros != 12 {
		t.Fatalf("unknown/observed0 obligations=%d/%d, want40/12", unknown, zeros)
	}
	// omitempty allocated=0 remains a known accounting zero only because its
	// own original raw/effective zero and local observation policy are proven.
	row := epathVRFSourceUnitRow(t, wire, false)
	delete(row, "allocatedValue")
	detail := row["scopeDetails"].([]any)[0].(map[string]any)
	delete(detail, "allocatedValue")
	bundle = epathVRFSourceUnitDecode(t, wire)
	for _, check := range checks.Rows {
		if err := epathCheckSQLVRFSource(bundle, check); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnergyPathRealSQLVRFSourceRejectsUnknownZeroAndScopeSpoofs(t *testing.T) {
	for _, kind := range []string{"missing root zero", "null root zero", "tiny root zero", "tiny scoped zero", "missing scoped zero", "null scoped zero", "shared measured zero", "shared changed integer", "shared multiplier", "local allocated flag", "missing scope", "duplicate scope", "wrong owner", "extra local owner", "shared extra plenum", "shared duplicate owner", "wrong Output index", "wrong unit", "duplicate source", "missing source"} {
		t.Run(kind, func(t *testing.T) {
			_, checks, wire := epathVRFSourceUnitFixture(t)
			shared := strings.HasPrefix(kind, "shared")
			row := epathVRFSourceUnitRow(t, wire, shared)
			detail := row["scopeDetails"].([]any)[0].(map[string]any)
			switch kind {
			case "missing root zero":
				delete(row, "rawValue")
			case "null root zero":
				row["rawValue"] = nil
			case "tiny root zero":
				row["rawValue"] = 1e-9
			case "tiny scoped zero":
				detail["effectiveValue"] = 1e-9
			case "missing scoped zero":
				delete(detail, "effectiveValue")
			case "null scoped zero":
				detail["effectiveValue"] = nil
			case "shared measured zero":
				detail["rawValue"] = 0
			case "shared changed integer":
				detail["allocatedValue"] = detail["allocatedValue"].(float64) + 1e-9
			case "shared multiplier":
				detail["effectiveMultiplier"], detail["multiplierApplication"] = 1, "already_model_total"
			case "local allocated flag":
				detail["allocationApplied"] = true
			case "missing scope":
				row["scopeDetails"] = []any{}
			case "duplicate scope":
				row["scopeDetails"] = append(row["scopeDetails"].([]any), detail)
			case "wrong owner":
				detail["scope"].(map[string]any)["zoneName"] = "PLENUM-1"
			case "extra local owner", "shared extra plenum":
				zone := "SPACE2-1"
				if shared {
					zone = "PLENUM-1"
				}
				extra := map[string]any{}
				for key, value := range detail {
					extra[key] = value
				}
				extra["scope"] = map[string]any{"kind": "zone", "zoneName": zone, "aggregationBasis": "model_total"}
				row["scopeDetails"] = append(row["scopeDetails"].([]any), extra)
			case "shared duplicate owner":
				row["scopeDetails"] = append(row["scopeDetails"].([]any), detail)
			case "wrong Output index":
				row["objectIndex"] = 1
			case "wrong unit":
				row["sourceUnit"] = "kWh"
			case "duplicate source":
				graph := wire["energyExplanation"].(map[string]any)
				graph["sources"] = append(graph["sources"].([]any), row)
			case "missing source":
				graph := wire["energyExplanation"].(map[string]any)
				graph["sources"] = graph["sources"].([]any)[1:]
			}
			bundle := epathVRFSourceUnitDecode(t, wire)
			failed := false
			for _, check := range checks.Rows {
				failed = failed || epathCheckSQLVRFSource(bundle, check) != nil
			}
			if !failed {
				t.Fatal("spoofed source observation accepted")
			}
		})
	}
	// Original-wire preflight forbids explicit null allocated values even
	// though the native scalar omitempty contract can legitimately omit zero.
	for _, field := range []string{"rawValue", "effectiveValue", "allocatedValue"} {
		if err := epathOracleWireNumber(map[string]json.RawMessage{field: json.RawMessage("null")}, field, "source.scopeDetails", false); err == nil {
			t.Fatal("explicit null healed as omitted0")
		}
	}
}

func TestEnergyPathRealSQLVRFSourceAnnualObservationIsNotMonthlyBudget(t *testing.T) {
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	var values [12]float64
	for month := range values {
		values[month] = 1.0004
	}
	epathSQLVRFAllocationUnitReplaceSource(&system, "local_cooling", "SPACE1-1", values)
	epathSQLVRFAllocationUnitReplaceSource(&system, "shared_cooling", "", values)
	frames, checks, wire := epathVRFSourceUnitSystemFixture(t, frames, system)
	bundle := epathVRFSourceUnitDecode(t, wire)
	for _, check := range checks.Rows {
		if err := epathCheckSQLVRFSource(bundle, check); err != nil {
			t.Fatal(err)
		}
	}
	for _, source := range system.Sources {
		if source.Role != "shared_cooling" && !(source.Role == "local_cooling" && source.ZoneName == "SPACE1-1") {
			continue
		}
		if got := math.Round(*source.Source.EnergyKWh*1000) / 1000; got != 12.005 {
			t.Fatalf("native annual once-round=%g, want 12.005", got)
		}
		if got := frames.NativeVRFAllocations[0].Annual.Sources[source.Source.DictionaryIndex].BudgetMilliKWh; got != 12000 {
			t.Fatalf("monthly budget=%d, want12000", got)
		}
		// Replacing an annual source inspector observation with the graph's
		// month-first budget must fail; no widened .005 display tolerance.
		mutant := bundle
		mutant.EnergyExplanation.Sources = append([]EnergyDataSource(nil), bundle.EnergyExplanation.Sources...)
		for index := range mutant.EnergyExplanation.Sources {
			if mutant.EnergyExplanation.Sources[index].ID == fmt.Sprintf("sql-rdd-%d", source.Source.DictionaryIndex) {
				mutant.EnergyExplanation.Sources[index].RawValue = 12
			}
		}
		for _, check := range checks.Rows {
			if check.NativeVRFSource.Observation.Source.DictionaryIndex == source.Source.DictionaryIndex && check.Item.Scope == "building" && check.Item.Target.Field == "rawValue" {
				if err := epathCheckSQLVRFSource(mutant, check); err == nil {
					t.Fatal("monthly budget replaced original annual scalar")
				}
			}
		}
	}
}

func TestEnergyPathRealSQLVRFSourceCannotChangeQuantityOrPeriod(t *testing.T) {
	_, checks, wire := epathVRFSourceUnitFixture(t)
	bundle := epathVRFSourceUnitDecode(t, wire)
	for _, kind := range []string{"monthly", "wrong source selector", "wrong Zone", "changed allocation quantity", "changed native quantity"} {
		t.Run(kind, func(t *testing.T) {
			var check epathSQLModelCheck
			for _, item := range checks.Rows {
				if kind == "changed allocation quantity" {
					if item.Item.Scope == "zone" && item.NativeVRFSource.Observation.Shared && item.Item.Target.Field == "allocatedValue" {
						check = item
						break
					}
				} else if item.Item.Scope == "building" && item.Item.Target.Field == "rawValue" {
					check = item
					break
				}
			}
			switch kind {
			case "monthly":
				check.Item.Period = "M1"
			case "wrong source selector":
				check.Item.Target.SourceName = "Zone Lights Electricity Energy"
			case "wrong Zone":
				check.Item.Scope, check.Item.Zone = "zone", "PLENUM-1"
			default:
				q := *check.Quantity
				q.Value += 1
				check.Quantity = &q
			}
			if err := epathCheckSQLVRFSource(bundle, check); err == nil {
				t.Fatal("source proof escaped fixed original quantity/period/context")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFOriginalSourceUnionRejectsMasquerade(t *testing.T) {
	frames, _, wire := epathVRFSourceUnitFixture(t)
	bundle := epathVRFSourceUnitDecode(t, wire)
	system := frames.NativeVRFSystems[0]
	observation := system.Sources[0]
	identity := epathSQLVRFSourceIdentity{System: system.Declaration, Observation: observation, Precision: system.Precision}
	proof, err := epathSQLOriginalVRF(identity)
	if err != nil {
		t.Fatal(err)
	}
	key, err := epathSQLOriginalKey(proof)
	if err != nil {
		t.Fatal(err)
	}
	var actual EnergyDataSource
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID == key {
			actual = source
		}
	}
	allowed := map[string]epathSQLOriginalSource{key: proof}
	if err := epathSQLVerifyOriginalSources([]string{key}, map[string]EnergyDataSource{key: actual}, allowed, map[string]bool{key: true}, "M7"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"same ID wrapper", "foreign ID wrapper", "wrong owner", "wrong output index", "different RDD binding", "PTAC overlap"} {
		t.Run(kind, func(t *testing.T) {
			bad, permit := actual, proof
			id := key
			switch kind {
			case "same ID wrapper":
				bad.InputSourceIDs = []string{"plain"}
			case "foreign ID wrapper":
				id = "derived-fake-vrf"
				bad.ID = id
				bad.SourceType = "derived"
				bad.InputSourceIDs = []string{key}
			case "wrong owner":
				bad.ZoneName = "SPACE2-1"
			case "wrong output index":
				index := 9
				bad.ObjectIndex = &index
			case "different RDD binding":
				copy := *proof.RDD
				copy.DictionaryIndex = 999
				permit.RDD = &copy
			case "PTAC overlap":
				permit.DirectHVAC = &epathSQLDirectHVACSourceIdentity{}
			}
			allowed := map[string]epathSQLOriginalSource{key: permit}
			sources := map[string]EnergyDataSource{key: actual, id: bad, "plain": actual}
			if _, err := epathSQLOriginalSourceLeaves([]string{id}, sources, allowed, "annual"); err == nil {
				t.Fatal("original VRF source masquerade accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFSourceCoverageUsesTypedScalarContract(t *testing.T) {
	fixture := func() (epathSQLModelChecks, map[string]any) {
		frames, checks, wire := epathVRFSourceUnitFixture(t)
		zones := []any{}
		for _, terminal := range frames.NativeVRFSystems[0].Declaration.Terminals {
			zones = append(zones, map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": terminal.ZoneName, "aggregationBasis": "model_total"}})
		}
		wire["energyExplanation"].(map[string]any)["zoneResults"] = zones
		return checks, wire
	}
	checks, wire := fixture()
	coverage := epathSQLModelCoverage(epathVRFSourceUnitDecode(t, wire), checks)
	for _, failure := range coverage.Failures {
		if checks.Keys[failure.Key] {
			t.Fatalf("valid original source selector failed coverage: %s: %s", failure.Key, failure.Message)
		}
		// This source-only hand fixture deliberately has no graph quality;
		// each of the six contexts must still retain all nine quality gaps.
		if !strings.Contains(failure.Key, "/quality/") {
			t.Fatalf("unexpected non-quality coverage failure: %#v", failure)
		}
	}
	if len(coverage.Failures) != 54 {
		t.Fatalf("unrelated required quality obligations changed: %d, want54", len(coverage.Failures))
	}
	allocated, unknown, zero := 0, 0, 0
	for _, check := range checks.Rows {
		if check.Item.Target.Field == "allocatedValue" {
			allocated++
		}
		if check.Quantity == nil {
			unknown++
		} else if check.Quantity.Value == 0 {
			zero++
		}
	}
	if len(checks.Rows) != 132 || allocated != 44 || unknown != 40 || zero != 12 {
		t.Fatalf("source coverage obligations changed: %d/%d/%d/%d", len(checks.Rows), allocated, unknown, zero)
	}
	for _, kind := range []string{"untyped allocated", "missing original source", "missing local zero", "shared fabricated zero", "foreign scope", "changed allocation", "unsupported selector property", "monthly substitution"} {
		t.Run(kind, func(t *testing.T) {
			checks, wire := fixture()
			row := epathVRFSourceUnitRow(t, wire, strings.HasPrefix(kind, "shared") || kind == "changed allocation")
			switch kind {
			case "untyped allocated", "unsupported selector property", "monthly substitution":
				for index := range checks.Rows {
					check := &checks.Rows[index]
					if check.Item.Target.Field != "allocatedValue" {
						continue
					}
					if kind == "untyped allocated" {
						check.NativeVRFSource = nil
					} else if kind == "unsupported selector property" {
						check.Item.Target.Carrier = "electricity"
					} else {
						check.Item.Target.Frequency = "Hourly"
					}
					break
				}
			case "missing original source":
				graph := wire["energyExplanation"].(map[string]any)
				graph["sources"] = graph["sources"].([]any)[1:]
			case "missing local zero":
				delete(row, "rawValue")
			case "shared fabricated zero":
				row["scopeDetails"].([]any)[0].(map[string]any)["rawValue"] = 0
			case "foreign scope":
				row["scopeDetails"].([]any)[0].(map[string]any)["scope"].(map[string]any)["zoneName"] = "PLENUM-1"
			case "changed allocation":
				detail := row["scopeDetails"].([]any)[0].(map[string]any)
				detail["allocatedValue"] = detail["allocatedValue"].(float64) + 1e-9
			}
			coverage := epathSQLModelCoverage(epathVRFSourceUnitDecode(t, wire), checks)
			for _, failure := range coverage.Failures {
				if checks.Keys[failure.Key] {
					return
				}
			}
			t.Fatal("invalid/unknown native source selector authorized coverage")
		})
	}
}
