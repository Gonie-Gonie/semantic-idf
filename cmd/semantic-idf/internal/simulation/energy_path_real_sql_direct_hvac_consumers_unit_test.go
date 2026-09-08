package simulation

import "testing"

func TestEnergyPathSQLDirectHVACCarrierUsesActualBranches(t *testing.T) {
	proof := &epathSQLZoneServiceProof{
		Service: "heating", Basis: "direct_zone_energy", DirectHVAC: true,
		Carriers: map[string]epathSQLQuantity{"electricity": {Value: 5}, "natural_gas": {Value: 7}},
		Branches: map[string]epathSQLDirectHVACBranchProof{
			"direct_zone_energy|electricity":      {Basis: "direct_zone_energy", Carrier: "electricity", Quantity: epathSQLQuantity{Value: 2}},
			"service_path_allocation|electricity": {Basis: "service_path_allocation", Carrier: "electricity", Quantity: epathSQLQuantity{Value: 3}},
			"service_path_allocation|natural_gas": {Basis: "service_path_allocation", Carrier: "natural_gas", Quantity: epathSQLQuantity{Value: 7}},
		},
	}
	direct, err := epathSQLDirectHVACCarrierParts(proof)
	if err != nil || direct["electricity"].Value != 2 || direct["natural_gas"].Value != 0 {
		t.Fatalf("node's preferred direct basis must not relabel allocated branches: direct=%v err=%v", direct, err)
	}
	proof.Carriers["electricity"] = epathSQLQuantity{Value: 6}
	if _, err := epathSQLDirectHVACCarrierParts(proof); err == nil {
		t.Fatal("accepted branch sum contradicting independent carrier subtotal")
	}
	proof.Carriers["electricity"] = epathSQLQuantity{Value: 5}
	proof.Branches["unexpected"] = epathSQLDirectHVACBranchProof{Basis: "direct_zone_energy", Carrier: "district_cooling", Quantity: epathSQLQuantity{Value: 1}}
	if _, err := epathSQLDirectHVACCarrierParts(proof); err == nil {
		t.Fatal("accepted undeclared carrier")
	}
}

func TestEnergyPathSQLDirectHVACQualityCannotInventOriginals(t *testing.T) {
	proof := &epathSQLZoneServiceProof{
		Service: "cooling", Basis: "direct_zone_energy", DirectHVAC: true,
		Carriers: map[string]epathSQLQuantity{"electricity": {Value: 2}},
		Branches: map[string]epathSQLDirectHVACBranchProof{
			"direct_zone_energy|electricity": {Basis: "direct_zone_energy", Carrier: "electricity", Quantity: epathSQLQuantity{Value: 2}},
		},
	}
	quality := &epathSQLQualityProof{
		SiteSources:   map[string]map[string][]int{"cooling": {"electricity": {99}}},
		SiteOriginals: map[string]map[string]map[string]epathSQLOriginalSource{},
	}
	if err := epathSQLDirectHVACQualitySources(quality, proof); err == nil {
		t.Fatal("unrelated preexisting broad meter was allowed to prove missing direct consumption")
	}
	if quality.SiteSources["cooling"]["electricity"][0] != 99 {
		t.Fatal("failed proof mutated original quality evidence")
	}
}
