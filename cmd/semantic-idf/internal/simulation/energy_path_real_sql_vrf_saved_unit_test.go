package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// This opt-in audits only the immutable original SQL/IDF/output plan. It does
// not build/read a candidate, install expected values, or approve a fixture.
func TestEnergyPathRealSQLVRFSavedObservations(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("EPATH_REAL_VRF_OBSERVATIONS_DIR"))
	if directory == "" {
		t.Skip("explicit saved VRF original SQL required; no engine or candidate")
	}
	if os.Getenv("EPATH_REAL_RUN") == "1" || os.Getenv("EPATH_REAL_CAPTURE") == "1" {
		t.Fatal("VRF original observation audit cannot launch an engine")
	}
	root, catalog := epathRealDirectories(t)
	var evidence epathRealRunEvidence
	data, err := os.ReadFile(filepath.Join(directory, "run-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	if !epathRealSamePath(directory, evidence.RunDirectory) || evidence.Fixture.ID != "vrf-25-1" || evidence.Version != "25.1" {
		t.Fatal("exact saved VRF 25.1 original evidence required")
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	// These hashes identify the independently audited capture, not a candidate
	// or an expected manifest. An unrelated/re-run fixture requires a new audit.
	if evidence.SQLSHA256 != "2e5d6f46d1d0fcf89cd433352faf51fb17a24f47ba0ebe4f0a35679c4408cc0e" ||
		evidence.ModelSHA256 != "0b533d3ec7abc449bc592f2853af6fcd994afe8d407075c16b5ec7d0f3eded4c" ||
		evidence.ExecutedSHA256 != "43d89b5f4dc4e8c3edcd1701cb1180112f6cd2c7079c4845574b32da3217e7dd" {
		t.Fatal("VRF audit capture/original/executed identity changed")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	defer func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("SQL-only VRF audit changed preserved artifacts")
		}
	}()
	recipe, err := epathLoadRealOracleRecipe(filepath.Join(catalog, filepath.FromSlash(evidence.Fixture.OraclePath)))
	if err != nil || recipe.SQLModel == nil || len(recipe.SQLModel.NativeVRFSystems) != 1 {
		t.Fatalf("one reviewed native VRF declaration required: %v", err)
	}
	observed, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	observed.outputPlan = evidence.Run.PurposeRunPlan
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, &observed); err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(evidence.SQLPath, observed.Sources, *recipe.SQLModel)
	if err != nil {
		t.Fatal(err)
	}
	if err := epathSQLBindVRFFrames(observed, *recipe.SQLModel, &frames); err != nil {
		t.Fatal(err)
	}
	if len(frames.NativeVRFSystems) != 1 || len(frames.NativeVRFAllocations) != 1 {
		t.Fatal("missing original system/allocation proof")
	}
	system, proof := frames.NativeVRFSystems[0], frames.NativeVRFAllocations[0]
	if len(system.Sources) != 14 || len(proof.Months) != 12 || len(proof.Zones) != 5 {
		t.Fatal("incomplete native constituent/owner/month roster")
	}
	// Original IDF terminal list order is intentionally not the tie-break order.
	if !reflect.DeepEqual(proof.Zones, []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}) {
		t.Fatalf("unexpected canonical owners: %v", proof.Zones)
	}
	ids := map[int]bool{1494: true, 1524: true, 1528: true, 1529: true, 1532: true, 1533: true, 1530: true, 1531: true, 1534: true, 1535: true, 1540: true, 1543: true, 1548: true, 1555: true}
	zeroRows, rows := 0, 0
	for _, source := range system.Sources {
		if !ids[source.Source.DictionaryIndex] {
			t.Fatalf("foreign original source: %d", source.Source.DictionaryIndex)
		}
		delete(ids, source.Source.DictionaryIndex)
		if source.Shared && source.ZoneName != "" || !source.Shared && !strings.HasPrefix(source.ZoneName, "SPACE") {
			t.Fatal("shared/local original ownership changed")
		}
		annualBudget := int64(0)
		for month, q := range source.Months {
			rows++
			if q.Value == 0 {
				zeroRows++
			}
			row := proof.Months[month+1].Sources[source.Source.DictionaryIndex]
			if !reflect.DeepEqual(row.Native, q) {
				t.Fatal("display proof replaced original native source precision")
			}
			closed := row.UnassignedMilliKWh
			for _, share := range row.Shares {
				closed += share.DisplayMilliKWh
			}
			if closed != row.BudgetMilliKWh || row.UnassignedMilliKWh != 0 {
				t.Fatal("observed positive owner loads did not exactly close the original pool")
			}
			annualBudget += row.BudgetMilliKWh
		}
		annual := proof.Annual.Sources[source.Source.DictionaryIndex]
		if annualBudget != annual.BudgetMilliKWh {
			t.Fatal("source annual display is not month-first")
		}
		t.Logf("source %d %s/%s: native %.12f kWh; displayed monthly sum %.3f kWh", source.Source.DictionaryIndex, source.Role, source.Source.KeyValue, annual.Native.Value, float64(annual.BudgetMilliKWh)/1000)
	}
	if rows != 168 || zeroRows != 10 || len(ids) != 0 {
		t.Fatalf("original source rows/known zeros/ids=%d/%d/%d", rows, zeroRows, len(ids))
	}
	for _, service := range []string{"cooling", "heating"} {
		siteID := system.Declaration.CoolingSiteID
		if service == "heating" {
			siteID = system.Declaration.HeatingSiteID
		}
		totalLoad, pairedLoad, nativeSite := 0.0, 0.0, 0.0
		displayedSite, pairedDisplayLoad := int64(0), int64(0)
		for month := 1; month <= 12; month++ {
			native := 0.0
			for _, source := range proof.Months[month].Sources {
				if source.Service == service {
					native += source.Native.Value
				}
			}
			meter, err := epathSQLSitePeriod(frames, siteID, fmt.Sprintf("M%d", month))
			if err != nil || meter == nil {
				t.Fatalf("missing original broad monthly meter: %v", err)
			}
			// Only summation-order floating-point error is allowed here. This is
			// not display rounding or an acceptance tolerance against a candidate.
			ulp := math.Nextafter(math.Max(native, meter.Value), math.Inf(1)) - math.Max(native, meter.Value)
			if math.Abs(native-meter.Value) > 128*ulp {
				t.Fatalf("M%d %s meter/native cohort mismatch: %.15g vs %.15g", month, service, meter.Value, native)
			}
			nativeSite += native
		}
		for _, zone := range proof.Zones {
			annual := proof.Annual.Zones[zone][service]
			totalLoad += annual.Load.Value
			pairedLoad += annual.PairedLoad.Value
			displayedSite += annual.DirectMilliKWh + annual.AllocatedMilliKWh
			pairedDisplayLoad += annual.PairedLoadDisplayMilliKWh
			monthly := int64(0)
			for month := 1; month <= 12; month++ {
				row := proof.Months[month].Zones[zone][service]
				monthly += row.DirectMilliKWh + row.AllocatedMilliKWh
			}
			if monthly != annual.DirectMilliKWh+annual.AllocatedMilliKWh {
				t.Fatal("Zone annual changed month-first integer allocation")
			}
			if service == "heating" {
				july := proof.Months[7].Zones[zone][service]
				if july.Load.Value <= 0 || july.DirectMilliKWh != 0 || july.AllocatedMilliKWh != 0 || len(july.PairMonths) != 0 || len(annual.PairMonths) != 11 {
					t.Fatalf("July physical load/zero-site/pair mismatch: %#v", july)
				}
			}
			t.Logf("Zone %s %s native load %.12f; paired native %.12f; local/shared display %.3f/%.3f", zone, service, annual.Load.Value, annual.PairedLoad.Value, float64(annual.DirectMilliKWh)/1000, float64(annual.AllocatedMilliKWh)/1000)
		}
		wantLoad, wantPaired, wantSite := 31774.122339519017, 31774.122339519017, 10983.596654202734
		if service == "heating" {
			wantLoad, wantPaired, wantSite = 19278.97865827664, 18929.35033342803, 9013.367910417186
		}
		if math.Abs(totalLoad-wantLoad) > 1e-8 || math.Abs(pairedLoad-wantPaired) > 1e-8 || math.Abs(nativeSite-wantSite) > 1e-8 {
			t.Fatalf("original independent annual arithmetic changed: %s %.12f/%.12f/%.12f", service, totalLoad, pairedLoad, nativeSite)
		}
		t.Logf("service %s native site %.12f; display site %.3f; paired native %.12f; paired display %.3f", service, nativeSite, float64(displayedSite)/1000, pairedLoad, float64(pairedDisplayLoad)/1000)
	}
	t.Logf("SQL-only proof PASS: %s; original 14 identities/168 rows/10 known zeros, five owners x13 contexts; no candidate/expected approval", evidence.SQLSHA256)
}
