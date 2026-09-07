package simulation

// The real-model oracle deliberately does not call the production SQL reader,
// output classifier, source preference, allocation, or quality builders. Its
// observations remain useful before any reviewed expected manifest exists.
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type epathRealSQLMonth struct {
	Month       int      `json:"month"`
	Rows        int      `json:"rows"`
	MissingRows int      `json:"missingRows"`
	RawSum      *float64 `json:"rawSum"`
	EnergyKWh   *float64 `json:"energyKWh"`
}

type epathRealSQLSource struct {
	DictionaryIndex    int                 `json:"dictionaryIndex"`
	Name               string              `json:"name"`
	KeyValue           string              `json:"keyValue"`
	IsMeter            bool                `json:"isMeter"`
	ReportingFrequency string              `json:"reportingFrequency"`
	SourceUnit         string              `json:"sourceUnit"`
	IndexGroup         string              `json:"indexGroup"`
	Rows               int                 `json:"rows"`
	MissingRows        int                 `json:"missingRows"`
	RawSum             *float64            `json:"rawSum"`
	EnergyKWh          *float64            `json:"energyKWh"`
	Months             []epathRealSQLMonth `json:"months"`
}

type epathRealSQLWeather struct {
	Year             int    `json:"year"`
	EnvironmentIndex int    `json:"environmentIndex"`
	EnvironmentName  string `json:"environmentName"`
	TimeRows         int    `json:"timeRows"`
	Months           []int  `json:"months"`
	FirstMonth       int    `json:"firstMonth"`
	FirstDay         int    `json:"firstDay"`
	LastMonth        int    `json:"lastMonth"`
	LastDay          int    `json:"lastDay"`
	CoverageBasis    string `json:"coverageBasis"`
}

type epathRealOracleEvidence struct {
	Schema        string                  `json:"schema"`
	Status        string                  `json:"status"`
	Acceptance    bool                    `json:"acceptance"`
	Reason        string                  `json:"reason"`
	Weather       epathRealSQLWeather     `json:"weather"`
	Sources       []epathRealSQLSource    `json:"sources"`
	CheckedGroups []string                `json:"checkedGroups"`
	Metrics       []epathRealOracleMetric `json:"metrics,omitempty"`
	sqlPath       string
	outputPlan    *PurposeRunPlan
}

func epathOracleNumber(value float64) *float64 { return &value }

// EnergyPlus SQLiteProcedures.cc reportFreqInts maps Hour/Day/Month/Simulation/
// Year to 1..5. Those insert branches deliberately leave column13 unbound;
// only EachCall/TimeStep bind WarmupFlag. NULL is therefore accepted only in
// those documented aggregate-frequency branches, never unknown timestep data.
// https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/SQLiteProcedures.cc#L1424
const epathOracleNonWarmupSQL = `(t.WarmupFlag=0 OR (t.WarmupFlag IS NULL AND t.IntervalType IN (1,2,3,4,5)))`

func epathOpenOracleSQL(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("oracle SQL is not a regular file")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(absolute + suffix); err == nil {
			return nil, fmt.Errorf("oracle requires a closed SQL snapshot: %s sidecar", suffix)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&_pragma=query_only%281%29"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func epathReadOracleWeather(db *sql.DB) (epathRealSQLWeather, error) {
	var out epathRealSQLWeather
	rows, err := db.Query(`SELECT e.EnvironmentPeriodIndex, e.EnvironmentName, t.Year, t.Month, t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.SimulationDays
FROM "Time" t JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE e.EnvironmentType=3 AND ` + epathOracleNonWarmupSQL + ` AND t.IntervalType IN (-1,0,1,2,3) ORDER BY t.TimeIndex`)
	if err != nil {
		return out, fmt.Errorf("weather-run SQL schema: %w", err)
	}
	defer rows.Close()
	months := map[int]bool{}
	monthlyCoverage := map[int]int{}
	monthlyRows := map[int]int{}
	observedDays := map[int]bool{}
	for rows.Next() {
		var environment, year, month, day, hour, minute, intervalType int
		var interval sql.NullFloat64
		var simulationDays sql.NullInt64
		var name string
		if err := rows.Scan(&environment, &name, &year, &month, &day, &hour, &minute, &interval, &intervalType, &simulationDays); err != nil {
			return out, err
		}
		if month < 1 || month > 12 || day < 1 || day > 31 {
			return out, fmt.Errorf("invalid weather calendar %d/%d", month, day)
		}
		if year != 2017 {
			return out, fmt.Errorf("weather calendar year %d does not match controlled 2017 run", year)
		}
		lastDay := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		if day > lastDay.Day() {
			return out, fmt.Errorf("invalid weather calendar %d/%d", month, day)
		}
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if simulationDays.Valid && simulationDays.Int64 == int64(date.YearDay()) {
			observedDays[date.YearDay()] = true
		}
		if intervalType == 3 {
			monthlyRows[month]++
		}
		if intervalType == 3 && day == lastDay.Day() && hour == 24 && minute == 0 && interval.Valid && interval.Float64 == float64(lastDay.Day()*1440) && simulationDays.Valid && simulationDays.Int64 == int64(lastDay.YearDay()) {
			monthlyCoverage[month]++
		}
		if out.TimeRows == 0 {
			out.Year = year
			out.EnvironmentIndex = environment
			out.EnvironmentName = name
			out.FirstMonth = month
			out.FirstDay = day
			out.LastMonth = month
			out.LastDay = day
		}
		if out.EnvironmentIndex != environment {
			return out, fmt.Errorf("multiple weather environments are ambiguous: %d and %d", out.EnvironmentIndex, environment)
		}
		out.TimeRows++
		months[month] = true
		if month*100+day < out.FirstMonth*100+out.FirstDay {
			out.FirstMonth, out.FirstDay = month, day
		}
		if month*100+day > out.LastMonth*100+out.LastDay {
			out.LastMonth, out.LastDay = month, day
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for month := 1; month <= 12; month++ {
		if months[month] {
			out.Months = append(out.Months, month)
		}
	}
	if len(out.Months) != 12 {
		return out, fmt.Errorf("weather-run observations cover %v, need all 12 months (design days and warmup excluded)", out.Months)
	}
	if len(monthlyRows) > 0 {
		// The native writer uses a full calendar-month Interval even for a
		// partially simulated month. Cumulative SimulationDays is essential:
		// interval length alone cannot prove that the run began on January1.
		for month := 1; month <= 12; month++ {
			if monthlyRows[month] != 1 || monthlyCoverage[month] != 1 {
				return out, fmt.Errorf("full monthly calendar coverage is not proven for month %d: require one complete interval and matching cumulative SimulationDays", month)
			}
		}
		out.CoverageBasis = "monthly_intervals_and_cumulative_simulation_days"
	} else {
		for day := 1; day <= 365; day++ {
			if !observedDays[day] {
				return out, fmt.Errorf("full weather calendar coverage is missing observed calendar/SimulationDays pair for day %d", day)
			}
		}
		out.CoverageBasis = "365_observed_calendar_and_simulation_days"
	}
	return out, nil
}

func epathAssertRealAnnualWeatherSQL(t *testing.T, path string) {
	t.Helper()
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := epathReadOracleWeather(db); err != nil {
		t.Fatal(err)
	}
}

// No interval inference from neighbouring TimeIndex values or a default year:
// a rate needs EnergyPlus's explicit interval; energy observations do not.
func epathOracleEnergy(value float64, unit string, minutes sql.NullFloat64) (float64, bool) {
	unit = strings.ToLower(strings.TrimSpace(unit))
	factor, ok := map[string]float64{"j": 1.0 / 3600000, "kj": 1.0 / 3600, "mj": 1.0 / 3.6, "gj": 1000.0 / 3.6, "wh": .001, "kwh": 1, "mwh": 1000}[unit]
	if ok {
		return value * factor, true
	}
	if unit != "w" && unit != "kw" {
		return 0, false
	}
	if !minutes.Valid || minutes.Float64 <= 0 || math.IsNaN(minutes.Float64) || math.IsInf(minutes.Float64, 0) {
		return 0, false
	}
	factor = minutes.Float64 / 60
	if unit == "w" {
		factor /= 1000
	}
	return value * factor, true
}

func epathReadRealSQLOracle(path string) (epathRealOracleEvidence, error) {
	out := epathRealOracleEvidence{Schema: "semantic-idf.energy-path-sql-oracle/v1", Status: "metadata_only", Acceptance: false,
		Reason: "Independent weather-run SQL observations only; no approved eight-group oracle recipe/expected manifest has been accepted.", Sources: []epathRealSQLSource{}, CheckedGroups: []string{}}
	out.sqlPath = path
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		return out, err
	}
	defer db.Close()
	out.Weather, err = epathReadOracleWeather(db)
	if err != nil {
		return out, err
	}
	rows, err := db.Query(`SELECT d.ReportDataDictionaryIndex, d.Name, COALESCE(d.KeyValue,''), d.IsMeter,
COALESCE(d.ReportingFrequency,''), COALESCE(d.Units,''), COALESCE(d.IndexGroup,''),
t.TimeIndex,t.Month,t.IntervalType,t."Interval",r.Value,t.Year,t.Day,t.Hour,t.Minute,t.SimulationDays
FROM ReportData r JOIN ReportDataDictionary d ON d.ReportDataDictionaryIndex=r.ReportDataDictionaryIndex
JOIN "Time" t ON t.TimeIndex=r.TimeIndex
JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE e.EnvironmentType=3 AND ` + epathOracleNonWarmupSQL + ` ORDER BY d.ReportDataDictionaryIndex,t.TimeIndex,r.ReportDataIndex`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	byID := map[int]int{}
	monthPositions := map[int]map[int]int{}
	var previous [2]int64
	hasPrevious := false
	unknownEnergy := map[int]bool{}
	unknownMonthEnergy := map[[2]int]bool{}
	for rows.Next() {
		var id, intervalType int
		var monthValue sql.NullInt64
		var yearValue, dayValue, hourValue, minuteValue, simulationDays sql.NullInt64
		var index int64
		var name, key, frequency, unit, group string
		var meter bool
		var minutes, value sql.NullFloat64
		if err := rows.Scan(&id, &name, &key, &meter, &frequency, &unit, &group, &index, &monthValue, &intervalType, &minutes, &value, &yearValue, &dayValue, &hourValue, &minuteValue, &simulationDays); err != nil {
			return out, err
		}
		month := int(monthValue.Int64)
		identity := [2]int64{int64(id), index}
		if hasPrevious && previous == identity {
			return out, fmt.Errorf("duplicate ReportData observation dictionary=%d TimeIndex=%d", id, index)
		}
		previous, hasPrevious = identity, true
		position, exists := byID[id]
		if !exists {
			position = len(out.Sources)
			byID[id] = position
			monthPositions[id] = map[int]int{}
			out.Sources = append(out.Sources, epathRealSQLSource{DictionaryIndex: id, Name: name, KeyValue: key, IsMeter: meter, ReportingFrequency: frequency, SourceUnit: unit, IndexGroup: group, Months: []epathRealSQLMonth{}})
		}
		source := &out.Sources[position]
		source.Rows++
		// A RunPeriod scalar emitted at December's final TimeIndex is annual,
		// never a December scalar. Keep its monthly collection empty.
		annual := strings.EqualFold(frequency, "Run Period") || strings.EqualFold(frequency, "RunPeriod") || strings.EqualFold(frequency, "Annual")
		// Real 25.1 meter reports can reuse the final Monthly SQL timestamp
		// for a single RunPeriod scalar. Dictionary frequency remains annual;
		// its timestamp must never promote the value into December.
		terminalAnnual := annual && intervalType == 3 && yearValue.Valid && yearValue.Int64 == 2017 && monthValue.Valid && month == 12 && dayValue.Valid && dayValue.Int64 == 31 && hourValue.Valid && hourValue.Int64 == 24 && minuteValue.Valid && minuteValue.Int64 == 0 && simulationDays.Valid && simulationDays.Int64 == 365
		if annual != (intervalType == 4 || intervalType == 5) && !terminalAnnual {
			return out, fmt.Errorf("dictionary frequency %q contradicts Time.IntervalType %d", frequency, intervalType)
		}
		if intervalType == 5 && (!yearValue.Valid || yearValue.Int64 != 2017) {
			return out, fmt.Errorf("annual source has wrong calendar year")
		}
		if !annual && (!monthValue.Valid || month < 1 || month > 12) {
			return out, fmt.Errorf("nonannual source %s has no valid month", name)
		}
		var bucket *epathRealSQLMonth
		if !annual {
			mp, ok := monthPositions[id][month]
			if !ok {
				mp = len(source.Months)
				monthPositions[id][month] = mp
				source.Months = append(source.Months, epathRealSQLMonth{Month: month})
			}
			bucket = &source.Months[mp]
			bucket.Rows++
		}
		valid := value.Valid && !math.IsNaN(value.Float64) && !math.IsInf(value.Float64, 0)
		if !valid {
			source.MissingRows++
			if bucket != nil {
				bucket.MissingRows++
			}
			continue
		}
		if source.RawSum == nil {
			source.RawSum = epathOracleNumber(0)
		}
		*source.RawSum += value.Float64
		if bucket != nil {
			if bucket.RawSum == nil {
				bucket.RawSum = epathOracleNumber(0)
			}
			*bucket.RawSum += value.Float64
		}
		energy, known := epathOracleEnergy(value.Float64, unit, minutes)
		if terminalAnnual && (strings.EqualFold(unit, "W") || strings.EqualFold(unit, "kW")) {
			known = false
		}
		if known {
			if source.EnergyKWh == nil {
				source.EnergyKWh = epathOracleNumber(0)
			}
			*source.EnergyKWh += energy
			if bucket != nil {
				if bucket.EnergyKWh == nil {
					bucket.EnergyKWh = epathOracleNumber(0)
				}
				*bucket.EnergyKWh += energy
			}
		} else {
			unknownEnergy[id] = true
			unknownMonthEnergy[[2]int{id, month}] = true
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for index := range out.Sources {
		source := &out.Sources[index]
		if (strings.EqualFold(source.ReportingFrequency, "Run Period") || strings.EqualFold(source.ReportingFrequency, "RunPeriod") || strings.EqualFold(source.ReportingFrequency, "Annual")) && source.Rows != 1 {
			return out, fmt.Errorf("annual source %s/%s has %d rows; exact single annual observation required", source.Name, source.KeyValue, source.Rows)
		}
		if source.MissingRows > 0 {
			source.RawSum = nil
			source.EnergyKWh = nil
		}
		if unknownEnergy[source.DictionaryIndex] {
			source.EnergyKWh = nil
		}
		for index := range source.Months {
			month := &source.Months[index]
			if month.MissingRows > 0 {
				month.RawSum = nil
				month.EnergyKWh = nil
			}
			if unknownMonthEnergy[[2]int{source.DictionaryIndex, month.Month}] {
				month.EnergyKWh = nil
			}
			if month.EnergyKWh != nil && !epathOracleFinite(*month.EnergyKWh) {
				return out, fmt.Errorf("nonfinite monthly aggregate for %s/%s", source.Name, source.KeyValue)
			}
		}
		sort.Slice(source.Months, func(i, j int) bool { return source.Months[i].Month < source.Months[j].Month })
		if source.RawSum != nil && !epathOracleFinite(*source.RawSum) || source.EnergyKWh != nil && !epathOracleFinite(*source.EnergyKWh) {
			return out, fmt.Errorf("nonfinite annual aggregate for %s/%s", source.Name, source.KeyValue)
		}
	}
	return out, nil
}

func epathOracleFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func epathCollectRealSQLOracle(t *testing.T, evidence epathRealRunEvidence) epathRealOracleEvidence {
	t.Helper()
	out, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Run != nil {
		out.outputPlan = evidence.Run.PurposeRunPlan
	}
	if strings.TrimSpace(evidence.Fixture.OraclePath) == "" {
		return out
	}
	path := filepath.Join(evidence.CatalogDirectory, filepath.FromSlash(evidence.Fixture.OraclePath))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return out
	} else if err != nil {
		t.Fatal(err)
	}
	recipe, err := epathLoadRealOracleRecipe(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := epathEvaluateRealOracle(&out, recipe, evidence.Bundle); err != nil {
		t.Fatal(err)
	}
	out.Status = "sql_checked_partial_candidate"
	out.Reason = "Only the listed candidate groups were independently checked; full eight-group expected-manifest review remains required."
	if len(out.CheckedGroups) == len(epathRealOracleGroups) {
		out.Status = "sql_checked_candidate"
		out.Reason = "All eight candidate groups have independent SQL checks; approved expected-manifest acceptance has not been performed."
	}
	return out
}

func epathDecodeOracleFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%s: trailing JSON data: %v", path, err)
	}
	return nil
}

// Replays an already closed real SQL even if another process is still building
// its UI projection. This is explicitly observation collection, not acceptance.
func TestEnergyPathRealSQLObservationReplay(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("EPATH_REAL_SQL_REPLAY"))
	if path == "" {
		t.Skip("closed real SQL observation replay requires EPATH_REAL_SQL_REPLAY; not acceptance")
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.TrimSpace(os.Getenv("EPATH_REAL_SQL_OBSERVATIONS")); output != "" {
		root, _ := epathRealDirectories(t)
		absolute, err := filepath.Abs(output)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(filepath.Join(root, ".runtime"), absolute)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatal("SQL observation replay may write only an explicitly named .runtime artifact, never expected manifests")
		}
		epathWriteRealJSON(t, absolute, observed)
	}
	t.Logf("METADATA ONLY, NOT ACCEPTANCE: %d SQL source identities, weather %d/%d/%d–%d/%d/%d, months=%v", len(observed.Sources), observed.Weather.Year, observed.Weather.FirstMonth, observed.Weather.FirstDay, observed.Weather.Year, observed.Weather.LastMonth, observed.Weather.LastDay, observed.Weather.Months)
}
