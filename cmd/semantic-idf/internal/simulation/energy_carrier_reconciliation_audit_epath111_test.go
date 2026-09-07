package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestEPATH111FacilityCarrierReconciliationExcludesSupplyActivities(t *testing.T) {
	result := UpgradeEnergyExplanationV1(epath111LegacyCarrierFixture(100, 90, nil))

	carriers := epath111NodesAtLevel(result.Nodes, "carrier")
	if len(carriers) != 1 || carriers[0].ID != "carrier.electricity.building" || carriers[0].Carrier != "electricity" || carriers[0].Value != 100 {
		t.Fatalf("main graph electricity carriers = %#v; want one 100 kWh Electricity carrier", carriers)
	}
	carrier := carriers[0]

	wantSupply := map[string]float64{
		"generators":            25,
		"storage_discharge":     8,
		"electricity_purchased": 120,
		"electricity_sold":      15,
	}
	gotSupply := map[string]float64{}
	for _, node := range epath111NodesAtLevel(result.Nodes, "support") {
		if node.Value <= 0 || node.DisplayValue < 0 {
			t.Errorf("support activity must retain a positive magnitude: %#v", node)
		}
		gotSupply[node.EndUse] += node.Value
		link := epath111Link(result.Links, node.ID, carrier.ID, "support_supply")
		if link == nil {
			t.Errorf("support activity %q has no support_supply link to the single Electricity carrier", node.EndUse)
		} else if !reflect.DeepEqual(link.SourceIDs, node.SourceIDs) {
			t.Errorf("support activity %q link sources = %#v, want only its own %#v; facility/other supply sources must not contaminate trace", node.EndUse, link.SourceIDs, node.SourceIDs)
		}
	}
	if !reflect.DeepEqual(gotSupply, wantSupply) {
		t.Fatalf("supply activities = %#v, want %#v", gotSupply, wantSupply)
	}

	consumption := 0.0
	for _, link := range result.Links {
		if link.ToID != carrier.ID || (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") {
			continue
		}
		consumption += link.ToValue
		from := epath111Node(result.Nodes, link.FromID)
		if from == nil || from.Level != "end_use" {
			t.Errorf("consumption link begins at a non-end-use node: %#v", link)
			continue
		}
		if _, supply := wantSupply[from.EndUse]; supply {
			t.Errorf("supply activity %q was counted as consumption: %#v", from.EndUse, link)
		}
	}
	if consumption != 90 {
		t.Fatalf("mapped consumption = %g kWh, want 90; supply activities must not inflate it", consumption)
	}

	reconciliation := epath111Reconciliation(result.Reconciliation, "electricity", "annual")
	if reconciliation == nil {
		t.Fatalf("missing Electricity facility reconciliation: %#v", result.Reconciliation)
	}
	if reconciliation.ExpectedValue != 100 || reconciliation.ExplainedValue != 90 || reconciliation.ResidualValue != 10 {
		t.Fatalf("facility reconciliation = %#v; want 100 = 90 + 10", reconciliation)
	}
	if reconciliation.ExpectedValue != reconciliation.ExplainedValue+reconciliation.ResidualValue {
		t.Errorf("carrier total equation does not close: %#v", reconciliation)
	}
	for _, supportSource := range []string{"produced", "discharge", "purchased", "sold"} {
		if stringSliceContains(reconciliation.SourceIDs, supportSource) {
			t.Errorf("supply source %q leaked into consumption reconciliation provenance: %#v", supportSource, reconciliation.SourceIDs)
		}
	}
	if !stringSliceContains(carrier.Badges, "carrier_residual") {
		t.Errorf("unbalanced carrier has no carrier_residual quality badge: %#v", carrier.Badges)
	}
	residual := epath111Node(result.Nodes, "residual.site_electricity.building")
	if residual == nil || residual.Label != "Unclassified energy" || residual.Value != 10 || residual.Carrier != "electricity" || !stringSliceContains(residual.Badges, "unclassified_energy") {
		t.Errorf("threshold-qualified Unclassified Electricity energy node = %#v", residual)
	}
	if link := epath111Link(result.Links, "residual.site_electricity.building", carrier.ID, "residual"); link == nil || link.FromValue != 10 || link.ToValue != 10 {
		t.Errorf("unclassified residual link = %#v", link)
	}
}

func TestEPATH111ResidualVisibilityUsesStrictRatioOrAbsoluteThreshold(t *testing.T) {
	tests := []struct {
		name             string
		facility         float64
		mapped           float64
		wantResidual     float64
		wantGraphNode    bool
		wantCarrierBadge bool
	}{
		{name: "balanced", facility: 100, mapped: 100, wantResidual: 0, wantGraphNode: false, wantCarrierBadge: false},
		{name: "below both thresholds hidden", facility: 0.4, mapped: 0.393, wantResidual: 0.007, wantGraphNode: false, wantCarrierBadge: true},
		{name: "exact ratio below absolute hidden", facility: 0.4, mapped: 0.392, wantResidual: 0.008, wantGraphNode: false, wantCarrierBadge: true},
		{name: "above ratio only shown", facility: 0.4, mapped: 0.391, wantResidual: 0.009, wantGraphNode: true, wantCarrierBadge: true},
		{name: "exact absolute below ratio hidden", facility: 100, mapped: 99.99, wantResidual: 0.01, wantGraphNode: false, wantCarrierBadge: true},
		{name: "above absolute only shown", facility: 100, mapped: 99.989, wantResidual: 0.011, wantGraphNode: true, wantCarrierBadge: true},
		{name: "exact both thresholds hidden", facility: 0.5, mapped: 0.49, wantResidual: 0.01, wantGraphNode: false, wantCarrierBadge: true},
		{name: "exact ratio but above absolute shown", facility: 100, mapped: 98, wantResidual: 2, wantGraphNode: true, wantCarrierBadge: true},
		{name: "exact absolute but above ratio shown", facility: 0.4, mapped: 0.39, wantResidual: 0.01, wantGraphNode: true, wantCarrierBadge: true},
		{name: "above both thresholds shown", facility: 100, mapped: 97.999, wantResidual: 2.001, wantGraphNode: true, wantCarrierBadge: true},
		{name: "exact negative two percent hidden", facility: 100, mapped: 102, wantResidual: -2, wantGraphNode: false, wantCarrierBadge: true},
		{name: "negative above two percent remains inspector only", facility: 100, mapped: 102.001, wantResidual: -2.001, wantGraphNode: false, wantCarrierBadge: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := epath111LegacyCarrierFixture(test.facility, test.mapped, nil)
			// Threshold tests isolate accounting from supply-strip behavior.
			fixture.Nodes = fixture.Nodes[:2]
			fixture.Edges = fixture.Edges[:1]
			fixture.Sources = fixture.Sources[:2]
			result := UpgradeEnergyExplanationV1(fixture)

			row := epath111Reconciliation(result.Reconciliation, "electricity", "annual")
			if row == nil || !epath111Close(row.ResidualValue, test.wantResidual) || !epath111Close(row.ExpectedValue, row.ExplainedValue+row.ResidualValue) {
				t.Fatalf("reconciliation = %#v; want residual %g and exact signed closure", row, test.wantResidual)
			}
			carrier := epath111Node(result.Nodes, "carrier.electricity.building")
			if carrier == nil {
				t.Fatal("missing Electricity carrier")
			}
			if got := stringSliceContains(carrier.Badges, "carrier_residual"); got != test.wantCarrierBadge {
				t.Errorf("carrier_residual badge = %t, want %t; badges=%#v", got, test.wantCarrierBadge, carrier.Badges)
			}
			residual := epath111Node(result.Nodes, "residual.site_electricity.building")
			if got := residual != nil; got != test.wantGraphNode {
				t.Fatalf("Unclassified energy graph node present = %t, want %t; residual=%#v", got, test.wantGraphNode, residual)
			}
			if residual != nil {
				if !epath111Close(residual.Value, math.Abs(test.wantResidual)) || residual.Label != "Unclassified energy" || residual.Carrier != "electricity" || !stringSliceContains(residual.Badges, "unclassified_energy") {
					t.Errorf("visible residual node = %#v", residual)
				}
				if link := epath111Link(result.Links, residual.ID, carrier.ID, "residual"); link == nil || !epath111Close(link.FromValue, math.Abs(test.wantResidual)) || !epath111Close(link.ToValue, math.Abs(test.wantResidual)) {
					t.Errorf("visible residual link = %#v", link)
				}
			} else {
				for _, link := range result.Links {
					if link.Relation == "residual" {
						t.Errorf("hidden residual retained a graph link: %#v", link)
					}
				}
			}
		})
	}
}

func TestEPATH111AnnualResidualUsesSignedMonthlyCancellation(t *testing.T) {
	monthly := []EnergyPeriod{
		epath111LegacyPeriod("M1", 100, 90),
		epath111LegacyPeriod("M2", 100, 110),
	}
	// Conflicting annual raw totals ensure this exercises monthly-first annual
	// aggregation rather than accidentally passing through the annual fixture.
	fixture := epath111LegacyCarrierFixture(999, 999, monthly)
	fixture.canonicalMonthlyBasis = true
	fixture.Nodes = fixture.Nodes[:2]
	fixture.Edges = fixture.Edges[:1]
	fixture.Sources = fixture.Sources[:2]
	result := UpgradeEnergyExplanationV1(fixture)

	annual := epath111Reconciliation(result.Reconciliation, "electricity", "annual")
	if annual == nil || annual.ExpectedValue != 200 || annual.ExplainedValue != 200 || annual.ResidualValue != 0 {
		t.Fatalf("annual signed cancellation = %#v; want 200 - 200 = 0", annual)
	}
	annualCarrier := epath111Node(result.Nodes, "carrier.electricity.building")
	if annualCarrier == nil || stringSliceContains(annualCarrier.Badges, "carrier_residual") {
		t.Errorf("balanced annual carrier retained residual badge: %#v", annualCarrier)
	}
	if residual := epath111Node(result.Nodes, "residual.site_electricity.building"); residual != nil {
		t.Errorf("opposite monthly residuals were summed by magnitude into annual Unclassified energy: %#v", residual)
	}

	wantMonthly := map[string]float64{"M1": 10, "M2": -10}
	sum := 0.0
	for periodID, want := range wantMonthly {
		period := epath111Period(result.Periods, periodID)
		if period == nil {
			t.Fatalf("missing %s period: %#v", periodID, result.Periods)
		}
		row := epath111Reconciliation(period.Reconciliation, "electricity", periodID)
		if row == nil || row.ResidualValue != want || row.ExpectedValue != row.ExplainedValue+row.ResidualValue {
			t.Errorf("%s reconciliation = %#v; want signed residual %g", periodID, row, want)
			continue
		}
		sum += row.ResidualValue
		carrier := epath111Node(period.Nodes, "carrier.electricity.building")
		residual := epath111Node(period.Nodes, "residual.site_electricity.building")
		wantResidualNode := want > 0
		if carrier == nil || !stringSliceContains(carrier.Badges, "carrier_residual") || (residual != nil) != wantResidualNode || residual != nil && residual.Value != 10 {
			t.Errorf("%s threshold presentation carrier/residual = %#v / %#v", periodID, carrier, residual)
		}
	}
	if sum != annual.ResidualValue {
		t.Errorf("monthly signed residual sum = %g, annual residual = %g", sum, annual.ResidualValue)
	}
}

func TestEPATH111StoredV2SanitizesFacilityReconciliationAndResidualPresentation(t *testing.T) {
	tests := []struct {
		name          string
		mapped        float64
		staleResidual bool
		wantResidual  float64
		wantNode      bool
	}{
		{name: "removes stale node below both thresholds", mapped: 99.995, staleResidual: true, wantResidual: 0.005, wantNode: false},
		{name: "creates missing node above threshold", mapped: 97, staleResidual: false, wantResidual: 3, wantNode: true},
		{name: "overmapping remains inspector only", mapped: 110, staleResidual: true, wantResidual: -10, wantNode: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stored := epath111StoredV2Payload(test.mapped, test.staleResidual)
			stored.Completeness.MappedPercent = 100
			encoded, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			var first EnergyExplanationResult
			if err := json.Unmarshal(encoded, &first); err != nil {
				t.Fatal(err)
			}

			carrier := epath111Node(first.Nodes, "carrier.electricity.building")
			if carrier == nil || carrier.Value != 100 || !stringSliceContains(carrier.Badges, "carrier_residual") {
				t.Errorf("sanitized stored carrier = %#v", carrier)
			}
			if got := len(epath111NodesAtLevel(first.Nodes, "carrier")); got != 1 {
				t.Errorf("stored payload produced %d carrier nodes, want one", got)
			}
			row := epath111Reconciliation(first.Reconciliation, "electricity", "annual")
			if row == nil || row.ExpectedValue != 100 || row.ExplainedValue != test.mapped || !epath111Close(row.ResidualValue, test.wantResidual) {
				t.Errorf("stale stored reconciliation was not rebuilt: %#v", row)
			}
			if want := math.Min(100, test.mapped); !epath111Close(first.Completeness.MappedPercent, want) {
				t.Errorf("stored-v2 mapped coverage = %g, want %g from rebuilt consumption accounting", first.Completeness.MappedPercent, want)
			}
			residual := epath111Node(first.Nodes, "residual.site_electricity.building")
			if got := residual != nil; got != test.wantNode {
				t.Errorf("sanitized residual node present = %t, want %t; node=%#v", got, test.wantNode, residual)
			}
			if residual != nil && (residual.Value != math.Abs(test.wantResidual) || residual.Label != "Unclassified energy" || residual.Carrier != "electricity" || !stringSliceContains(residual.Badges, "unclassified_energy")) {
				t.Errorf("sanitized Unclassified node = %#v", residual)
			}
			for _, token := range []string{"generators", "storage_discharge", "electricity_purchased", "electricity_sold"} {
				support := epath111SupportByEndUse(first.Nodes, token)
				if support == nil || support.Value <= 0 || epath111Link(first.Links, support.ID, carrier.ID, "support_supply") == nil {
					t.Errorf("stored supply activity %q was lost or converted to consumption: %#v", token, support)
					continue
				}
				if link := epath111Link(first.Links, support.ID, carrier.ID, "support_supply"); !reflect.DeepEqual(link.SourceIDs, support.SourceIDs) {
					t.Errorf("stored support activity %q source trace was contaminated: link=%#v support=%#v", token, link.SourceIDs, support.SourceIDs)
				}
			}

			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			var second EnergyExplanationResult
			if err := json.Unmarshal(firstJSON, &second); err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if string(firstJSON) != string(secondJSON) {
				t.Errorf("stored-v2 sanitation is not round-trip idempotent\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
			}
		})
	}
}

func TestEPATH111StoredV2DropsOrphanCarrierAccountingAndKeepsAuxiliaryAllocation(t *testing.T) {
	stored := epath111StoredV2Payload(97, false)
	stored.Nodes = nil
	stored.Links = nil
	stored.Completeness.MappedPercent = 100
	auxiliary := EnergyReconciliation{
		ID: "reconcile.zone_auxiliary_allocation.fans.electricity.annual", Level: "allocation", Period: "annual",
		Label: "Fans allocation", ExpectedValue: 12, ExplainedValue: 9, ResidualValue: 3, AllocatedValue: 9, UnassignedValue: 3,
		Unit: "kWh", Basis: "service_path_allocation", SourceIDs: []string{"fan"},
	}
	stored.Reconciliation = append(stored.Reconciliation, auxiliary)
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var result EnergyExplanationResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Reconciliation) != 1 || !reflect.DeepEqual(result.Reconciliation[0], auxiliary) {
		t.Errorf("orphan carrier row must disappear while auxiliary allocation accounting survives unchanged: %#v", result.Reconciliation)
	}
	if result.Completeness.MappedPercent != 0 {
		t.Errorf("no carrier graph retained stale mapped coverage %g", result.Completeness.MappedPercent)
	}
}

func TestEPATH111BasicEnergyRequestsSupplyMetersWithOnsiteGeneration(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Generator:Photovoltaic,
  Roof PV;
ElectricLoadCenter:Storage:Simple,
  Battery;
`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	for _, meter := range []string{"Electricity:Facility", "ElectricityPurchased:Facility", "ElectricityProduced:Facility", "ElectricitySurplusSold:Facility"} {
		output := findPurposeOutput(plan, "Output:Meter", meter, "")
		if output == nil || output.ReportingFrequency != "Monthly" {
			t.Errorf("Basic Energy must request monthly %s to populate facility and Supply breakdown, got %#v", meter, output)
		}
	}
	for _, variable := range []string{"Electric Storage Charge Energy", "Electric Storage Discharge Energy"} {
		output := findPurposeOutput(plan, "Output:Variable", "*", variable)
		if output == nil || output.ReportingFrequency != "Monthly" {
			t.Errorf("Basic Energy must request monthly %s, got %#v", variable, output)
		}
	}
}

func TestEPATH111SQLSupplyMetersAndStorageConsumptionRemainSeparate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Existing fixture: facility consumption 3 kWh, cooling 1 kWh. Storage
	// charge adds 1 kWh of consumption; purchased/produced/sold/discharge
	// activity must be traceable while leaving that accounting unchanged.
	for _, statement := range []string{
		`INSERT INTO ReportDataDictionary VALUES
		 (30, '', 'ElectricityPurchased:Facility', 'J'),
		 (31, '', 'ElectricityProduced:Facility', 'J'),
		 (32, '', 'ElectricitySurplusSold:Facility', 'J'),
		 (33, 'Battery', 'Electric Storage Charge Energy', 'J'),
		 (34, 'Battery', 'Electric Storage Discharge Energy', 'J')`,
		`INSERT INTO ReportData VALUES
		 (30, 1, 30, 7200000), (31, 2, 30, 7200000),
		 (32, 1, 31, 3600000), (33, 2, 31, 3600000),
		 (34, 1, 32, 1800000), (35, 2, 32, 1800000),
		 (36, 1, 33, 1800000), (37, 2, 33, 1800000),
		 (38, 1, 34, 900000), (39, 2, 34, 900000)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := parseSimulationEnergyExplanationSQL(path, &PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	carriers := epath111NodesAtLevel(result.Nodes, "carrier")
	if len(carriers) != 1 || carriers[0].Carrier != "electricity" || carriers[0].Value != 3 {
		t.Fatalf("SQL facility carrier must remain 3 kWh consumption, got %#v", carriers)
	}
	row := epath111Reconciliation(result.Reconciliation, "electricity", "annual")
	if row == nil || row.ExpectedValue != 3 || row.ExplainedValue != 2 || row.ResidualValue != 1 {
		t.Fatalf("SQL consumption must count cooling + storage charge only: %#v", row)
	}
	wantSupply := []struct {
		token, source string
		value         float64
	}{
		{"electricity_purchased", "sql-rdd-30", 4},
		{"generators", "sql-rdd-31", 2},
		{"electricity_sold", "sql-rdd-32", 1},
		{"storage_discharge", "sql-rdd-34", 0.5},
	}
	for _, want := range wantSupply {
		node := epath111SupportByEndUse(result.Nodes, want.token)
		if node == nil || node.Value != want.value || !reflect.DeepEqual(node.SourceIDs, []string{want.source}) {
			t.Errorf("SQL support %s = %#v, want %g kWh from %s", want.token, node, want.value, want.source)
			continue
		}
		link := epath111Link(result.Links, node.ID, carriers[0].ID, "support_supply")
		if link == nil || link.ToValue != want.value || !reflect.DeepEqual(link.SourceIDs, []string{want.source}) {
			t.Errorf("SQL support %s link must isolate its own source: %#v", want.token, link)
		}
		if stringSliceContains(row.SourceIDs, want.source) {
			t.Errorf("SQL support source %s contaminated carrier consumption reconciliation", want.source)
		}
	}
	chargeFound := false
	for _, link := range result.Links {
		if !stringSliceContains(link.SourceIDs, "sql-rdd-33") || link.ToID != carriers[0].ID {
			continue
		}
		if link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier" {
			chargeFound = link.ToValue == 1
		}
	}
	if !chargeFound || !stringSliceContains(row.SourceIDs, "sql-rdd-33") {
		t.Errorf("storage charge is missing from consumption links/reconciliation: links=%#v row=%#v", result.Links, row)
	}
	assertChargeContext := func(label string, graph EnergyExplanationResult) {
		t.Helper()
		charge := epath111SupportByEndUse(graph.Nodes, "storage_charge")
		if charge == nil || charge.ID != "support.storage_charge.building" || charge.Value != 1 || !reflect.DeepEqual(charge.SourceIDs, []string{"sql-rdd-33"}) {
			t.Errorf("%s must retain exact charge context alongside grouped Other consumption: %#v", label, charge)
		}
		for _, link := range graph.Links {
			if charge != nil && (link.FromID == charge.ID || link.ToID == charge.ID) {
				t.Errorf("%s storage charge context created an extra energy flow: %#v", label, link)
			}
		}
		other := epath111Node(graph.Nodes, "end_use.other.building")
		if other == nil || other.Value != 1 || !stringSliceContains(other.SourceIDs, "sql-rdd-33") {
			t.Errorf("%s charge context displaced the canonical Other consumption: %#v", label, other)
		}
		if january := epath111Period(graph.Periods, "M1"); january == nil {
			t.Errorf("%s lost January graph", label)
		} else if charge := epath111SupportByEndUse(january.Nodes, "storage_charge"); charge == nil || charge.Value != 0.5 {
			t.Errorf("%s January charge context lost period-specific value: %#v", label, charge)
		}
	}
	assertChargeContext("SQL result", result)
	stored := result
	for _, label := range []string{"first roundtrip", "second roundtrip"} {
		encoded, err := json.Marshal(stored)
		if err != nil {
			t.Fatal(err)
		}
		var decoded EnergyExplanationResult
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		assertChargeContext(label, decoded)
		stored = decoded
	}
	summary := buildEnergyExplanationSummaryV2(result)
	if len(summary.Carriers) != 1 || summary.Carriers[0].Value != 3 {
		t.Errorf("Supply context inflated summary carrier energy: %#v", summary.Carriers)
	}
	for _, item := range summary.EndUses {
		for _, want := range wantSupply {
			if stringSliceContains(item.SourceIDs, want.source) {
				t.Errorf("Supply activity leaked into end-use summary: %#v", item)
			}
		}
	}
}

func epath111LegacyCarrierFixture(facility float64, mapped float64, periods []EnergyPeriod) EnergyExplanationV1 {
	return EnergyExplanationV1{
		Schema:  energyExplanationV1Schema,
		Purpose: string(SimulationPurposeBasicEnergy),
		Periods: periods,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity total", Value: facility, Unit: "kWh", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"facility"}},
			{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "Cooling energy", Value: mapped, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"cooling"}},
			{ID: "energy.end_use.generators.electricity", Level: "support", Kind: "energy.generators", Label: "Onsite production", Value: 25, Unit: "kWh", Carrier: "electricity", EndUse: "generators", Basis: "measured_meter", SourceIDs: []string{"produced"}},
			{ID: "energy.end_use.storage_discharge.electricity", Level: "support", Kind: "energy.storage_discharge", Label: "Storage discharge", Value: 8, Unit: "kWh", Carrier: "electricity", EndUse: "storage_discharge", Basis: "measured_energy_variable", SourceIDs: []string{"discharge"}},
			{ID: "energy.support.electricity_purchased", Level: "support", Kind: "energy.electricity_purchased", Label: "Purchased electricity", Value: 120, Unit: "kWh", Carrier: "electricity", EndUse: "electricity_purchased", Basis: "measured_meter", SourceIDs: []string{"purchased"}},
			{ID: "energy.support.electricity_sold", Level: "support", Kind: "energy.electricity_sold", Label: "Sold electricity", Value: 15, Unit: "kWh", Carrier: "electricity", EndUse: "electricity_sold", Basis: "measured_meter", SourceIDs: []string{"sold"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "cooling", FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: mapped, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"cooling"}},
			{ID: "produced", FromID: "energy.carrier.electricity", ToID: "energy.end_use.generators.electricity", Value: 25, Unit: "kWh", Relation: "onsite_production", Basis: "measured_meter", SourceIDs: []string{"produced"}},
			{ID: "discharge", FromID: "energy.carrier.electricity", ToID: "energy.end_use.storage_discharge.electricity", Value: 8, Unit: "kWh", Relation: "storage_discharge", Basis: "measured_energy_variable", SourceIDs: []string{"discharge"}},
			{ID: "purchased", FromID: "energy.carrier.electricity", ToID: "energy.support.electricity_purchased", Value: 120, Unit: "kWh", Relation: "support_supply", Basis: "measured_meter", SourceIDs: []string{"purchased"}},
			{ID: "sold", FromID: "energy.carrier.electricity", ToID: "energy.support.electricity_sold", Value: 15, Unit: "kWh", Relation: "support_supply", Basis: "measured_meter", SourceIDs: []string{"sold"}},
		},
		Sources: []EnergyDataSource{
			{ID: "facility", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "cooling", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "produced", SourceType: "sql_meter", IsMeter: true, Name: "ElectricityProduced:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "discharge", SourceType: "sql_variable", Name: "Electric Storage Discharge Energy", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "purchased", SourceType: "sql_meter", IsMeter: true, Name: "ElectricityPurchased:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "sold", SourceType: "sql_meter", IsMeter: true, Name: "ElectricitySurplusSold:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
		},
	}
}

func epath111LegacyPeriod(id string, facility float64, mapped float64) EnergyPeriod {
	return EnergyPeriod{
		ID: id, Label: id, Kind: "monthly",
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity total", Value: facility, Unit: "kWh", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "measured_meter", SourceIDs: []string{"facility"}},
			{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "Cooling energy", Value: mapped, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "measured_meter", SourceIDs: []string{"cooling"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "cooling." + id, FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: mapped, Unit: "kWh", Period: id, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"cooling"}},
		},
	}
}

func epath111StoredV2Payload(mapped float64, staleResidual bool) EnergyExplanationResult {
	nodes := []EnergyExplanationNode{
		{ID: "carrier.electricity.building", Level: "carrier", Kind: "carrier.electricity", Label: "old Electricity", Value: 100, Unit: "kWh", ScaleDomain: "site", Carrier: "electricity", Badges: []string{"stale_badge"}, SourceIDs: []string{"facility"}},
		{ID: "end_use.cooling.building", Level: "end_use", Kind: "energy.cooling", Label: "Cooling", Value: mapped, Unit: "kWh", ScaleDomain: "site", EndUse: "cooling", SourceIDs: []string{"cooling"}},
		{ID: "support.generators.building", Level: "support", Kind: "energy.generators", Label: "Produced", Value: 25, DisplayValue: 25, Unit: "kWh", ScaleDomain: "site", Carrier: "electricity", EndUse: "generators", SourceIDs: []string{"produced"}},
		{ID: "support.storage_discharge.building", Level: "support", Kind: "energy.storage_discharge", Label: "Discharge", Value: 8, DisplayValue: 8, Unit: "kWh", ScaleDomain: "site", Carrier: "electricity", EndUse: "storage_discharge", SourceIDs: []string{"discharge"}},
		{ID: "support.electricity_purchased.building", Level: "support", Kind: "energy.electricity_purchased", Label: "Purchased", Value: 120, DisplayValue: 120, Unit: "kWh", ScaleDomain: "site", Carrier: "electricity", EndUse: "electricity_purchased", SourceIDs: []string{"purchased"}},
		{ID: "support.electricity_sold.building", Level: "support", Kind: "energy.electricity_sold", Label: "Sold", Value: 15, DisplayValue: 15, Unit: "kWh", ScaleDomain: "site", Carrier: "electricity", EndUse: "electricity_sold", SourceIDs: []string{"sold"}},
	}
	links := []EnergyPathLink{
		{ID: "cooling", FromID: "end_use.cooling.building", ToID: "carrier.electricity.building", Relation: "end_use_to_carrier", Basis: "measured_meter", FromValue: mapped, FromUnit: "kWh", ToValue: mapped, ToUnit: "kWh", SourceIDs: []string{"cooling"}},
		{ID: "produced", FromID: "support.generators.building", ToID: "carrier.electricity.building", Relation: "support_supply", Basis: "measured_meter", FromValue: 25, FromUnit: "kWh", ToValue: 25, ToUnit: "kWh", SourceIDs: []string{"produced"}},
		{ID: "discharge", FromID: "support.storage_discharge.building", ToID: "carrier.electricity.building", Relation: "support_supply", Basis: "measured_energy_variable", FromValue: 8, FromUnit: "kWh", ToValue: 8, ToUnit: "kWh", SourceIDs: []string{"discharge"}},
		{ID: "purchased", FromID: "support.electricity_purchased.building", ToID: "carrier.electricity.building", Relation: "support_supply", Basis: "measured_meter", FromValue: 120, FromUnit: "kWh", ToValue: 120, ToUnit: "kWh", SourceIDs: []string{"purchased"}},
		{ID: "sold", FromID: "support.electricity_sold.building", ToID: "carrier.electricity.building", Relation: "support_supply", Basis: "measured_meter", FromValue: 15, FromUnit: "kWh", ToValue: 15, ToUnit: "kWh", SourceIDs: []string{"sold"}},
	}
	if staleResidual {
		nodes = append(nodes, EnergyExplanationNode{ID: "residual.site_electricity.building", Level: "residual", Kind: "energy.residual", Label: "stale always-visible other", Value: 2, Unit: "kWh", ScaleDomain: "site", Carrier: "electricity", Badges: []string{"stale"}, Basis: "residual"})
		links = append(links, EnergyPathLink{ID: "stale-residual", FromID: "residual.site_electricity.building", ToID: "carrier.electricity.building", Relation: "residual", Basis: "residual", FromValue: 2, FromUnit: "kWh", ToValue: 2, ToUnit: "kWh"})
	}
	return EnergyExplanationResult{
		Schema: energyExplanationSchema,
		Scope:  EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Nodes:  nodes,
		Links:  links,
		Reconciliation: []EnergyReconciliation{{
			ID: "reconcile.energy.electricity.annual", Level: "energy", Period: "annual", Label: "stale",
			Status: "balanced", ExpectedValue: 999, ExplainedValue: 999, ResidualValue: 0, Unit: "kWh", Basis: "residual",
		}},
		Sources: []EnergyDataSource{
			{ID: "facility", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "cooling", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "produced", SourceType: "sql_meter", IsMeter: true, Name: "ElectricityProduced:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "discharge", SourceType: "sql_variable", Name: "Electric Storage Discharge Energy", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "purchased", SourceType: "sql_meter", IsMeter: true, Name: "ElectricityPurchased:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "sold", SourceType: "sql_meter", IsMeter: true, Name: "ElectricitySurplusSold:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
		},
	}
}

func epath111NodesAtLevel(nodes []EnergyExplanationNode, level string) []EnergyExplanationNode {
	out := make([]EnergyExplanationNode, 0)
	for _, node := range nodes {
		if node.Level == level {
			out = append(out, node)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func epath111Node(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath111SupportByEndUse(nodes []EnergyExplanationNode, endUse string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].Level == "support" && nodes[index].EndUse == endUse {
			return &nodes[index]
		}
	}
	return nil
}

func epath111Link(links []EnergyPathLink, fromID string, toID string, relation string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID && links[index].Relation == relation {
			return &links[index]
		}
	}
	return nil
}

func epath111Reconciliation(rows []EnergyReconciliation, carrier string, period string) *EnergyReconciliation {
	wantID := "reconcile.energy." + carrier + "." + period
	for index := range rows {
		if rows[index].ID == wantID {
			return &rows[index]
		}
	}
	return nil
}

func epath111Period(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func epath111Close(left float64, right float64) bool {
	return math.Abs(left-right) <= 1e-9
}
