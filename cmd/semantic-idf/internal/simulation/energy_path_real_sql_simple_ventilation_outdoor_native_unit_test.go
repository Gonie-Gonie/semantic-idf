package simulation

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// All rows below are synthetic hand literals; no actual capture quantities.
func epathSQLSimpleVentilationOutdoorNativeUnit(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames) {
	t.Helper()
	observed, model, frames := epathSQLSimpleVentilationZeroServiceUnit(t)
	_, declaration := epathSQLSimpleVentilationOutdoorUnit()
	model.Families, model.HourlyCompanions = declaration.Families, declaration.HourlyCompanions
	db, err := sql.Open("sqlite", observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := tx.Exec(query, args...); e != nil {
			t.Fatal(e)
		}
	}
	start := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
	for hour := 0; hour < 8760; hour++ {
		stamp := start.Add(time.Duration(hour) * time.Hour)
		exec("INSERT INTO Time VALUES(?,1,2017,?,?,?,0,60,NULL,1,?)", 10000+hour, int(stamp.Month()), stamp.Day(), stamp.Hour()+1, stamp.YearDay())
	}
	for fi, family := range model.Families {
		a := family.Terms[0].Source.Alternatives[0]
		value := []float64{2 * 3600000, 3600000, 5 * 3600000, 3 * 3600000, 1000}[fi]
		typ := "Sum"
		if a.Unit == "W" {
			typ = "Avg"
		}
		for zi, zone := range family.Keys {
			hID := 100 + fi*6 + zi*2
			mID := hID + 1
			for _, spec := range []struct {
				id        int
				frequency string
			}{{hID, "Hourly"}, {mID, "Monthly"}} {
				exec("INSERT INTO ReportDataDictionary VALUES(?,?,?,0,?,?,'System',?,'HVAC System','')", spec.id, a.Name, zone, spec.frequency, a.Unit, typ)
				observed.executedText += "\nOutput:Variable," + zone + "," + a.Name + "," + spec.frequency + ";"
				observed.outputPlan.OutputObjects = append(observed.outputPlan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: a.Name, KeyValue: zone, ReportingFrequency: spec.frequency, State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: zone}, {Value: a.Name}, {Value: spec.frequency}}})
			}
			exec("INSERT INTO ReportData SELECT ?*100000+TimeIndex,?,TimeIndex,? FROM Time WHERE TimeIndex BETWEEN 10000 AND 18759", hID, hID, value)
			for month := 1; month <= 12; month++ {
				v := value
				if a.Unit == "J" {
					v *= float64(time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24)
				}
				exec("INSERT INTO ReportData VALUES(?,?,?,?)", mID*100000+month, mID, month, v)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	native, err := epathReadRealSQLOracle(observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	native.originalText, native.executedText, native.outputPlan = observed.originalText, observed.executedText, observed.outputPlan
	observed = native
	frames.Cells = map[string]*epathSQLCell{}
	for _, family := range model.Families {
		for _, s := range observed.Sources {
			if s.Name != family.Terms[0].Source.Alternatives[0].Name || s.ReportingFrequency != "Monthly" {
				continue
			}
			q, err := epathSQLMonthly(s, model.Precision)
			if err != nil {
				t.Fatal(err)
			}
			z := strings.ToLower(s.KeyValue)
			frames.SourceIdentities[s.DictionaryIndex] = s
			frames.SourceRaw[s.DictionaryIndex] = q
			frames.SourceEffective[s.DictionaryIndex] = q
			frames.SourceZone[s.DictionaryIndex] = z
			for mi, value := range q {
				v := value.times(family.Terms[0].Sign)
				ids := []int{s.DictionaryIndex}
				for _, detail := range family.Subtract {
					c := frames.Cells[epathSQLKey(z, detail, mi+1)]
					v = v.add(c.Effective.times(-1))
					ids = epathSQLDictionaryUnion(ids, c.SourceIDs)
				}
				frames.Cells[epathSQLKey(z, family.ID, mi+1)] = &epathSQLCell{Zone: z, Family: family.ID, Category: family.Category, Component: family.Component, Month: mi + 1, Raw: v, Effective: v, BuildingVisible: true, SourceIDs: ids}
			}
		}
	}
	if err := epathSQLBindHourlyCompanions(observed.sqlPath, observed.Sources, model, &frames); err != nil {
		t.Fatal(err)
	}
	return observed, model, frames
}

func TestEnergyPathSQLSimpleVentilationOutdoorNativeOwnersRequestsAndCalendars(t *testing.T) {
	observed, model, frames := epathSQLSimpleVentilationOutdoorNativeUnit(t)
	if err := epathSQLValidateSimpleVentilationOutdoorEvidence(observed, frames, model); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLDriverReconciliationChecks(frames, model, []string{"zone 1", "zone 2", "zone 3"}, []string{"annual"}, &epathSQLModelChecks{}); err != nil {
		t.Fatal(err)
	}
	// The literal builder has closed SQL and bound all native companions.
	// Reuse only its immutable bytes; every negative owns a new SQL file and
	// detached model/frames, including nested companion calendar/authority.
	data, err := os.ReadFile(observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*epathRealOracleEvidence, *epathRealSQLModel, *epathSQLFrames){
		"missing actual Hourly request": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			for i, p := range o.outputPlan.OutputObjects {
				if p.VariableName == "Zone Ventilation Latent Heat Loss Energy" && p.KeyValue == "ZONE 2" && p.ReportingFrequency == "Hourly" {
					o.outputPlan.OutputObjects = append(o.outputPlan.OutputObjects[:i], o.outputPlan.OutputObjects[i+1:]...)
					return
				}
			}
			t.Fatal("missing fixture request")
		},
		"original member alias": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			o.originalText = strings.Replace(o.originalText, "ZONE 3 Ventl 2", "FOREIGN", 1)
		},
		"missing companion declaration": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			m.HourlyCompanions = m.HourlyCompanions[:4]
		},
		"missing companion proof": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			delete(f.TraceSourceIdentities, 100)
		},
		"missing Hourly chart row": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			p := f.TraceSourceIdentities[100].NativeCompanion
			p.HourlyLabels = p.HourlyLabels[:8759]
		},
		"missing Monthly authority": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			p := f.TraceSourceIdentities[100].NativeCompanion
			p.Authority.Months = p.Authority.Months[:11]
		},
		"misplaced native owner": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			f.SourceZone[101] = "zone 2"
		},
		"orphan native row": func(o *epathRealOracleEvidence, m *epathRealSQLModel, f *epathSQLFrames) {
			epathOracleEditSQL(t, o.sqlPath, "INSERT INTO ReportData VALUES(99000000,100,99000000,0)")
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := epathSQLHeatOnlyUnitSQLCopy(t, observed, data)
			m := epathSQLHeatOnlyClone(t, model)
			f := epathSQLHeatOnlyClone(t, frames)
			edit(&o, &m, &f)
			if err := epathSQLValidateSimpleVentilationOutdoorEvidence(o, f, m); err == nil {
				t.Fatal("unproved finite outdoor authority accepted")
			}
		})
	}
	// Native signs are checked before 0.001 chart rounding can hide a negative.
	epathOracleEditSQL(t, observed.sqlPath, "UPDATE ReportData SET Value=-0.00001 WHERE ReportDataDictionaryIndex=100 AND TimeIndex=10000")
	if err := epathSQLValidateSimpleVentilationOutdoorEvidence(observed, frames, model); err == nil {
		t.Fatal("sub-quantum negative native gain accepted")
	}
}

func TestEnergyPathSQLSimpleVentilationOutdoorNativeCalendarLossRejectedAtCompilation(t *testing.T) {
	observed, model, frames := epathSQLSimpleVentilationOutdoorNativeUnit(t)
	epathOracleEditSQL(t, observed.sqlPath, "DELETE FROM ReportData WHERE ReportDataDictionaryIndex=100 AND TimeIndex=10000")
	// Call the existing native companion compiler anew: a stale in-memory proof
	// is not used to establish a changed database's complete native calendar.
	native, err := epathReadRealSQLOracle(observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := epathCompileSQLHourlyCompanions(observed.sqlPath, native.Sources, model, frames); err == nil {
		t.Fatal("lost native Hourly row became complete")
	}
}
