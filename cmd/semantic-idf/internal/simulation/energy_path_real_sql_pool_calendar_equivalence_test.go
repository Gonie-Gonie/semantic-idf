package simulation

// Calendar/data-structure optimization regression only. These retain every
// native row and the actual TimeIndex namespace; no engine or SQL is needed.
import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestEnergyPathRealSQLPoolNativeCalendarFiniteAnnualEquivalence(t *testing.T) {
	for _, frequency := range []string{"Monthly", "Hourly"} {
		for _, unit := range []string{"J", "W"} {
			for _, power := range []float64{0, 1.25} {
				t.Run(fmt.Sprintf("%s/%s/%g", frequency, unit, power), func(t *testing.T) {
					source, rows := epathSQLPoolSourceHandObservation(77, "Calendar native observation", "Exact Owner", unit, frequency, false, power)
					weather := epathSQLPoolSourceHandWeather()
					var wantRaw, wantEnergy [12]float64
					for month, bucket := range source.Months {
						wantRaw[month], wantEnergy[month] = *bucket.RawSum, *bucket.EnergyKWh
					}
					// Native identifiers need not equal slots, be consecutive,
					// or follow input row order. Calendar closure still must hold.
					for index := range rows {
						rows[index].TimeIndex = 100003 + index*13
					}
					for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
						rows[left], rows[right] = rows[right], rows[left]
					}
					before := append([]epathSQLPoolNativeRow(nil), rows...)
					raw, energy, err := epathSQLValidatePoolNativeRows(source, weather, rows)
					if err != nil || raw != wantRaw || energy != wantEnergy {
						t.Fatalf("complete native %s/%s calendar or unchanged arithmetic failed: raw=%v energy=%v err=%v", frequency, unit, raw, energy, err)
					}
					if !reflect.DeepEqual(rows, before) {
						t.Fatal("native validation changed row order, identifiers or observations")
					}
					if frequency == "Hourly" {
						first, last := rows[len(rows)-1], rows[0]
						if first.Month != 1 || first.Day != 1 || first.Hour != 1 || first.SimulationDays != 1 || last.Month != 12 || last.Day != 31 || last.Hour != 24 || last.SimulationDays != 365 {
							t.Fatal("literal annual source lost exact first/last Hourly slots")
						}
					}
				})
			}
		}
	}
}

func TestEnergyPathRealSQLPoolNativeCalendarRejectsExactBoundaryMutations(t *testing.T) {
	const invalid = "Pool native source: unknown, negative, duplicated or invalid native interval"
	const incomplete = "Pool native source: incomplete controlled annual/source identity"
	const repeated = "Pool native source: duplicate calendar slot"
	const monthly = "Pool native source: Monthly row is not one complete actual month"
	const hourly = "Pool native source: Hourly row escaped its actual hourly interval"
	const summary = "Pool native source: reported monthly summary differs from native rows or knownness"
	tests := []struct {
		name, frequency, want string
		mutate                func(*epathRealSQLSource, *epathRealSQLWeather, []epathSQLPoolNativeRow)
	}{
		{"month zero", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Month = 0 }},
		{"month thirteen", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Month = 13 }},
		{"day zero", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Day = 0 }},
		{"January day thirty two", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Day = 32 }},
		{"nonleap February twenty nine", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].Month, rows[0].Day, rows[0].SimulationDays = 2, 29, 60
		}},
		{"April day thirty one", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].Month, rows[0].Day, rows[0].SimulationDays = 4, 31, 121
		}},
		{"wrong row year", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Year = 2016 }},
		{"wrong cumulative day", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].SimulationDays++
		}},
		{"wrong native environment", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].EnvironmentIndex++
		}},
		{"nonzero minute", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Minute = 1 }},
		{"zero TimeIndex", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].TimeIndex = 0
		}},
		{"duplicate TimeIndex distinct calendar", "", invalid, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[1].TimeIndex = rows[0].TimeIndex
		}},
		{"duplicate calendar distinct TimeIndex", "", repeated, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			index := rows[1].TimeIndex
			rows[1] = rows[0]
			rows[1].TimeIndex = index
		}},
		{"hour zero", "Hourly", hourly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Hour = 0 }},
		{"hour twenty five", "Hourly", hourly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[len(rows)-1].Hour = 25
		}},
		{"wrong Hourly interval type", "Hourly", hourly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].IntervalType = 3
		}},
		{"short Hourly interval", "Hourly", hourly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].IntervalMinutes = 30
		}},
		{"Monthly hour zero", "Monthly", monthly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Hour = 0 }},
		{"Monthly hour twenty five", "Monthly", monthly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) { rows[0].Hour = 25 }},
		{"Monthly before month end", "Monthly", monthly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].Day--
			rows[0].SimulationDays--
		}},
		{"wrong Monthly interval type", "Monthly", monthly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].IntervalType = 1
		}},
		{"short Monthly interval", "Monthly", monthly, func(_ *epathRealSQLSource, _ *epathRealSQLWeather, rows []epathSQLPoolNativeRow) {
			rows[0].IntervalMinutes -= 60
		}},
		{"foreign leap weather", "", incomplete, func(_ *epathRealSQLSource, weather *epathRealSQLWeather, _ []epathSQLPoolNativeRow) {
			weather.Year = 2016
		}},
		{"invalid weather environment", "", incomplete, func(_ *epathRealSQLSource, weather *epathRealSQLWeather, _ []epathSQLPoolNativeRow) {
			weather.EnvironmentIndex = 0
		}},
		{"missing weather month", "", incomplete, func(_ *epathRealSQLSource, weather *epathRealSQLWeather, _ []epathSQLPoolNativeRow) {
			weather.Months = weather.Months[:11]
		}},
		{"blank weather basis", "", incomplete, func(_ *epathRealSQLSource, weather *epathRealSQLWeather, _ []epathSQLPoolNativeRow) {
			weather.CoverageBasis = ""
		}},
		{"reordered weather months", "", summary, func(_ *epathRealSQLSource, weather *epathRealSQLWeather, _ []epathSQLPoolNativeRow) {
			weather.Months[0], weather.Months[1] = weather.Months[1], weather.Months[0]
		}},
	}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		for _, test := range tests {
			if test.frequency != "" && test.frequency != frequency {
				continue
			}
			t.Run(frequency+"/"+test.name, func(t *testing.T) {
				// Zero observations keep calendar and ownership failures from
				// being hidden behind a coincidental scalar-sum mismatch.
				source, rows := epathSQLPoolSourceHandObservation(77, "Calendar native observation", "Exact Owner", "J", frequency, false, 0)
				weather := epathSQLPoolSourceHandWeather()
				test.mutate(&source, &weather, rows)
				_, _, err := epathSQLValidatePoolNativeRows(source, weather, rows)
				if err == nil || err.Error() != test.want {
					t.Fatalf("native boundary mutation changed established failure: got %v; want %s", err, test.want)
				}
			})
		}
	}
}

func TestEnergyPathRealSQLPoolNativeCalendarMonthEndCensus(t *testing.T) {
	// Independently enumerate every valid day and compare the finite lookup
	// identity with the former calendar authority. This is not source proof;
	// full native J/W and zero/nonzero validations above retain that proof.
	daysBefore := 0
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		for day := 1; day <= last; day++ {
			date := time.Date(2017, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			if date.YearDay() != daysBefore+day || date.Month() != time.Month(month) || date.Day() != day {
				t.Fatalf("finite non-leap lookup disagrees at %d/%d", month, day)
			}
		}
		daysBefore += last
	}
	if daysBefore != 365 {
		t.Fatal("controlled 2017 calendar no longer has 365 days")
	}
}
