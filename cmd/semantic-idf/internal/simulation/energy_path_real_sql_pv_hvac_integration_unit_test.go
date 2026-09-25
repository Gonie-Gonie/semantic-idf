package simulation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPVHVACStampFixture(t *testing.T) (epathRealSQLModel, epathSQLModelChecks) {
	t.Helper()
	pools, services, auxiliaries := epathSQLPVHVACUnitInputs()
	model := epathRealSQLModel{Schema: "semantic-idf.energy-path-sql-model/large-office-monthly/v1", PVSystems: []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}, PVHVACLoops: epathSQLPVHVACOriginalDeclarations(), FanPools: pools, Services: services, Auxiliaries: auxiliaries, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
	original := epathSQLPVOriginalFixture(t)
	observed := epathRealOracleEvidence{originalText: original, executedText: "Output:Variable,*,Zone Mean Air Temperature,Hourly;\n" + original}
	for n := range model.Services {
		model.Services[n].ReconciliationID = "allocation." + model.Services[n].Service + ".annual"
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLBindPVHVACChecks(observed, model, &checks); err != nil {
		t.Fatal(err)
	}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}, LoadSourceIDs: map[string][]int{}, SourceRaw: map[int][]epathSQLQuantity{}}
	for n, declaration := range model.PVHVACLoops {
		zone := strings.ToLower(declaration.ZoneName)
		frames.Zones[zone] = epathSQLZone{Name: declaration.ZoneName, Multiplier: 1}
		for s, service := range model.Services {
			id := 100 + n*2 + s
			frames.SourceIdentities[id] = epathRealSQLSource{DictionaryIndex: id, Name: service.Service + " hand delivered load", KeyValue: declaration.ZoneName, ReportingFrequency: "Monthly", SourceUnit: "J"}
			for month := 1; month <= 12; month++ {
				frames.Loads[epathSQLKey(zone, service.Service, month)] = epathSQLQuantity{Value: float64(n + 1)}
				frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)] = []int{id}
			}
		}
	}
	for n, service := range model.Services {
		id := n + 1
		site := epathRealSQLSite{ID: service.SiteIDs[0], EndUse: service.Service, Carrier: "electricity"}
		model.Site = append(model.Site, site)
		frames.SiteSources[site.ID] = []int{id}
		frames.SourceIdentities[id] = epathRealSQLSource{DictionaryIndex: id, Name: service.Service + " hand meter", IsMeter: true, ReportingFrequency: "Monthly", SourceUnit: "J"}
		for month := 1; month <= 12; month++ {
			q := epathSQLQuantity{Value: 20}
			frames.Site[site.ID] = append(frames.Site[site.ID], &q)
			frames.SourceRaw[id] = append(frames.SourceRaw[id], q)
		}
	}
	// Actual generic service APIs establish the 676 service selectors. This
	// fixture does not copy a successful candidate or synthesize its values.
	// The separate SHW auxiliary remains in the physical stamp, but its
	// numeric ledger is not one of these two service builders' 676 selectors.
	serviceModel := model
	serviceModel.Auxiliaries = nil
	if err := epathSQLModelServiceChecks(frames, serviceModel, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, serviceModel, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 676 {
		t.Fatalf("generic two-service compiler selector contract changed: %d", len(checks.Rows))
	}
	// The independent fan SQL/calendar tests retain numerical authority. Here
	// hand fan targets exercise only the retained original-owner stamp gate.
	periods := []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"}
	zero := epathSQLQuantity{}
	for _, pool := range pools {
		for _, field := range []string{"rawValue", "effectiveValue"} {
			target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: strings.ToUpper(pool.Name), SourceKey: strings.ToUpper(pool.Key), SourceUnit: "J", Frequency: "Hourly", Unit: "kWh"}
			if err := checks.add("endUses", "building", "", "annual", "fan_pool/"+pool.Key+"/"+field, "kWh", &zero, target, "", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		for _, period := range periods {
			target := epathSQLNodeTarget("end_use", "fans", "", "site")
			target.Basis, target.AllowPrunedZero = "service_path_allocation", true
			if err := checks.add("zoneAllocation", "zone", pool.ServedZones[0], period, "fan_pools/value", "kWh", &zero, target, "", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	checks.FanPoolTotals = map[string]epathSQLFanPoolTotal{"fans.electricity": {Pools: pools}}
	// Binding-only hand anchor. Complete native SQL/source authority belongs to
	// the separate PV registry tests; this fixture never calls their evaluator.
	checks.RequiredPVSystems = epathSQLPVCloneDeclarations(model.PVSystems)
	checks.PVSourceRegistry = &epathSQLPVSourceRegistry{Native: []epathSQLPVSourceFrames{{Original: epathSQLPVOriginalProof{Declaration: checks.RequiredPVSystems[0], OriginalSHA256: checks.PVHVAC.Original.OriginalSHA256}, ExecutedSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(observed.executedText)))}}}
	if len(checks.Rows) != 751 {
		t.Fatalf("finite HVAC selector census changed: %d", len(checks.Rows))
	}
	if err := epathSQLSealPVHVACChecks(&checks, model); err != nil {
		t.Fatal(err)
	}
	return model, checks
}

func epathSQLPVHVACCopyChecks(t *testing.T, input epathSQLModelChecks) epathSQLModelChecks {
	t.Helper()
	out := input
	out.Rows = append([]epathSQLModelCheck(nil), input.Rows...)
	out.Keys = map[string]bool{}
	for key, value := range input.Keys {
		out.Keys[key] = value
	}
	if input.PVHVAC != nil {
		data, err := json.Marshal(input.PVHVAC)
		if err != nil {
			t.Fatal(err)
		}
		out.PVHVAC = &epathSQLPVHVACStamp{}
		if err := json.Unmarshal(data, out.PVHVAC); err != nil {
			t.Fatal(err)
		}
	}
	if input.PVSourceRegistry != nil {
		data, err := json.Marshal(input.PVSourceRegistry)
		if err != nil {
			t.Fatal(err)
		}
		out.PVSourceRegistry = &epathSQLPVSourceRegistry{}
		if err := json.Unmarshal(data, out.PVSourceRegistry); err != nil {
			t.Fatal(err)
		}
	}
	out.FanPoolTotals = map[string]epathSQLFanPoolTotal{}
	for key, total := range input.FanPoolTotals {
		out.FanPoolTotals[key] = total
	}
	return out
}

func TestEnergyPathSQLPVHVACStampGenericServiceCensusAndDetachedInputs(t *testing.T) {
	model, checks := epathSQLPVHVACStampFixture(t)
	if err := epathSQLPreparePVHVACChecks(checks, &model); err != nil {
		t.Fatal(err)
	}
	foreign := model
	foreign.AirLoopFans = []epathRealSQLAirLoopFan{{}}
	if err := epathSQLPreparePVHVACChecks(checks, &foreign); err == nil {
		t.Fatal("external full model introduced a competing HVAC ownership path")
	}
	if checks.PVHVAC.Original.Objects[0].ObjectIndex == checks.PVHVAC.Executed[0].ObjectIndex {
		t.Fatal("executed object indices borrowed original identity")
	}
	before, err := epathSQLPVHVACStampDigest(checks.PVHVAC)
	if err != nil {
		t.Fatal(err)
	}
	model.FanPools[0].ServedZones[0] = "caller mutation"
	model.Services[0].ServedZones[0] = "caller mutation"
	model.Services[0].SiteIDs[0] = "caller mutation"
	model.Auxiliaries[0].ServedZones = []string{"caller mutation"}
	model.PVHVACLoops[0].FanName = "caller mutation"
	model.PVSystems[0].Generators[0].Name = "caller mutation"
	after, err := epathSQLPVHVACStampDigest(checks.PVHVAC)
	if err != nil || before != after {
		t.Fatal("retained stamp aliases caller declarations")
	}
	if err := epathSQLPreparePVHVACChecks(checks, &model); err == nil {
		t.Fatal("changed external full model accepted")
	}
}

func TestEnergyPathSQLPVHVACStampRejectsRemovedOrForgedEvidence(t *testing.T) {
	model, original := epathSQLPVHVACStampFixture(t)
	for _, tc := range []struct {
		name   string
		mutate func(*epathSQLModelChecks)
	}{
		{"required flag", func(c *epathSQLModelChecks) { c.PVHVACRequired = false }},
		{"stamp", func(c *epathSQLModelChecks) { c.PVHVAC = nil }},
		{"both local anchors", func(c *epathSQLModelChecks) { c.PVHVACRequired, c.PVHVAC = false, nil }},
		{"all local evidence", func(c *epathSQLModelChecks) { *c = epathSQLModelChecks{} }},
		{"declarations", func(c *epathSQLModelChecks) { c.PVHVAC.Inputs.Loops = nil }},
		{"original index", func(c *epathSQLModelChecks) { c.PVHVAC.Original.Objects[0].ObjectIndex++ }},
		{"executed index", func(c *epathSQLModelChecks) { c.PVHVAC.Executed[0].ObjectIndex++ }},
		{"physical field", func(c *epathSQLModelChecks) { c.PVHVAC.Executed[0].Fields[0] = "forged" }},
		{"original text", func(c *epathSQLModelChecks) { c.PVHVAC.OriginalText = "" }},
		{"executed text", func(c *epathSQLModelChecks) { c.PVHVAC.ExecutedText = "" }},
		{"row stamp", func(c *epathSQLModelChecks) { c.Rows[0].PVHVACProofSHA256 = "" }},
		{"required key", func(c *epathSQLModelChecks) { delete(c.Keys, c.Rows[0].Want.Key) }},
		{"row and key", func(c *epathSQLModelChecks) { delete(c.Keys, c.Rows[0].Want.Key); c.Rows = c.Rows[1:] }},
		{"sealed row roster", func(c *epathSQLModelChecks) { c.PVHVAC.RowDigests = nil }},
		{"selector", func(c *epathSQLModelChecks) { c.Rows[0].Item.Target.RatioKind = "borrowed" }},
		{"typed proof", func(c *epathSQLModelChecks) {
			for n := range c.Rows {
				if c.Rows[n].Conversion != nil {
					c.Rows[n].Conversion = nil
					return
				}
			}
		}},
		{"fan audited partition", func(c *epathSQLModelChecks) { delete(c.FanPoolTotals, "fans.electricity") }},
		{"missing fan recipient", func(c *epathSQLModelChecks) { c.PVHVAC.Inputs.FanPools[0].ServedZones = nil }},
		{"native original anchor", func(c *epathSQLModelChecks) {
			c.PVSourceRegistry.Native[0].Original.OriginalSHA256 = strings.Repeat("f", 64)
		}},
		{"native executed anchor", func(c *epathSQLModelChecks) { c.PVSourceRegistry.Native[0].ExecutedSHA256 = strings.Repeat("f", 64) }},
		{"native registry", func(c *epathSQLModelChecks) { c.PVSourceRegistry = nil }},
		{"native required family", func(c *epathSQLModelChecks) { c.RequiredPVSystems = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := epathSQLPVHVACCopyChecks(t, original)
			tc.mutate(&changed)
			if err := epathSQLPreparePVHVACChecks(changed, &model); err == nil {
				t.Fatal("removed/forged full-model HVAC evidence accepted")
			}
		})
	}
	// Rehashing a forged physical stamp does not turn it into original proof.
	changed := epathSQLPVHVACCopyChecks(t, original)
	changed.PVHVAC.Executed[0].ObjectIndex++
	changed.PVHVAC.Digest, _ = epathSQLPVHVACStampDigest(changed.PVHVAC)
	for n := range changed.Rows {
		changed.Rows[n].PVHVACProofSHA256 = epathSQLPVHVACRowStamp(changed.Rows[n], changed.PVHVAC.Digest)
	}
	if err := epathSQLPreparePVHVACChecks(changed, &model); err == nil {
		t.Fatal("rehashed forged executed object index accepted")
	}
}

func TestEnergyPathSQLPVHVACApplicabilityKeepsNativeSourceDiagnosticsSeparate(t *testing.T) {
	model := epathRealSQLModel{PVSystems: []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}}
	sourceOnly := epathSQLModelChecks{RequiredPVSystems: epathSQLPVCloneDeclarations(model.PVSystems)}
	if err := epathSQLPreparePVHVACChecks(sourceOnly); err != nil {
		t.Fatal("standalone native-source registry was forced into a full HVAC model")
	}
	if err := epathSQLPreparePVHVACChecks(sourceOnly, &model); err != nil {
		t.Fatal("schema-less external PV/CG source model was forced into a full HVAC model")
	}
	if err := epathSQLBindPVHVACChecks(epathRealOracleEvidence{}, model, &epathSQLModelChecks{}); err == nil {
		t.Fatal("full-model compile accepted deleted HVAC declarations")
	}
	model.Schema = "semantic-idf.energy-path-sql-model/large-office-monthly/v1"
	if err := epathSQLPreparePVHVACChecks(sourceOnly, &model); err == nil {
		t.Fatal("external full-model PV anchor lost its mandatory HVAC proof")
	}
	legacy := epathSQLModelChecks{}
	if err := epathSQLBindPVHVACChecks(epathRealOracleEvidence{}, epathRealSQLModel{}, &legacy); err != nil || !reflect.DeepEqual(legacy, epathSQLModelChecks{}) {
		t.Fatal("legacy absent-family behavior changed")
	}
	if err := epathSQLPreparePVHVACChecks(legacy, &epathRealSQLModel{}); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLPreparePVHVACChecks(legacy, &model, &model); err == nil {
		t.Fatal("ambiguous external full-model authority accepted")
	}
}

func TestEnergyPathSQLPVHVACCompileFinalRejectsOmittedGenericBuilderSelectors(t *testing.T) {
	model, original := epathSQLPVHVACStampFixture(t)
	for _, family := range []string{"fan_pool/", "fan_pools/value", "hvac_zone/heating/", "cooling/service_path_allocation", "heating/expectedValue"} {
		t.Run(family, func(t *testing.T) {
			changed := epathSQLPVHVACCopyChecks(t, original)
			changed.PVHVAC.RowDigests = nil
			for n := range changed.Rows {
				changed.Rows[n].PVHVACProofSHA256 = ""
			}
			found := false
			for n, row := range changed.Rows {
				if strings.Contains(row.Want.Key, family) {
					delete(changed.Keys, row.Want.Key)
					changed.Rows = append(changed.Rows[:n], changed.Rows[n+1:]...)
					found = true
					break
				}
			}
			if !found {
				t.Fatal(fmt.Sprintf("hand fixture lacks %s", family))
			}
			if err := epathSQLSealPVHVACChecks(&changed, model); err == nil {
				t.Fatal("compiler omission was blessed by a new stamp")
			}
		})
	}
}

func TestEnergyPathSQLPVHVACExternalFullModelSurvivesAllLocalDeletion(t *testing.T) {
	// Every check-local flag, stamp, row, key and source registry has vanished.
	// The external recipe still requires the finite family's full-model proof.
	model := epathRealSQLModel{Schema: "semantic-idf.energy-path-sql-model/large-office-monthly/v1", PVSystems: []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}}
	empty := epathSQLModelChecks{}
	out := epathRealOracleEvidence{}
	failures := epathEvaluateSQLModelChecks(&out, PurposeResultBundle{}, empty, &model)
	if len(failures) == 0 || !strings.Contains(failures[0].Message, "Shop HVAC") || len(out.CheckedGroups) != 0 {
		t.Fatal("evaluation lost the external full-model HVAC obligation")
	}
	coverage := epathSQLModelCoverage(PurposeResultBundle{}, empty, &model)
	found := false
	for _, failure := range coverage.Failures {
		found = found || strings.Contains(failure.Message, "Shop HVAC")
	}
	if !found {
		t.Fatal("standalone coverage lost the external full-model HVAC obligation")
	}
	input := epathOraclePendingUnitFixture(t)
	input.recipe.SQLModel = &model
	if err := input.write(); err == nil || !strings.Contains(err.Error(), "Shop HVAC") {
		t.Fatalf("pending export lost external HVAC obligation: %v", err)
	}
	if _, err := os.Stat(input.destination); !os.IsNotExist(err) {
		t.Fatal("pending denial wrote a review artifact")
	}
}

func TestEnergyPathSQLPVHVACStampPreservesIndependentCanonicalSourceRows(t *testing.T) {
	model, checks := epathSQLPVHVACStampFixture(t)
	checks.PVHVAC.RowDigests = nil
	for n := range checks.Rows {
		checks.Rows[n].PVHVACProofSHA256 = ""
	}
	zero := epathSQLQuantity{}
	target := epathRealOracleTarget{Collection: "sources", Field: "rawValue", SourceName: "hand native identity", SourceKey: "hand", SourceUnit: "J", Frequency: "Monthly", Unit: "kWh"}
	if err := checks.add("loads", "building", "", "annual", "pv_native_source/hand/rawValue", "kWh", &zero, target, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	index := len(checks.Rows) - 1
	checks.Rows[index].PVSource = &epathSQLPVSourceProof{SystemID: "hand", SpecID: "hand", Field: "rawValue", DictionaryIndex: 1}
	before := checks.Rows[index]
	if err := epathSQLSealPVHVACChecks(&checks, model); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, checks.Rows[index]) {
		t.Fatal("HVAC stamp changed an independent canonical native source row")
	}
	if err := epathSQLPreparePVHVACChecks(checks, &model); err != nil {
		t.Fatal(err)
	}
}
