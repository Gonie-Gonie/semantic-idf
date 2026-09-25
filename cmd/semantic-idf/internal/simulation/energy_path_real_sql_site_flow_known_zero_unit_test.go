package simulation

import "testing"

func TestEnergyPathSQLSiteFlowUnassignedThermalRequiresExactNativeZero(t *testing.T) {
	for _, service := range []string{"cooling", "heating"} {
		for _, tc := range []struct {
			name string
			q    *epathSQLQuantity
			pass bool
		}{
			{"known-zero", &epathSQLQuantity{}, true},
			{"bounded-known-zero", &epathSQLQuantity{Bounds: &[2]float64{0, 0}}, true},
			{"positive", &epathSQLQuantity{Value: 1}, false},
			{"tiny-positive", &epathSQLQuantity{Value: 1e-12}, false},
			{"missing", nil, false},
			{"negative-uncertainty", &epathSQLQuantity{Error: 1e-12, Bounds: &[2]float64{-1e-12, 0}}, false},
			{"positive-uncertainty", &epathSQLQuantity{Error: 1e-12, Bounds: &[2]float64{0, 1e-12}}, false},
		} {
			t.Run(service+"/"+tc.name, func(t *testing.T) {
				site := epathRealSQLSite{ID: "unassigned", EndUse: service, Carrier: "district_" + service}
				frames := epathSQLFrames{Site: map[string][]*epathSQLQuantity{site.ID: make([]*epathSQLQuantity, 12)}}
				frames.Site[site.ID][0] = tc.q
				model := epathRealSQLModel{}
				split, err := epathSQLSiteFlowMonth(frames, model, site, 1)
				if (err == nil) != tc.pass {
					t.Fatalf("unassigned thermal flow acceptance = %v: %v", err == nil, err)
				}
				if !tc.pass {
					return
				}
				if len(split) != 2 || len(model.Services) != 0 || len(frames.SourceZone) != 0 {
					t.Fatal("zero flow created service/Zone ownership")
				}
				for _, q := range split {
					lo, hi := q.bounds()
					if q.Value != 0 || lo != 0 || hi != 0 {
						t.Fatal("known zero acquired a paid flow or uncertainty")
					}
				}
			})
		}
	}
}

func TestEnergyPathSQLSiteFlowKnownZeroRetainsNativeSourceAndCalendar(t *testing.T) {
	for _, service := range []string{"cooling", "heating"} {
		fixture := func() (epathSQLFrames, epathRealSQLModel) {
			name := "Cooling:DistrictCooling"
			if service == "heating" {
				name = "Heating:DistrictHeatingWater"
			}
			site := epathRealSQLSite{ID: service + ".district", EndUse: service, Carrier: "district_" + service}
			source := epathSQLHVACConsumptionUnitSource(77, name, "", true, [12]float64{})
			frames := epathSQLFrames{Site: map[string][]*epathSQLQuantity{site.ID: {}}, SiteSources: map[string][]int{site.ID: {77}}, SourceIdentities: map[int]epathRealSQLSource{77: source}}
			for month := 1; month <= 12; month++ {
				frames.Site[site.ID] = append(frames.Site[site.ID], &epathSQLQuantity{})
			}
			return frames, epathRealSQLModel{Site: []epathRealSQLSite{site}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
		}
		frames, model := fixture()
		var checks epathSQLModelChecks
		if err := epathSQLModelSiteFlowChecks(frames, model, &checks); err != nil {
			t.Fatal(err)
		}
		if len(checks.Rows) != 13*4 || len(model.Services) != 0 || len(model.HVACConsumptionPools) != 0 {
			t.Fatal("known zero lost Monthly/annual checks or created paid ownership")
		}
		for _, check := range checks.Rows {
			if check.Quantity == nil || check.Quantity.Value != 0 || check.Quantity.Error != 0 || check.SiteFlow == nil || len(check.SiteFlow.Sources) != 1 || len(check.SiteFlow.NodeSources) != 1 || check.SiteFlow.Sources["sql-rdd-77"].RDD == nil {
				t.Fatal("known zero dropped its original site/Building identity")
			}
		}
		for _, mutation := range []string{"missing-identity", "missing-leaves", "missing-native-month", "unknown-native-month", "wrong-calendar-month", "nonmeter"} {
			t.Run(service+"/"+mutation, func(t *testing.T) {
				frames, model := fixture()
				source := frames.SourceIdentities[77]
				switch mutation {
				case "missing-identity":
					delete(frames.SourceIdentities, 77)
				case "missing-leaves":
					delete(frames.SiteSources, model.Site[0].ID)
				case "missing-native-month":
					source.Months = source.Months[:11]
				case "unknown-native-month":
					source.Months[1].EnergyKWh = nil
				case "wrong-calendar-month":
					source.Months[1].Month = 1
				case "nonmeter":
					source.IsMeter = false
				}
				if mutation != "missing-identity" {
					frames.SourceIdentities[77] = source
				}
				if err := epathSQLModelSiteFlowChecks(frames, model, &epathSQLModelChecks{}); err == nil {
					t.Fatal("known-zero arithmetic hid invalid native provenance")
				}
			})
		}
	}
}
