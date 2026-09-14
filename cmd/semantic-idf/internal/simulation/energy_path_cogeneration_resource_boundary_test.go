package simulation

// Literal boundary tests. No SQL transactions, engine or candidate data.
import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func cogenerationDraftInventory(t *testing.T, extra string) energyPathCogenerationInventory {
	t.Helper()
	doc, err := idf.Parse("Version,25.1; ElectricLoadCenter:Distribution,LC,,,,,,DirectCurrentWithInverterDCStorage,INV; ElectricLoadCenter:Inverter:LookUpTable,INV; " + extra)
	if err != nil {
		t.Fatal(err)
	}
	return energyPathBuildCogenerationInventory(doc)
}
func cogenerationDraftSource(resource string) EnergyDataSource {
	return EnergyDataSource{ID: "sql-rdd-51", SourceType: "sql_report_data", IsMeter: true, Name: "Cogeneration:" + resource, Units: "J", SourceUnit: "J", ReportingFrequency: "Monthly"}
}
func cogenerationDraftSeries(source EnergyDataSource) energyExplanationSeries {
	return canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: "energy.generators", EndUse: "generators", Carrier: "electricity", Unit: "kWh", SourceIDs: []string{source.ID}, sourceName: source.Name, sourceKeyValue: source.KeyValue, sourceFrequency: source.ReportingFrequency, Total: 10, Monthly: map[int]float64{1: 0, 2: 10}})
}

func TestEnergyPathCogenerationNativeConsumedResourceSurvivesCanonicalization(t *testing.T) {
	inventory := cogenerationDraftInventory(t, "")
	for _, tc := range []struct{ resource, carrier string }{{"Electricity", "electricity"}, {"NaturalGas", "natural_gas"}, {"Propane", "propane"}, {"Diesel", "diesel"}, {"Gasoline", "gasoline"}, {"Coal", "coal"}, {"FuelOilNo1", "fuel_oil_1"}, {"FuelOilNo2", "fuel_oil_2"}, {"OtherFuel1", "other_fuel_1"}, {"OtherFuel2", "other_fuel_2"}} {
		t.Run(tc.resource, func(t *testing.T) {
			source := cogenerationDraftSource(tc.resource)
			before := source
			item := cogenerationDraftSeries(source)
			actual, b := qualifyEnergyPathCogenerationMeter(item, source, []EnergyDataSource{source}, inventory)
			actual = canonicalEnergyExplanationSeries(actual)
			if b == nil || b.Role != energyPathCogenerationConsumed || b.NonFlow || actual.Stage != "end_use" || actual.Level != "energy" || actual.EndUse != "cogeneration_input" || actual.CanonicalKind != "energy.cogeneration_input" || actual.Carrier != tc.carrier || energyExplanationIsSupportEndUse(actual) {
				t.Fatalf("native consumed resource became supply: %#v %#v", actual, b)
			}
			if actual.Total != item.Total || actual.RawTotal != item.RawTotal || !reflect.DeepEqual(actual.Monthly, item.Monthly) || !reflect.DeepEqual(actual.RawMonthly, item.RawMonthly) || !reflect.DeepEqual(actual.SourceIDs, item.SourceIDs) || !reflect.DeepEqual(source, before) {
				t.Fatal("resource qualification changed native quantity/presence/provenance")
			}
		})
	}
}

func TestEnergyPathCogenerationNonConsumedResourcesAreExplicit(t *testing.T) {
	inventory := cogenerationDraftInventory(t, "")
	for _, tc := range []struct {
		resource, role, endUse string
		nonFlow                bool
	}{
		{"ElectricityProduced", energyPathCogenerationProduced, "generators", false},
		{"ElectricityPurchased", energyPathCogenerationPurchased, "electricity_purchased", false},
		{"ElectricitySurplusSold", energyPathCogenerationSold, "electricity_sold", false},
		{"ElectricityNet", energyPathCogenerationNet, "cogeneration_resource_context", true},
		{"EnergyTransfer", energyPathCogenerationThermal, "cogeneration_resource_context", true},
		{"UnreviewedResource", energyPathCogenerationUnresolved, "cogeneration_resource_context", true},
	} {
		t.Run(tc.resource, func(t *testing.T) {
			source := cogenerationDraftSource(tc.resource)
			item := cogenerationDraftSeries(source)
			if tc.resource == "ElectricityNet" {
				item.Total = -3
				item.RawTotal = -3
				item.Monthly = map[int]float64{1: -3}
				item.RawMonthly = map[int]float64{1: -3}
			}
			actual, b := qualifyEnergyPathCogenerationMeter(item, source, []EnergyDataSource{source}, inventory)
			if b == nil || b.Role != tc.role || b.NonFlow != tc.nonFlow || actual.EndUse != tc.endUse || actual.Stage == "end_use" || actual.Total != item.Total || !reflect.DeepEqual(actual.Monthly, item.Monthly) {
				t.Fatalf("resource direction/sign lost: %#v %#v", actual, b)
			}
			for i := 0; i < 2; i++ {
				raw, err := json.Marshal(b)
				if err != nil {
					t.Fatal(err)
				}
				var decoded energyPathCogenerationResourceBoundary
				if err := json.Unmarshal(raw, &decoded); err != nil {
					t.Fatal(err)
				}
				if decoded != *b {
					t.Fatal("typed resource role lost on metadata reload")
				}
				b = &decoded
			}
			if energyPathCogenerationBoundaryIsNonFlow(b) != tc.nonFlow {
				t.Fatal("role-based non-flow predicate differs")
			}
			if tc.nonFlow {
				b.NonFlow = false
				if !energyPathCogenerationBoundaryIsNonFlow(b) {
					t.Fatal("missing false-default boolean erased context-role denial")
				}
			}
			// This is annotation transport only: active graph/reload filters must
			// consume NonFlow rather than defaulting Stage=support to production.
		})
	}
}

func TestEnergyPathCogenerationNativeIdentityAndCustomMeterFailClosed(t *testing.T) {
	inventory := cogenerationDraftInventory(t, "")
	source := cogenerationDraftSource("Electricity")
	item := cogenerationDraftSeries(source)
	for _, tc := range []struct {
		name   string
		change func(*EnergyDataSource)
	}{
		{"not meter", func(s *EnergyDataSource) { s.IsMeter = false }},
		{"custom key", func(s *EnergyDataSource) { s.KeyValue = "custom key" }},
		{"rate unit", func(s *EnergyDataSource) { s.SourceUnit = "W" }},
		{"not SQL", func(s *EnergyDataSource) { s.SourceType = "series" }},
		{"wrong name", func(s *EnergyDataSource) { s.Name = "Generators:ElectricityProduced" }},
		{"wrong frequency", func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := source
			tc.change(&changed)
			actual, b := qualifyEnergyPathCogenerationMeter(item, changed, []EnergyDataSource{changed}, inventory)
			if b == nil || b.Role != energyPathCogenerationUnresolved || !b.NonFlow || actual.Stage == "end_use" || actual.Total != item.Total {
				t.Fatal("unproved native identity became consumed or generated input")
			}
		})
	}
	for _, roster := range [][]EnergyDataSource{nil, {source, source}, func() []EnergyDataSource { other := source; other.ID = "sql-rdd-52"; return []EnergyDataSource{other} }()} {
		_, b := qualifyEnergyPathCogenerationMeter(item, source, roster, inventory)
		if b == nil || !b.NonFlow {
			t.Fatal("missing/duplicate/foreign actual source accepted")
		}
	}
	for _, extra := range []string{"Meter:Custom,Cogeneration:Electricity,Electricity;", "Meter:CustomDecrement,Cogeneration:Electricity,Electricity,Electricity:Facility;", "Version,25.1;"} {
		_, b := qualifyEnergyPathCogenerationMeter(item, source, []EnergyDataSource{source}, cogenerationDraftInventory(t, extra))
		if b == nil || !b.NonFlow {
			t.Fatal("custom collision/ambiguous version accepted")
		}
	}
	legacy, b := qualifyEnergyPathCogenerationMeter(item, source, nil, energyPathCogenerationInventory{})
	if b != nil || !reflect.DeepEqual(legacy, item) {
		t.Fatal("omitted-original legacy API changed")
	}
	_, b = qualifyEnergyPathCogenerationMeter(item, source, []EnergyDataSource{source}, energyPathBuildCogenerationInventory(idf.Document{}))
	if b == nil || !b.NonFlow {
		t.Fatal("explicit empty original became omitted original")
	}
}

func TestEnergyPathCogenerationNativeTabularOverrideBeforeLegacySupportDrop(t *testing.T) {
	inventory := cogenerationDraftInventory(t, "")
	row := energyPathCogenerationTabularIdentity{SourceID: "tabular-cogeneration-electricity", ReportName: "AnnualBuildingUtilityPerformanceSummary", ReportForString: "Entire Facility", TableName: "End Uses", RowName: "Generators", ColumnName: "Electricity", Unit: "GJ"}
	for _, column := range []string{"Electricity", "Natural Gas", "Propane", "Fuel Oil No 1"} {
		t.Run(column, func(t *testing.T) {
			r := row
			r.ColumnName = column
			def, b, handled := qualifyEnergyPathCogenerationTabular(r, []energyPathCogenerationTabularIdentity{r}, inventory)
			if !handled || b == nil || b.Role != energyPathCogenerationConsumed || b.NonFlow || def.EndUse != "cogeneration_input" {
				t.Fatal("native consumed table row became legacy production alias")
			}
			item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: def.Kind, EndUse: def.EndUse, Carrier: def.Carrier, Unit: "kWh", Total: 0, SourceIDs: []string{r.SourceID}, sourceFrequency: "Annual"})
			if item.Stage != "end_use" || energyExplanationIsSupportEndUse(item) {
				t.Fatal("native Generators row will be dropped by old support filter")
			}
		})
	}
	for _, change := range []func(*energyPathCogenerationTabularIdentity){
		func(r *energyPathCogenerationTabularIdentity) { r.ReportName = "OtherSummary" },
		func(r *energyPathCogenerationTabularIdentity) { r.ReportForString = "One Zone" },
		func(r *energyPathCogenerationTabularIdentity) { r.TableName = "End Uses By Subcategory" },
		func(r *energyPathCogenerationTabularIdentity) { r.ColumnName = "Water" },
		func(r *energyPathCogenerationTabularIdentity) { r.Unit = "W" },
	} {
		changed := row
		change(&changed)
		_, b, handled := qualifyEnergyPathCogenerationTabular(changed, []energyPathCogenerationTabularIdentity{changed}, inventory)
		if !handled || b == nil || !b.NonFlow {
			t.Fatal("noncanonical table context accepted as consumed input")
		}
	}
	_, b, handled := qualifyEnergyPathCogenerationTabular(row, []energyPathCogenerationTabularIdentity{row, row}, inventory)
	if !handled || b == nil || !b.NonFlow {
		t.Fatal("duplicate native table cell accepted")
	}
	if _, b, handled := qualifyEnergyPathCogenerationTabular(row, nil, energyPathCogenerationInventory{}); handled || b != nil {
		t.Fatal("no-original legacy table API changed")
	}
}

func TestEnergyPathCogenerationSoleInverterRequiresWholeFiniteCensus(t *testing.T) {
	source := EnergyDataSource{ID: "sql-rdd-52", SourceType: "sql_report_data", Name: "Inverter Ancillary AC Electricity Energy", KeyValue: "INV", SourceUnit: "J", Units: "J", ReportingFrequency: "Monthly"}
	if !energyPathCogenerationSoleInverterSource(source, []EnergyDataSource{source}, cogenerationDraftInventory(t, "Generator:Photovoltaic,PV;")) {
		t.Fatal("finite native sole inverter not recognized")
	}
	for _, extra := range []string{
		"ElectricLoadCenter:Inverter:LookUpTable,Other;",
		"ElectricLoadCenter:Storage:Converter,CONVERTER;",
		"Generator:MicroTurbine,StandbyConsumer;",
		"Generator:InternalCombustionEngine,FuelConsumer;",
		"ElectricLoadCenter:Inverter:Simple,Other;",
		"ElectricLoadCenter:Distribution,Other;",
		"Meter:Custom,CustomCogeneration,Electricity;",
	} {
		if energyPathCogenerationSoleInverterSource(source, []EnergyDataSource{source}, cogenerationDraftInventory(t, extra)) {
			t.Fatalf("incomplete whole native consumption census accepted: %s", extra)
		}
	}
	for _, name := range []string{"Generator Ancillary Electricity Energy", "Generator Standby Electricity Energy", "Converter Ancillary AC Electricity Energy"} {
		other := source
		other.Name = name
		if energyPathCogenerationSoleInverterSource(other, []EnergyDataSource{other}, cogenerationDraftInventory(t, "")) {
			t.Fatal("ancillary label became blanket consumption fallback")
		}
	}
}

func TestEnergyPathCogenerationParentAndAncillaryAreOneBudget(t *testing.T) {
	known := func(value float64) *energyPathCogenerationBudgetObservation {
		return &energyPathCogenerationBudgetObservation{Known: true, Value: value}
	}
	unknown := &energyPathCogenerationBudgetObservation{}
	for _, tc := range []struct {
		name           string
		parent, member *energyPathCogenerationBudgetObservation
		sole           bool
		want           energyPathCogenerationBudgetChoice
	}{
		{"parent10 member10", known(10), known(10), true, energyPathCogenerationBudgetChoice{UseParent: true, Reason: "observed_parent_budget"}},
		{"parent10 memberabsent", known(10), nil, true, energyPathCogenerationBudgetChoice{UseParent: true, Reason: "observed_parent_budget"}},
		{"parent10 memberNULL", known(10), unknown, true, energyPathCogenerationBudgetChoice{UseParent: true, Reason: "observed_parent_budget"}},
		{"parentzero", known(0), known(0), true, energyPathCogenerationBudgetChoice{UseParent: true, Reason: "observed_parent_budget"}},
		{"absent parent sole member", nil, known(10), true, energyPathCogenerationBudgetChoice{UseSoleMember: true, Reason: "observed_sole_native_member"}},
		{"absent parent sole zero", nil, known(0), true, energyPathCogenerationBudgetChoice{UseSoleMember: true, Reason: "observed_sole_native_member"}},
		{"NULL parent", unknown, known(10), true, energyPathCogenerationBudgetChoice{Reason: "parent_budget_unknown"}},
		{"negative parent", known(-1), known(10), true, energyPathCogenerationBudgetChoice{Reason: "parent_budget_unknown"}},
		{"nonfinite parent", known(math.Inf(1)), known(10), true, energyPathCogenerationBudgetChoice{Reason: "parent_budget_unknown"}},
		{"unproved member roster", nil, known(10), false, energyPathCogenerationBudgetChoice{Reason: "native_budget_unassigned"}},
		{"NULL member", nil, unknown, true, energyPathCogenerationBudgetChoice{Reason: "native_budget_unassigned"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			choice := chooseEnergyPathCogenerationBudget(tc.parent, tc.member, tc.sole)
			if choice != tc.want {
				t.Fatalf("choice %#v want %#v", choice, tc.want)
			}
			if choice.UseParent && choice.UseSoleMember {
				t.Fatal("parent/member double count")
			}
		})
	}
	parent, member := known(10), known(10)
	choice := chooseEnergyPathCogenerationBudget(parent, member, true)
	explained := 90.0
	if choice.UseParent {
		explained += parent.Value
	}
	if choice.UseSoleMember {
		explained += member.Value
	}
	if explained != 100 || parent.Value != 10 || member.Value != 10 {
		t.Fatal("Facility100/ordinary90/parent10/member10 must explain100 without changing observations")
	}
}

func TestEnergyPathCogenerationEMSMeteredInputDeniesOnlySoleFallback(t *testing.T) {
	member := EnergyDataSource{ID: "sql-rdd-52", SourceType: "sql_report_data", Name: "Inverter Ancillary AC Electricity Energy", KeyValue: "INV", SourceUnit: "J", Units: "J", ReportingFrequency: "Monthly"}
	parent := cogenerationDraftSource("Electricity")
	for _, endUse := range []string{"OnSiteGeneration", "onsitegeneration", "Cogeneration"} {
		t.Run(endUse, func(t *testing.T) {
			extra := "EnergyManagementSystem:MeteredOutputVariable, ExtraConsumption, ErlVar, SystemTimestep,, Electricity,Plant," + endUse + ",,J;"
			inventory := cogenerationDraftInventory(t, extra)
			if inventory.SoleInverterKey != "" || energyPathCogenerationSoleInverterSource(member, []EnergyDataSource{member}, inventory) {
				t.Fatal("EMS metered input escaped the native electricity consumer census")
			}
			actual, b := qualifyEnergyPathCogenerationMeter(cogenerationDraftSeries(parent), parent, []EnergyDataSource{parent, member}, inventory)
			if b == nil || b.Role != energyPathCogenerationConsumed || b.NonFlow || actual.EndUse != "cogeneration_input" {
				t.Fatal("extra EMS consumer invalidated the observed native parent")
			}
			choice := chooseEnergyPathCogenerationBudget(nil, &energyPathCogenerationBudgetObservation{Known: true, Value: 10}, energyPathCogenerationSoleInverterSource(member, []EnergyDataSource{member}, inventory))
			if choice.UseSoleMember || choice.UseParent {
				t.Fatal("absent parent was inferred from one of two consumers")
			}
		})
	}
	for _, extra := range []string{
		"EnergyManagementSystem:MeteredOutputVariable, ExtraConsumption, ErlVar, SystemTimestep,, Electricity,Building,InteriorEquipment,,J;",
		"EnergyManagementSystem:MeteredOutputVariable, ExtraConsumption, ErlVar, SystemTimestep,, NaturalGas,Plant,OnSiteGeneration,,J;",
	} {
		if !energyPathCogenerationSoleInverterSource(member, []EnergyDataSource{member}, cogenerationDraftInventory(t, extra)) {
			t.Fatal("unrelated EMS resource/end-use became an electricity parent constituent")
		}
	}
}
