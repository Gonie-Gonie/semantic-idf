package simulation

import (
	"encoding/json"
	"math"
	"testing"
)

func TestEnergyPathRealSQLRatioKindIndependentBounds(t *testing.T) {
	for _, test := range []struct {
		name, declared, want string
		from, to             epathSQLQuantity
	}{
		{"winter", "efficiency", "efficiency", epathSQLQuantity{Value: 40, Error: .001}, epathSQLQuantity{Value: 100, Error: .001}},
		{"summer", "efficiency", "load_to_fuel", epathSQLQuantity{Value: .1806569780599601, Error: .001}, epathSQLBounded(.001516502816721742, .00029541681749688654, .0027419454527390415)},
		{"reverse seasonal declaration", "load_to_fuel", "efficiency", epathSQLQuantity{Value: 40}, epathSQLQuantity{Value: 100}},
		{"exact threshold", "efficiency", "efficiency", epathSQLQuantity{Value: 1 + 1e-9}, epathSQLQuantity{Value: 1}},
		{"above threshold", "efficiency", "load_to_fuel", epathSQLQuantity{Value: 1 + 2e-9}, epathSQLQuantity{Value: 1}},
		{"site may prune but present pair is fuel ratio", "efficiency", "load_to_fuel", epathSQLQuantity{Value: .1, Error: .001}, epathSQLBounded(.0001, 0, .0011)},
		{"known zero endpoint", "efficiency", "efficiency", epathSQLQuantity{Value: 40}, epathSQLQuantity{}},
		{"COP remains COP below one", "coefficient_of_performance", "coefficient_of_performance", epathSQLQuantity{Value: .5}, epathSQLQuantity{Value: 1}},
		{"COP remains COP above one", "coefficient_of_performance", "coefficient_of_performance", epathSQLQuantity{Value: 4}, epathSQLQuantity{Value: 1}},
		{"mixed site remains generic", "load_to_site_energy", "load_to_site_energy", epathSQLQuantity{Value: 4}, epathSQLQuantity{Value: 1}},
		{"district remains purchased", "load_to_purchased_energy", "load_to_purchased_energy", epathSQLQuantity{Value: 4}, epathSQLQuantity{Value: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := epathSQLConversionRatioKind(test.declared, test.from, test.to)
			if err != nil || got != test.want {
				t.Fatalf("kind %q, error %v; want %s", got, err, test.want)
			}
		})
	}
	for _, test := range []struct {
		name     string
		from, to epathSQLQuantity
	}{
		{"threshold crossing", epathSQLQuantity{Value: 1, Error: .001}, epathSQLQuantity{Value: 1, Error: .001}},
		{"uncertain zero", epathSQLBounded(.0001, 0, .001), epathSQLBounded(.0001, 0, .001)},
		{"negative source", epathSQLQuantity{Value: -1}, epathSQLQuantity{Value: 1}},
		{"invalid tolerance", epathSQLQuantity{Value: 1, Error: math.NaN()}, epathSQLQuantity{}},
		{"missing represented by nonfinite", epathSQLQuantity{Value: math.NaN()}, epathSQLQuantity{Value: 1}},
		{"infinite denominator", epathSQLQuantity{Value: 1}, epathSQLQuantity{Value: math.Inf(1)}},
		{"unordered bounds", epathSQLQuantity{Value: 1, Error: 1, Bounds: &[2]float64{2, 0}}, epathSQLQuantity{Value: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := epathSQLConversionRatioKind("efficiency", test.from, test.to); err == nil {
				t.Fatal("unknown/ambiguous independent evidence guessed a fossil label")
			}
		})
	}
}

func TestEnergyPathRealSQLRatioKindMonthlyBeforeOptionalAnnual(t *testing.T) {
	var pairs [12]epathSQLConversionProof
	pairs[0] = epathSQLConversionProof{From: epathSQLQuantity{Value: 2, Error: .001}, To: epathSQLBounded(.0001, 0, .0011)}
	pairs[1] = epathSQLConversionProof{From: epathSQLQuantity{Value: .5, Error: .001}, To: epathSQLQuantity{Value: 1, Error: .001}}
	if got, err := epathSQLConversionPeriodRatioKind("efficiency", "M1", pairs); err != nil || got != "load_to_fuel" {
		t.Fatalf("pre-optional positive monthly pair lost its classification: %q/%v", got, err)
	}
	if got, err := epathSQLConversionPeriodRatioKind("efficiency", "M2", pairs); err != nil || got != "efficiency" {
		t.Fatalf("independent winter pair: %q/%v", got, err)
	}
	// M1 may be omitted, leaving .5/1; or present, giving 2.5/1.0001.
	// The annual raw center alone cannot distinguish these two legal labels.
	if _, err := epathSQLConversionPeriodRatioKind("efficiency", "annual", pairs); err == nil {
		t.Fatal("raw annual quotient concealed whole-month presentation uncertainty")
	}
	for _, period := range []string{"", "M0", "M13", "M01", "annual.extra"} {
		if _, err := epathSQLConversionPeriodRatioKind("efficiency", period, pairs); err == nil {
			t.Fatalf("invalid period %q accepted", period)
		}
	}
}

func epathSQLRatioKindSeasonalFrames() (epathSQLFrames, epathRealSQLModel) {
	frames, model := epathSQLZoneServiceUnitFrames()
	model.Services[1].RatioKind, model.Services[1].FallbackRatioKind = "efficiency", "efficiency"
	for month := 1; month <= 12; month++ {
		a, b, gas := 10.0, 10.0, 100.0
		if month >= 5 && month <= 9 {
			a, b, gas = .1, .2, .01
		}
		if month == 7 {
			gas = .0001 // Positive SQL evidence, despite optional displayed zero.
		}
		frames.Loads[epathSQLKey("a", "heating", month)] = epathSQLQuantity{Value: a}
		frames.Loads[epathSQLKey("b", "heating", month)] = epathSQLQuantity{Value: b}
		frames.Loads[epathSQLKey("plenum", "heating", month)] = epathSQLQuantity{}
		frames.Site["heat.e"][month-1] = &epathSQLQuantity{}
		frames.Site["heat.g"][month-1] = &epathSQLQuantity{Value: gas}
		frames.SourceRaw[2][month-1], frames.SourceRaw[3][month-1] = *frames.Site["heat.e"][month-1], *frames.Site["heat.g"][month-1]
	}
	return frames, model
}

func TestEnergyPathRealSQLRatioKindBuildingAndZoneCompilation(t *testing.T) {
	frames, model := epathSQLRatioKindSeasonalFrames()
	before, _ := json.Marshal(frames)
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(frames)
	if string(before) != string(after) {
		t.Fatal("classification changed original SQL quantities or known zero evidence")
	}
	seen := 0
	for _, check := range checks.Rows {
		if check.Conversion == nil || check.Item.Target.Service != "heating" || check.Item.Target.Basis != "service_path_allocation" || check.Item.Zone == "Plenum" {
			continue
		}
		seen++
		want := "efficiency"
		if check.Item.Period == "M5" || check.Item.Period == "M6" || check.Item.Period == "M7" || check.Item.Period == "M8" || check.Item.Period == "M9" {
			want = "load_to_fuel"
		}
		if check.Item.Target.RatioKind != want {
			t.Fatalf("%s: kind %s, want %s", check.Want.Key, check.Item.Target.RatioKind, want)
		}
		if check.Item.Zone == "A" && check.Item.Period == "M7" {
			if check.Conversion.To.Value <= 0 || !check.Conversion.To.includesZero() || !check.Conversion.From.includesZero() {
				t.Fatal("classification erased raw tiny positive or changed optional-pair bounds")
			}
		}
	}
	if seen != 3*13 {
		t.Fatalf("Building + two served Zones did not classify all13 periods: %d", seen)
	}
	for _, compile := range []func(epathSQLFrames, epathRealSQLModel, *epathSQLModelChecks) error{epathSQLModelServiceChecks, epathSQLModelZoneServiceChecks} {
		f, m := epathSQLRatioKindSeasonalFrames()
		f.Site["heat.g"][0] = &epathSQLQuantity{Value: 20, Error: .001}
		f.SourceRaw[3][0] = *f.Site["heat.g"][0]
		if err := compile(f, m, &epathSQLModelChecks{}); err == nil {
			t.Fatal("compiler silently chose a kind when independent threshold bounds overlap")
		}
	}
}

func TestEnergyPathRealSQLRatioKindExactCandidateContractRetained(t *testing.T) {
	bundle, check := epathSQLModelConversionFixture()
	from, to := epathSQLQuantity{Value: .1806569780599601, Error: .001}, epathSQLBounded(.001516502816721742, .00029541681749688654, .0027419454527390415)
	kind, err := epathSQLConversionRatioKind("efficiency", from, to)
	if err != nil {
		t.Fatal(err)
	}
	check.Item.Target.Service, check.Item.Target.RatioKind = "heating", kind
	check.Conversion = &epathSQLConversionProof{From: from, To: to}
	check.Quantity = &epathSQLQuantity{Value: from.Value / to.Value}
	for index := range bundle.EnergyExplanation.Nodes {
		bundle.EnergyExplanation.Nodes[index].ServiceKind = "heating"
	}
	link := &bundle.EnergyExplanation.Links[0]
	link.ServiceKind, link.FromValue, link.ToValue, link.Ratio, link.RatioKind = "heating", .181, .001, 181, kind
	if err := epathCheckSQLModelConversion(bundle, check); err != nil {
		t.Fatalf("positive observed pair: %v", err)
	}
	for _, mutate := range []func(*EnergyPathLink){
		func(l *EnergyPathLink) { l.RatioKind = "efficiency" },
		func(l *EnergyPathLink) { l.RatioKind = "" },
		func(l *EnergyPathLink) { l.Ratio = check.Quantity.Value },
		func(l *EnergyPathLink) { l.ToUnit = "MJ" },
		func(l *EnergyPathLink) { l.FromValue = 1 },
		func(l *EnergyPathLink) { l.ToValue = 0 },
	} {
		original := *link
		mutate(link)
		if err := epathCheckSQLModelConversion(bundle, check); err == nil {
			t.Fatal("new classification weakened exact paired kind/value/unit/zero guard")
		}
		*link = original
	}
}
