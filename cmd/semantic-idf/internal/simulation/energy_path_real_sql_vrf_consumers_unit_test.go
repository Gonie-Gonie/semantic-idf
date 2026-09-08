package simulation

import (
	"reflect"
	"testing"
)

func epathSQLVRFConsumerUnitProof(t *testing.T) *epathSQLZoneServiceProof {
	t.Helper()
	frames, model := epathSQLVRFServiceUnitFrames(t)
	service, err := epathSQLCompileVRFService(frames, model, model.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	return service.Zones["space1-1"]["M1"]
}

func TestEnergyPathRealSQLVRFConsumersPreserveExactConstituents(t *testing.T) {
	service := epathSQLVRFConsumerUnitProof(t)
	quality := &epathSQLQualityProof{SiteSources: map[string]map[string][]int{"cooling": {"electricity": {999}}, "heating": {"electricity": {998}}}}
	if err := epathSQLVRFQualitySources(quality, service); err != nil {
		t.Fatal(err)
	}
	if len(quality.SiteSources["cooling"]) != 0 || len(quality.SiteOriginals["cooling"]["electricity"]) != 3 || !reflect.DeepEqual(quality.SiteSources["heating"]["electricity"], []int{998}) {
		t.Fatal("VRF consumed the broad meter again or changed an unrelated service")
	}
	for key := range quality.SiteOriginals["cooling"]["electricity"] {
		delete(quality.SiteOriginals["cooling"]["electricity"], key)
		break
	}
	if len(service.NativeVRF.Originals["electricity"]) != 3 {
		t.Fatal("quality mutation changed the independently compiled service source map")
	}
}

func TestEnergyPathRealSQLVRFConsumersRejectForeignOrDuplicatedBranches(t *testing.T) {
	for name, mutate := range map[string]func(*epathSQLZoneServiceProof){
		"also direct cohort":    func(p *epathSQLZoneServiceProof) { p.DirectHVAC = true },
		"direct subtotal label": func(p *epathSQLZoneServiceProof) { p.Basis = "direct_zone_energy" },
		"wrong service":         func(p *epathSQLZoneServiceProof) { p.NativeVRF.Service = "heating" },
		"wrong period":          func(p *epathSQLZoneServiceProof) { p.NativeVRF.Period = "hourly" },
		"another owner local":   func(p *epathSQLZoneServiceProof) { p.NativeVRF.ZoneName = "SPACE2-1" },
		"wrong carrier": func(p *epathSQLZoneServiceProof) {
			p.Carriers["natural_gas"] = p.Carriers["electricity"]
			delete(p.Carriers, "electricity")
		},
		"double counted local":  func(p *epathSQLZoneServiceProof) { p.NativeVRF.Value.Value++ },
		"lost shared partition": func(p *epathSQLZoneServiceProof) { p.NativeVRF.Allocated.Value-- },
		"unserved positive":     func(p *epathSQLZoneServiceProof) { p.NativeVRF.Owned = false },
		"ordinary source masquerade": func(p *epathSQLZoneServiceProof) {
			for key, original := range p.NativeVRF.Originals["electricity"] {
				original.NativeVRF = nil
				p.NativeVRF.Originals["electricity"][key] = original
				break
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			service := epathSQLVRFConsumerUnitProof(t)
			mutate(service)
			if err := epathSQLVRFQualitySources(&epathSQLQualityProof{}, service); err == nil {
				t.Fatal("unproved VRF service entered quality/carrier consumers")
			}
		})
	}
	if err := epathSQLVRFQualitySources(nil, epathSQLVRFConsumerUnitProof(t)); err == nil {
		t.Fatal("nil quality destination accepted")
	}
}

func TestEnergyPathRealSQLVRFConsumersUnservedZoneKeepsNoSources(t *testing.T) {
	service := &epathSQLZoneServiceProof{Service: "cooling", Basis: "service_path_allocation", Carriers: map[string]epathSQLQuantity{"electricity": {}},
		NativeVRF: &epathSQLVRFZoneServiceProof{ZoneName: "PLENUM-1", Service: "cooling", Period: "annual"}}
	quality := &epathSQLQualityProof{}
	if err := epathSQLVRFQualitySources(quality, service); err != nil || len(quality.SiteOriginals["cooling"]) != 0 {
		t.Fatalf("unserved known-zero service acquired source evidence: %v", err)
	}
	service.NativeVRF.Originals = epathSQLVRFConsumerUnitProof(t).NativeVRF.Originals
	if err := epathSQLVRFQualitySources(quality, service); err == nil {
		t.Fatal("unserved zero masqueraded as observed equipment ownership")
	}
}

func TestEnergyPathRealSQLVRFConsumersKeepWeightsSeparateFromDeliveredLoad(t *testing.T) {
	service := epathSQLVRFConsumerUnitProof(t)
	item := epathRealOracleMetricRecipe{Scope: "zone", Zone: "SPACE1-1", Period: "M1"}
	check := epathSQLModelCheck{Item: item, Quality: &epathSQLQualityProof{
		Dependencies: []epathSQLModelCheck{{Item: item, ZoneService: service}},
		LoadSources:  map[string][]int{"cooling": {302}},
	}}
	weights, err := epathSQLVRFQualityWeightSources(check, "cooling")
	if err != nil || len(weights) != 5 {
		t.Fatalf("complete original denominator was not retained: %v", err)
	}
	if len(check.Quality.LoadSources["cooling"]) != 1 || len(service.LoadSources) != 1 || len(service.NativeVRF.Originals["electricity"]) != 3 {
		t.Fatal("denominator was promoted to delivered load or site consumption")
	}
	for _, original := range weights {
		if original.NativeVRF != nil || original.RDD == nil || original.RDD.Name != "Zone Air System Sensible Cooling Energy" {
			t.Fatal("weight trace contains a consumption source")
		}
	}
	check.Quality.Dependencies[0].Item.Period = "M2"
	if _, err := epathSQLVRFQualityWeightSources(check, "cooling"); err == nil {
		t.Fatal("foreign-period denominator proof accepted")
	}
	if none, err := epathSQLVRFQualityWeightSources(epathSQLModelCheck{Quality: &epathSQLQualityProof{}}, "cooling"); err != nil || len(none) != 0 {
		t.Fatalf("ordinary model acquired VRF source checks: %v", err)
	}
}
