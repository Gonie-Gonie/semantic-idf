package simulation

import (
	"encoding/json"
	"fmt"
	"testing"
)

func epathSQLNativeZoneEquipmentUnit(t *testing.T) (string, epathRealSQLModel, *PurposeRunPlan) {
	t.Helper()
	path, model, plan := epathSQLDirectHVACSourceUnitFixture(t)
	epathOracleEditSQL(t, path, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex IN (42,43,44);
DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex IN (42,43,44);
UPDATE Zones SET Multiplier=4,ListMultiplier=8;
UPDATE ReportData SET Value=2*3600000 WHERE ReportDataDictionaryIndex=10;
UPDATE ReportData SET Value=CASE WHEN TimeIndex=1 THEN 20*3600000 ELSE 0 END WHERE ReportDataDictionaryIndex=52;
INSERT INTO ReportDataDictionary VALUES(60,'Fan Electricity Energy','Local fan',0,'Monthly','J','System'),(61,'Baseboard Electricity Energy','Local baseboard',0,'Monthly','J','System'),(62,'Fans:Electricity','',1,'Monthly','J','Facility');
INSERT INTO ReportData SELECT 90000+60*20+t.Month,60,t.TimeIndex,CASE WHEN t.Month=1 THEN 10*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;
INSERT INTO ReportData SELECT 90000+61*20+t.Month,61,t.TimeIndex,CASE WHEN t.Month=1 THEN 20*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;
INSERT INTO ReportData SELECT 90000+62*20+t.Month,62,t.TimeIndex,CASE WHEN t.Month=1 THEN 10*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;`)
	model.DirectHVACComponents = model.DirectHVACComponents[:2]
	plan.OutputObjects = plan.OutputObjects[:2]
	for index := range model.DirectHVACComponents {
		model.DirectHVACComponents[index].Owners[0].EquipmentType = epathSQLWindowACType
	}
	for _, item := range []struct{ id, name, key, kind, parent, service, site string }{
		{epathSQLDirectFanFamily, "Fan Electricity Energy", "Local fan", "Fan:OnOff", epathSQLWindowACType, "fans", "fans.electricity"},
		{epathSQLConvectiveBaseboardFamily, "Baseboard Electricity Energy", "Local baseboard", epathSQLConvectiveBaseboardType, epathSQLConvectiveBaseboardType, "heating", "heating.electricity"},
	} {
		parent := "Native package"
		if item.parent == epathSQLConvectiveBaseboardType {
			parent = item.key
		}
		model.DirectHVACComponents = append(model.DirectHVACComponents, epathRealSQLDirectHVACComponent{ID: item.id, Service: item.service, Carrier: "electricity", SiteID: item.site, Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{item.key}}, Owners: []epathRealSQLDirectHVACOwner{{KeyValue: item.key, ZoneName: "Office", EquipmentType: item.parent, EquipmentName: parent, ComponentType: item.kind}}})
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: item.name, KeyValue: item.key, ReportingFrequency: "Monthly", ScopeZoneName: "Office", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	model.Site = append(model.Site, epathRealSQLSite{ID: "fans.electricity", EndUse: "fans", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Fans:Electricity", Unit: "J"}}, Keys: []string{"*"}, IsMeter: true}})
	model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "fans.electricity", ServedZones: []string{"Office"}, Weight: "native_direct", AllocationMethod: "direct_only", ReconciliationID: "reconcile.zone_auxiliary_allocation.fans.electricity.annual"}}
	return path, model, plan
}

func TestEnergyPathRealSQLNativeZoneEquipmentModelTotalAndFanBudget(t *testing.T) {
	path, model, plan := epathSQLNativeZoneEquipmentUnit(t)
	if err := epathSQLValidateDirectHVACRequests(plan, model); err != nil {
		t.Fatal(err)
	}
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	near := func(got, want float64) bool {
		return epathSQLZoneCarrierQuantityEqual(epathSQLQuantity{Value: got}, epathSQLQuantity{Value: want})
	}
	if frames.Zones["office"].Multiplier != 32 || !near(frames.SourceRaw[10][0].Value, 2) || frames.Loads[epathSQLKey("office", "cooling", 1)].Value != frames.SourceRaw[10][0].Value*32 || !near(frames.Loads[epathSQLKey("office", "cooling", 1)].Value, 64) {
		t.Fatalf("representative Zone load lost factor4x8 once: multiplier=%g, raw=%#.17g, effective=%#.17g", frames.Zones["office"].Multiplier, frames.SourceRaw[10][0].Value, frames.Loads[epathSQLKey("office", "cooling", 1)].Value)
	}
	for _, test := range []struct {
		service string
		value   float64
	}{{"cooling", 2.25}, {"heating", 20}, {"fans", 10}} {
		if !near(frames.DirectHVAC[epathSQLDirectHVACKey("office", test.service, "electricity", 1)].Quantity.Value, test.value) {
			t.Fatalf("native %s was multiplied or crossed consumption pools", test.service)
		}
	}
	fans, err := epathSQLCompileDirectFans(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	if !near(fans.Meter[0].Value, 10) || !near(fans.Direct[0].Value, 10) || !near(fans.Zones["office"][0].Value, 10) || len(fans.Sources["office"]) != 1 {
		t.Fatal("direct fan source boundaries changed")
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectFanChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 13*8 {
		t.Fatalf("missing fan monthly/annual ledger and Zone source fields: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.DirectFan != nil && check.Item.Period == "annual" && !near(check.Quantity.Value, 10) {
			t.Fatal("annual fan is not sum of original monthly observations")
		}
	}
	identity := frames.DirectHVACSourceIdentities[61]
	if err := epathSQLMatchDirectHVACSource(epathSQLDirectHVACSourceUnitCandidate(identity), identity); err != nil {
		t.Fatal(err)
	}
	bad := epathSQLDirectHVACSourceUnitCandidate(identity)
	bad.EffectiveValue *= 32
	bad.EffectiveMultiplier = 32
	bad.MultiplierApplication = "zone_multiplier"
	if err := epathSQLMatchDirectHVACSource(bad, identity); err == nil {
		t.Fatal("baseboard consuming source accepted a second multiplier")
	}
}

func TestEnergyPathRealSQLNativeFanMissingAndCohortAmbiguityFailClosed(t *testing.T) {
	for _, mutation := range []string{
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=60 AND TimeIndex=2`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=60 AND TimeIndex=2`,
		`INSERT INTO ReportDataDictionary VALUES(70,'Fan Electricity Energy','Local fan',0,'Monthly','J','System')`,
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=41 AND TimeIndex=2`,
	} {
		path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
		epathOracleEditSQL(t, path, mutation)
		observed, err := epathReadRealSQLOracle(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := epathCompileSQLModelFrames(path, observed.Sources, model); err == nil {
			t.Fatalf("accepted absent/ambiguous native cohort: %s", mutation)
		}
	}
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	for _, mutation := range []string{"missing", "foreign", "duplicate", "multiplied", "shared"} {
		copyFrames := frames
		copyFrames.DirectHVACSourceIdentities = map[int]epathSQLDirectHVACSourceIdentity{}
		for id, identity := range frames.DirectHVACSourceIdentities {
			copyFrames.DirectHVACSourceIdentities[id] = identity
		}
		copyModel := model
		identity := copyFrames.DirectHVACSourceIdentities[60]
		switch mutation {
		case "missing":
			delete(copyFrames.DirectHVACSourceIdentities, 60)
		case "foreign":
			identity.Owner.ZoneName = "Elsewhere"
			copyFrames.DirectHVACSourceIdentities[60] = identity
		case "duplicate":
			copyFrames.DirectHVACSourceIdentities[999] = identity
		case "multiplied":
			identity.Source.Months = append([]epathRealSQLMonth(nil), identity.Source.Months...)
			identity.Source.Months[0].EnergyKWh = epathOracleNumber(320)
			copyFrames.DirectHVACSourceIdentities[60] = identity
		case "shared":
			copyModel.FanPools = []epathRealSQLFanPool{{SiteID: "fans.electricity"}}
		}
		if _, err := epathSQLCompileDirectFans(copyFrames, copyModel); err == nil {
			t.Fatalf("native fan accepted %s proof", mutation)
		}
	}
}

func TestEnergyPathRealSQLNativeFanConsumerRejectsThermalAndForeignTrace(t *testing.T) {
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectFanChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	var selected epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.DirectFan != nil && check.Item.Period == "M1" && check.Item.Target.Field == "value" {
			selected = check
		}
	}
	// Hand-written native monthly fan=10, not values exported by the oracle.
	bundle := PurposeResultBundle{
		EnergyExplanation: EnergyExplanationResult{
			Schema:  energyExplanationSchema,
			Scope:   EnergyExplanationScope{Kind: "building"},
			Sources: []EnergyDataSource{epathSQLDirectHVACSourceUnitCandidate(frames.DirectHVACSourceIdentities[60])},
			ZoneResults: []EnergyExplanationZoneResult{
				{
					Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
					Periods: []EnergyPeriod{
						{
							ID: "M1", Kind: "monthly",
							Nodes: []EnergyExplanationNode{
								{ID: "fans", Level: "end_use", EndUse: "fans", Basis: "direct_zone_energy", Value: 10, RawValue: 10, EffectiveValue: 10, AllocatedValue: 10, Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", Period: "M1", ZoneName: "Office", SourceIDs: []string{"sql-rdd-60"}},
								{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: 10, Unit: "kWh", ScaleDomain: "site", Period: "M1", ZoneName: "Office"},
							},
							Links: []EnergyPathLink{
								{ID: "fan-carrier", FromID: "fans", ToID: "electricity", Relation: "direct_end_use_to_carrier", Basis: "direct_zone_energy", FromValue: 10, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh", Period: "M1", ZoneName: "Office", SourceIDs: []string{"sql-rdd-60"}},
							},
						},
					},
				},
			},
		},
	}
	if err := epathCheckSQLDirectFan(bundle, selected); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"foreign-source", "extra-thermal", "allocation", "double-multiplier", "missing-zero-source"} {
		var changed PurposeResultBundle
		if err := json.Unmarshal(encoded, &changed); err != nil {
			t.Fatal(err)
		}
		period := &changed.EnergyExplanation.ZoneResults[0].Periods[0]
		switch mutation {
		case "foreign-source":
			period.Links[0].SourceIDs = append(period.Links[0].SourceIDs, "sql-rdd-40")
		case "extra-thermal":
			period.Links = append(period.Links, EnergyPathLink{ID: "load-to-fan", FromID: "electricity", ToID: "fans", Relation: "load_to_end_use"})
		case "allocation":
			period.Nodes[0].AllocationApplied = true
		case "double-multiplier":
			period.Nodes[0].EffectiveValue = 320
		case "missing-zero-source":
			changed.EnergyExplanation.Sources = nil
		}
		if err := epathCheckSQLDirectFan(changed, selected); err == nil {
			t.Fatal(fmt.Sprintf("accepted incorrect native fan consumer %s", mutation))
		}
	}
}
