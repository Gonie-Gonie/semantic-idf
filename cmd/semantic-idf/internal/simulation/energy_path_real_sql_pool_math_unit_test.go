package simulation

import "testing"

func epathSQLPoolMathFixture() epathSQLPoolPumpMathInput {
	input := epathSQLPoolPumpMathInput{Cooling: map[string][12]*epathSQLQuantity{}, ServedZones: []string{"ZONE A", "ZONE B"}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 3}}
	for month := 0; month < 12; month++ {
		input.Broad[month] = &epathSQLQuantity{Value: 30}
		input.HotWater[month] = &epathSQLQuantity{Value: 10}
		input.ChilledWater[month] = &epathSQLQuantity{Value: 20}
	}
	for name, weight := range map[string]float64{"zone a": 1, "zone b": 3, "return plenum": 100} {
		var months [12]*epathSQLQuantity
		for month := range months {
			months[month] = &epathSQLQuantity{Value: weight}
		}
		input.Cooling[name] = months
	}
	return input
}

func TestEnergyPathRealSQLPoolPumpMathNativeBudgetOnly(t *testing.T) {
	input := epathSQLPoolMathFixture()
	result, err := epathSQLCompilePoolPumpMath(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Months {
		if row.Shares["zone a"].Value != 5 || row.Shares["zone b"].Value != 15 || row.Shares["return plenum"].Value != 0 || row.Allocated.Value != 20 || row.Unassigned.Value != 10 || row.HotWaterObserved.Value != 10 {
			t.Fatalf("native CW-only shares were replaced by broad/HW/plenum allocation: %#v", row)
		}
	}
	if result.AnnualShares["zone a"].Value != 60 || result.AnnualExpected.Value != 360 || result.AnnualUnassigned.Value != 120 {
		t.Fatalf("bad annual source-local ledger: %#v", result)
	}
}

func TestEnergyPathRealSQLPoolPumpMathIntegerQuotaQuantumAndJointBudget(t *testing.T) {
	input := epathSQLPoolMathFixture()
	input.Precision.ContributionStages = 1
	input.ServedZones = []string{"ZONE A", "ZONE B", "ZONE C"}
	input.Cooling = map[string][12]*epathSQLQuantity{}
	for zone, weight := range map[string]float64{"zone a": .34, "zone b": .33, "zone c": .33} {
		var values [12]*epathSQLQuantity
		for month := range values {
			values[month] = &epathSQLQuantity{Value: weight}
		}
		input.Cooling[zone] = values
	}
	for month := 0; month < 12; month++ {
		input.Broad[month], input.HotWater[month], input.ChilledWater[month] = &epathSQLQuantity{Value: .001}, &epathSQLQuantity{}, &epathSQLQuantity{Value: .001}
	}
	result, err := epathSQLCompilePoolPumpMath(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Months {
		// Hand Hamilton result: only the .34 quota receives the one unit.
		for zone, actual := range map[string]float64{"zone a": .001, "zone b": 0, "zone c": 0} {
			q := row.Shares[zone]
			if err := epathCheckSQLModelQuantity(&actual, &q); err != nil {
				t.Fatal(err)
			}
		}
		low, high := row.Allocated.bounds()
		if row.Allocated.Value != .001 || low != .001 || high != .001 || row.Unassigned.Value != 0 {
			t.Fatal("recipient quota errors inflated or consumed the independently conserved CW budget")
		}
		wrong := .002
		if err := epathCheckSQLModelQuantity(&wrong, &row.Allocated); err == nil {
			t.Fatal("a second allocation quantum was manufactured from recipient error bounds")
		}
	}
}

func TestEnergyPathRealSQLPoolPumpMathAnnualSumsFinishedMonths(t *testing.T) {
	input := epathSQLPoolMathFixture()
	a, b := input.Cooling["zone a"], input.Cooling["zone b"]
	for month := 1; month < 12; month++ {
		a[month] = &epathSQLQuantity{}
		b[month] = &epathSQLQuantity{}
	}
	// A 40 kWh second-month budget has the opposite allocation, so an annual
	// load ratio would allocate 30/30 instead of the correct 35/25 kWh.
	a[1], b[1] = &epathSQLQuantity{Value: 3}, &epathSQLQuantity{Value: 1}
	input.Cooling["zone a"], input.Cooling["zone b"] = a, b
	input.ChilledWater[1], input.Broad[1] = &epathSQLQuantity{Value: 40}, &epathSQLQuantity{Value: 50}
	result, err := epathSQLCompilePoolPumpMath(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.AnnualShares["zone a"].Value != 35 || result.AnnualShares["zone b"].Value != 25 || result.AnnualAllocated.Value != 60 || result.Months[2].Unassigned.Value != 30 {
		t.Fatalf("annual recomputation or zero-denominator allocation: %#v", result)
	}
}

func TestEnergyPathRealSQLPoolPumpMathRejectsMissingIdentityAndObservations(t *testing.T) {
	cases := map[string]func(*epathSQLPoolPumpMathInput){
		"missing CW":           func(i *epathSQLPoolPumpMathInput) { i.ChilledWater[0] = nil },
		"missing HW":           func(i *epathSQLPoolPumpMathInput) { i.HotWater[0] = nil },
		"missing broad":        func(i *epathSQLPoolPumpMathInput) { i.Broad[0] = nil },
		"wrong parent":         func(i *epathSQLPoolPumpMathInput) { i.Broad[0] = &epathSQLQuantity{Value: 20} },
		"negative CW":          func(i *epathSQLPoolPumpMathInput) { i.ChilledWater[0] = &epathSQLQuantity{Value: -1} },
		"duplicate recipient":  func(i *epathSQLPoolPumpMathInput) { i.ServedZones = append(i.ServedZones, "zone a") },
		"foreign recipient":    func(i *epathSQLPoolPumpMathInput) { i.ServedZones[0] = "other" },
		"missing primary load": func(i *epathSQLPoolPumpMathInput) { q := i.Cooling["zone a"]; q[0] = nil; i.Cooling["zone a"] = q },
		"zero denominator uncertain": func(i *epathSQLPoolPumpMathInput) {
			for _, z := range []string{"zone a", "zone b"} {
				q := i.Cooling[z]
				q[0] = &epathSQLQuantity{Value: 0, Error: .0005}
				i.Cooling[z] = q
			}
		},
		"finite inputs share overflow": func(i *epathSQLPoolPumpMathInput) {
			i.Broad[0], i.ChilledWater[0], i.HotWater[0] = &epathSQLQuantity{Value: 1e200}, &epathSQLQuantity{Value: 1e200}, &epathSQLQuantity{}
			for _, z := range []string{"zone a", "zone b"} {
				q := i.Cooling[z]
				q[0] = &epathSQLQuantity{Value: 1e200}
				i.Cooling[z] = q
			}
		},
		"finite months annual overflow": func(i *epathSQLPoolPumpMathInput) {
			for month := 0; month < 12; month++ {
				i.Broad[month], i.HotWater[month], i.ChilledWater[month] = &epathSQLQuantity{Value: 1e308}, &epathSQLQuantity{Value: 1e308}, &epathSQLQuantity{}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := epathSQLPoolMathFixture()
			mutate(&input)
			if _, err := epathSQLCompilePoolPumpMath(input); err == nil {
				t.Fatal("invalid independent math input accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolPumpMathMeasuredZeroIsNotMissing(t *testing.T) {
	input := epathSQLPoolMathFixture()
	input.ChilledWater[0], input.Broad[0] = &epathSQLQuantity{}, &epathSQLQuantity{Value: 10}
	result, err := epathSQLCompilePoolPumpMath(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Months[0].Allocated.Value != 0 || result.Months[0].Unassigned.Value != 10 || result.Months[0].ChilledWaterObserved.Value != 0 {
		t.Fatal("native zero lost its source-local meaning")
	}
}
