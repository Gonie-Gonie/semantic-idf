package simulation

import (
	"encoding/json"
	"testing"
)

func epathSQLCentralCOPUnit(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	path, original, executed, plan, model, observed, frames := epathSQLCentralSharedUnit(t, true, false)
	if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &frames, plan, original, executed); err != nil {
		t.Fatal(err)
	}
	frames.SourceEffective = map[int][]epathSQLQuantity{}
	// Independent hand observations for the four outside-pool native District
	// identities. Their twelve observed zeroes are not missing consumption.
	for n, item := range []struct {
		id, name, group, carrier, endUse string
		facility                         bool
	}{
		{"cooling.district_cooling", "Cooling:DistrictCooling", "Facility:DistrictCooling:Cooling", "district_cooling", "cooling", false},
		{"heating.district_heating", "Heating:DistrictHeatingWater", "Facility:DistrictHeatingWater:Heating", "district_heating", "heating", false},
		{"facility.district_cooling", "DistrictCooling:Facility", "Facility:DistrictCooling", "district_cooling", "", true},
		{"facility.district_heating", "DistrictHeatingWater:Facility", "Facility:DistrictHeatingWater", "district_heating", "", true},
	} {
		id := 901 + n
		source := epathSQLHVACConsumptionUnitSource(id, item.name, "", true, [12]float64{})
		source.IndexGroup = item.group
		model.Site = append(model.Site, epathRealSQLSite{ID: item.id, EndUse: item.endUse, Carrier: item.carrier, Facility: item.facility, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{""}, IsMeter: true}})
		frames.SourceIdentities[id] = source
		frames.SiteSources[item.id] = []int{id}
		frames.SourceRaw[id], frames.SourceEffective[id] = make([]epathSQLQuantity, 12), make([]epathSQLQuantity, 12)
		for month := 0; month < 12; month++ {
			frames.Site[item.id] = append(frames.Site[item.id], &epathSQLQuantity{})
		}
	}
	return frames, model
}

func TestEnergyPathSQLCentralCoolingCOPKeepsNativePairedArithmetic(t *testing.T) {
	frames, model := epathSQLCentralCOPUnit(t)
	checks := epathSQLModelChecks{}
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, check := range checks.Rows {
		if check.Item.Group != "ratios" || check.Item.Period != "M1" || check.Item.Target.Basis != "service_path_allocation" || check.Conversion == nil || check.Quantity == nil {
			continue
		}
		service := check.Item.Target.Service
		wantKind, want := "coefficient_of_performance", 1.0
		if service == "heating" {
			wantKind, want = "load_to_site_energy", .5
		}
		if check.Item.Target.RatioKind != wantKind || check.Quantity.Value != want || check.Conversion.From.Value/check.Conversion.To.Value != want {
			t.Fatalf("native C15/H30 distinct denominators or ratio label changed: %+v %+v", check.Item, check.Conversion)
		}
		counts[service]++
	}
	if counts["cooling"] != 6 || counts["heating"] != 6 {
		t.Fatalf("want Building and five positive recipient ratios, with no PLENUM paid pair: %v", counts)
	}
	heat, err := epathSQLCompileHVACConsumptionService(frames, model, model.Services[1])
	if err != nil || heat == nil || heat.centralCoolingCOP {
		t.Fatalf("Heating acquired Cooling authority: %v", err)
	}
	heat.centralCoolingCOP = true
	if err := epathSQLValidateHVACConsumptionService(heat); err == nil {
		t.Fatal("forged detached Cooling flag bypassed original arithmetic rebuild")
	}
	boilerFrames, boilerModel := epathSQLHVACConsumptionUnitFrames(t, [12]float64{1}, [12]float64{1}, [12]float64{1})
	boiler, err := epathSQLCompileHVACConsumptionService(boilerFrames, boilerModel, boilerModel.Services[0])
	if err != nil || boiler == nil || boiler.centralCoolingCOP || epathSQLHVACConsumptionRatioDeclaration(boiler, "", "M1", "coefficient_of_performance") != "load_to_site_energy" {
		t.Fatalf("existing Boiler/mixed-carrier behavior changed: %v", err)
	}
}

func TestEnergyPathSQLCentralCoolingCOPRejectsUnprovedDistrictOrRoles(t *testing.T) {
	frames, model := epathSQLCentralCOPUnit(t)
	type input struct {
		Frames epathSQLFrames
		Model  epathRealSQLModel
	}
	data, err := json.Marshal(input{frames, model})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing-month", "uncertain-zero", "tiny-positive", "positive-district", "missing-native", "null-native", "wrong-native-group", "foreign-selector", "mixed-carrier", "wrong-role", "wrong-recipient", "missing-sibling-pool"} {
		t.Run(name, func(t *testing.T) {
			var bad input
			if err := json.Unmarshal(data, &bad); err != nil {
				t.Fatal(err)
			}
			f, m := &bad.Frames, &bad.Model
			source := f.SourceIdentities[901]
			switch name {
			case "missing-month":
				f.Site["cooling.district_cooling"][0] = nil
			case "uncertain-zero":
				f.Site["cooling.district_cooling"][0] = &epathSQLQuantity{Error: 1e-12}
			case "tiny-positive":
				f.Site["cooling.district_cooling"][0] = &epathSQLQuantity{Value: 1e-12}
			case "positive-district":
				f.Site["cooling.district_cooling"][0] = &epathSQLQuantity{Value: 1}
			case "missing-native":
				delete(f.SourceIdentities, 901)
			case "null-native":
				source.Months[0].RawSum = nil
				f.SourceIdentities[901] = source
			case "wrong-native-group":
				source.IndexGroup = "Facility:Electricity:Cooling"
				f.SourceIdentities[901] = source
			case "foreign-selector":
				m.Site[2].Source.Alternatives[0].Name = "Cooling:Electricity"
			case "mixed-carrier":
				m.Site[2].Carrier = "natural_gas"
			case "wrong-role":
				m.HVACConsumptionPools[0].Shared[0].PlantLoopName = "Hot Water Loop"
			case "wrong-recipient":
				m.Services[0].ServedZones[0] = "PLENUM-1"
			case "missing-sibling-pool":
				m.HVACConsumptionPools = m.HVACConsumptionPools[:1]
				f.HVACConsumptionPools = f.HVACConsumptionPools[:1]
			}
			pool, err := epathSQLCompileHVACConsumptionService(*f, *m, m.Services[0])
			if err != nil {
				return // Existing source/ownership contracts may reject first.
			}
			if pool == nil || pool.centralCoolingCOP || epathSQLHVACConsumptionRatioDeclaration(pool, "", "M1", "coefficient_of_performance") == "coefficient_of_performance" {
				t.Fatal("unproved pure-electric Central boundary acquired COP authority")
			}
		})
	}
}
