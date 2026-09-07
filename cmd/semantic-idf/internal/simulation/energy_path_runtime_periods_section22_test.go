package simulation

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Official examples retain their original Hourly requests when the Monthly
// Energy Path plan is added. Those source series must survive without building
// one full graph per hour, recursively repeated for every Zone.
func TestEnergyPathRuntimeMonthlyProjectionKeepsOriginalHourlyEvidence(t *testing.T) {
	doc, err := idf.Parse(`Version,25.1; Zone,Office,0,0,0,0,1,1;
Material:NoMass,Insulation,Rough,2; Construction,Wall Construction,Insulation;
BuildingSurface:Detailed,Seasonal Wall,Wall,Wall Construction,Office,,Outdoors,,SunExposed,WindExposed,0.5,4,
0,0,0,0,0,3,4,0,3,4,0,0;`)
	if err != nil {
		t.Fatal(err)
	}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	path := epath192SeasonalWallSQL(t, [12]float64{12, 10, 6, 2, -2, -6, -12, -10, -6, -2, 2, 6})
	baseline, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	want := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(baseline.Series, baseline.Sources, plan, context))
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO ReportDataDictionary VALUES (101,'Seasonal Wall','Surface Inside Face Convection Heat Gain Energy','J',0,'Hourly','Surface'),(102,'Office','Zone Air System Sensible Heating Energy','J',0,'Hourly','Zone'),(103,'Office','Zone Air System Sensible Cooling Energy','J',0,'Hourly','Zone')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	for hour := 1; hour <= 24; hour++ {
		if _, err := db.Exec(`INSERT INTO "Time" VALUES (?,?,2,?,0)`, 100+hour, 1, hour); err != nil {
			db.Close()
			t.Fatal(err)
		}
		for dictionary := 101; dictionary <= 103; dictionary++ {
			if _, err := db.Exec(`INSERT INTO ReportData VALUES (?,?,?,?)`, 1000+hour*3+dictionary, 100+hour, dictionary, float64(100+hour)*3600000); err != nil {
				db.Close()
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	hourly := false
	for _, series := range parsed.Series {
		hourly = hourly || len(series.Hourly) == 24
	}
	if !hourly {
		t.Fatal("fixture lacks original Hourly observations")
	}
	legacyRuntime := buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, plan, context)
	assertPeriods := func(label string, periods []EnergyPeriod) {
		t.Helper()
		if len(periods) != 13 {
			t.Fatalf("%s materialized %d periods, want annual + twelve months", label, len(periods))
		}
		seen := map[string]bool{}
		for _, period := range periods {
			if period.ID != "annual" && !strings.HasPrefix(period.ID, "M") {
				t.Fatalf("%s built unused %s", label, period.ID)
			}
			seen[period.ID] = true
		}
		for month := 1; month <= 12; month++ {
			if !seen[fmt.Sprintf("M%d", month)] {
				t.Fatalf("%s missing M%d", label, month)
			}
		}
	}
	assertPeriods("runtime pre-upgrade", legacyRuntime.Periods)
	got := UpgradeEnergyExplanationV1(legacyRuntime)
	assertPeriods("Building", got.Periods)
	if len(got.ZoneResults) != 1 {
		t.Fatal("fixture must exercise per-Zone conversion")
	}
	for _, zone := range got.ZoneResults {
		assertPeriods(zone.Scope.ZoneName, zone.Periods)
	}
	values := func(periods []EnergyPeriod) map[string]float64 {
		out := map[string]float64{}
		for _, period := range periods {
			for _, node := range period.Nodes {
				out[period.ID+"/node/"+node.ID] = node.Value
			}
			for _, link := range period.Links {
				out[period.ID+"/from/"+link.ID], out[period.ID+"/to/"+link.ID] = link.FromValue, link.ToValue
			}
		}
		return out
	}
	if !reflect.DeepEqual(values(want.Periods), values(got.Periods)) {
		t.Fatal("original Hourly outputs changed canonical annual/monthly values")
	}
	for _, zone := range got.ZoneResults {
		if !reflect.DeepEqual(values(want.ZoneResults[0].Periods), values(zone.Periods)) {
			t.Fatal("original Hourly outputs changed Zone annual/monthly values")
		}
	}
	for _, id := range []string{"sql-rdd-101", "sql-rdd-102", "sql-rdd-103"} {
		if energyExplanationSourceByID(got.Sources, id) == nil {
			t.Fatalf("original Hourly source %s was deleted", id)
		}
	}
	// The frozen adapter remains capable of returning detailed periods.
	classic := buildEnergyExplanationResult(parsed.Series, parsed.Sources, plan)
	hourly = false
	for _, period := range classic.Periods {
		hourly = hourly || period.Kind == "hourly"
	}
	if !hourly {
		t.Fatal("runtime guard removed detailed periods from the frozen v1 adapter")
	}
}
