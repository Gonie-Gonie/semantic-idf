package idf

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func hvacOperationSchedulesFixture(t *testing.T, source string) []HVACOperationSchedule {
	t.Helper()
	doc, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return buildHVACOperationSchedules(doc)
}

func hvacOperationFindSchedule(t *testing.T, schedules []HVACOperationSchedule, objectType, name string) HVACOperationSchedule {
	t.Helper()
	for _, schedule := range schedules {
		if strings.EqualFold(schedule.ObjectType, objectType) && strings.EqualFold(schedule.Name, name) {
			return schedule
		}
	}
	t.Fatalf("missing %s %q", objectType, name)
	return HVACOperationSchedule{}
}

func TestHVACOperationSchedulesPreserveConstantsAndExactMinuteIntervals(t *testing.T) {
	schedules := hvacOperationSchedulesFixture(t, `
Schedule:Constant, Off, , 0;
Schedule:Constant, NegativeSetpoint, , -2.5;
Schedule:Compact, Seasonal, ,
 Through: 2/29, For: Weekdays, Until: 08:15, 0, Until: 18:45, 1, Until: 24:00, 0,
 For: Weekends Holidays, Until: 24:00, 0.5,
 For: AllOtherDays, Until: 24:00, 0.25,
 Through 12/31, For AllDays, Interpolate No, Until 24:00, 1;
`)
	for _, schedule := range schedules {
		if schedule.Status != "supported" {
			t.Fatalf("%s unexpectedly unknown: %s", schedule.Name, schedule.Reason)
		}
	}
	if schedules[0].Value == nil || *schedules[0].Value != 0 || schedules[1].Value == nil || *schedules[1].Value != -2.5 {
		t.Fatalf("numeric schedules must preserve zero and negative values: %+v", schedules)
	}
	seasonal := schedules[2]
	if len(seasonal.Rules) != 4 || seasonal.Rules[0].EndDay != 60 || seasonal.Rules[3].StartDay != 61 || seasonal.Rules[3].EndDay != 366 {
		t.Fatalf("season dates lost: %+v", seasonal.Rules)
	}
	if seasonal.Rules[0].Intervals[0].UntilMinute != 495 || seasonal.Rules[0].Intervals[1].UntilMinute != 1125 {
		t.Fatalf("sub-hourly boundaries were rounded: %+v", seasonal.Rules[0].Intervals)
	}
	if seasonal.Rules[0].Selector != "monday tuesday wednesday thursday friday" || seasonal.Rules[2].Selector != "summerdesignday winterdesignday customday1 customday2" {
		t.Fatalf("day selectors or AllOtherDays not resolved: %+v", seasonal.Rules)
	}
	encoded, err := json.Marshal(schedules[0])
	if err != nil || !strings.Contains(string(encoded), `"value":0`) {
		t.Fatalf("zero constant omitted from JSON: %s (%v)", encoded, err)
	}
}

func TestHVACOperationSchedulesResolveDayWeekYearNamespaces(t *testing.T) {
	hourValues := make([]string, 24)
	for index := range hourValues {
		hourValues[index] = fmt.Sprint(index)
	}
	source := `
Schedule:Day:Hourly, Shared, , ` + strings.Join(hourValues, ",") + `;
Schedule:Day:Interval, Other, , No, 07:30, 0, 24:00, 1;
Schedule:Week:Daily, Shared, Shared,Shared,Shared,Shared,Shared,Shared,Shared,Other,Other,Other,Other,Other;
Schedule:Week:Compact, Second, Weekdays, Shared, For: AllOtherDays, Other;
Schedule:Year, Shared, , Shared,11,1,2,28, Second,3,1,10,31;
`
	schedules := hvacOperationSchedulesFixture(t, source)
	for _, schedule := range schedules {
		if schedule.Status != "supported" {
			t.Fatalf("%s %s unexpectedly unknown: %s", schedule.ObjectType, schedule.Name, schedule.Reason)
		}
	}
	year := hvacOperationFindSchedule(t, schedules, "Schedule:Year", "Shared")
	if len(year.Periods) != 4 || year.Periods[0].StartDay != 1 || year.Periods[0].EndDay != 59 || year.Periods[1].StartDay != 60 || year.Periods[1].EndDay != 60 || year.Periods[1].ScheduleName != "Shared" {
		t.Fatalf("wrap date ranges / February 29 inheritance lost: %+v", year.Periods)
	}
	week := hvacOperationFindSchedule(t, schedules, "Schedule:Week:Daily", "Shared")
	if week.WeekDays["monday"] != "Shared" || week.WeekDays["holiday"] != "Other" {
		t.Fatalf("wrong canonical weekday order: %+v", week.WeekDays)
	}
	day := hvacOperationFindSchedule(t, schedules, "Schedule:Day:Hourly", "Shared")
	if len(day.Intervals) != 24 || day.Intervals[23].UntilMinute != 1440 || day.Intervals[23].Value != 23 {
		t.Fatalf("hourly values not preserved: %+v", day)
	}
}

func TestHVACOperationSchedulesDayListPreservesSubhourlyValues(t *testing.T) {
	values := make([]string, 96)
	for index := range values {
		values[index] = fmt.Sprint(index % 2)
	}
	schedules := hvacOperationSchedulesFixture(t, "Schedule:Day:List, QuarterHour, , No, 15,"+strings.Join(values, ",")+";")
	schedule := schedules[0]
	if schedule.Status != "supported" || len(schedule.Intervals) != 96 || schedule.Intervals[0].UntilMinute != 15 || schedule.Intervals[95].UntilMinute != 1440 || schedule.Intervals[1].Value != 1 {
		t.Fatalf("quarter-hour list: %+v", schedule)
	}
}

func TestHVACOperationSchedulesMalformedSchedulesNeverGainFallbackValues(t *testing.T) {
	tests := map[string]string{
		"missing constant":          "Schedule:Constant, Bad, ;",
		"not finite":                "Schedule:Constant, Bad, , NaN;",
		"infinite":                  "Schedule:Constant, Bad, , +Inf;",
		"missing final interval":    "Schedule:Day:Interval, Bad, , No, 18:00, 1;",
		"repeated time":             "Schedule:Day:Interval, Bad, , No, 12:00,0,12:00,1,24:00,1;",
		"invalid time":              "Schedule:Day:Interval, Bad, , No, 24:01,1;",
		"zero time":                 "Schedule:Day:Interval, Bad, , No, 00:00,0,24:00,1;",
		"missing hourly value":      "Schedule:Day:Hourly, Bad, , 1;",
		"list cannot divide hour":   "Schedule:Day:List, Bad, , No, 7,1;",
		"incomplete list":           "Schedule:Day:List, Bad, , No, 60,1;",
		"unknown compact token":     "Schedule:Compact, Bad, , Through: 12/31, For: AllDays, Surprise, Until: 24:00,1;",
		"bad compact day":           "Schedule:Compact, Bad, , Through: 12/31, For: Funday, Until: 24:00,1;",
		"missing compact through":   "Schedule:Compact, Bad, , For: AllDays, Until: 24:00,1;",
		"incomplete year":           "Schedule:Compact, Bad, , Through: 6/30, For: AllDays, Until: 24:00,1;",
		"invalid date":              "Schedule:Compact, Bad, , Through: 2/30, For: AllDays, Until: 24:00,1;",
		"missing compact day types": "Schedule:Compact, Bad, , Through: 12/31, For: Weekdays, Until: 24:00,1;",
		"overlap compact day types": "Schedule:Compact, Bad, , Through: 12/31, For: AllDays, Until: 24:00,1, For: Monday, Until: 24:00,0;",
		"reversed compact periods":  "Schedule:Compact, Bad, , Through: 6/30, For: AllDays, Until: 24:00,1, Through: 3/31, For: AllDays, Until: 24:00,0;",
		"overlap year":              "Schedule:Year, Bad, , Week,1,1,12,31, Week,2,1,2,28;",
		"gap year":                  "Schedule:Year, Bad, , Week,1,1,12,30;",
		"bad year date":             "Schedule:Year, Bad, , Week,1,1,13,1;",
		"incomplete week daily":     "Schedule:Week:Daily, Bad, Day;",
		"missing week day types":    "Schedule:Week:Compact, Bad, Monday, Day;",
		"overlap week day types":    "Schedule:Week:Compact, Bad, AllDays, Day, Monday, Day;",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			schedules := hvacOperationSchedulesFixture(t, source)
			if len(schedules) != 1 {
				t.Fatalf("expected one schedule: %+v", schedules)
			}
			schedule := schedules[0]
			if schedule.Status != "unknown" || schedule.Reason == "" || schedule.Value != nil || len(schedule.Intervals)+len(schedule.Rules)+len(schedule.Periods)+len(schedule.WeekDays) != 0 {
				t.Fatalf("bad schedule exposed a usable fallback: %+v", schedule)
			}
		})
	}
}

func TestHVACOperationSchedulesExternalAndInterpolatedAreUnknown(t *testing.T) {
	schedules := hvacOperationSchedulesFixture(t, `
Schedule:File, File, , missing.csv,1,0,8760,Comma;
ExternalInterface:Schedule, External, , 1;
ExternalInterface:FunctionalMockupUnitImport:To:Schedule, FMU, ,file.fmu, instance,variable,1;
Schedule:Constant, Actuated, , 1;
EnergyManagementSystem:Actuator, EMS, Actuated, Schedule:Constant, Schedule Value;
Schedule:Constant, ExternalActuated, , 1;
ExternalInterface:FunctionalMockupUnitExport:To:Actuator, ExternalEMS, ExternalActuated, Schedule:Constant, Schedule Value, fmu, 1;
Schedule:Day:Interval, AverageDay, , Average,24:00,1;
Schedule:Compact, LinearCompact, ,Through:12/31,For:AllDays,Interpolate:Linear,Until:24:00,1;
Schedule:Constant, Unrelated, , 1;
EnergyManagementSystem:Actuator, OtherEMS, Unrelated, Fan, Fan Pressure Rise;
`)
	for _, schedule := range schedules {
		if schedule.Name == "Unrelated" {
			if schedule.Status != "supported" {
				t.Fatalf("unrelated actuator must not taint schedule: %+v", schedule)
			}
			continue
		}
		if schedule.Status != "unknown" || schedule.Value != nil || len(schedule.Intervals)+len(schedule.Rules) != 0 {
			t.Fatalf("runtime/interpolated value exposed as known: %+v", schedule)
		}
	}
	if len(schedules) != 8 {
		t.Fatalf("actuator objects must not become schedules: %+v", schedules)
	}
}

func TestHVACOperationSchedulesValidateReferencesAndDuplicateNames(t *testing.T) {
	schedules := hvacOperationSchedulesFixture(t, `
Schedule:Constant, Duplicate, ,1;
Schedule:Compact, duplicate, ,Through:12/31,For:AllDays,Until:24:00,1;
Schedule:Week:Compact, MissingWeek, AllDays, MissingDay;
Schedule:Year, MissingYear, ,MissingWeek,1,1,12,31;
Schedule:Day:Interval, UnknownDay, ,Linear,24:00,1;
Schedule:Week:Compact, UnknownWeek, AllDays,UnknownDay;
Schedule:Year, UnknownYear, ,UnknownWeek,1,1,12,31;
`)
	for _, schedule := range schedules {
		if schedule.Status != "unknown" {
			t.Fatalf("unknown dependency or duplicate name did not propagate: %+v", schedule)
		}
	}
	year := hvacOperationFindSchedule(t, schedules, "Schedule:Year", "MissingYear")
	if !strings.Contains(year.Reason, "MissingWeek") || !strings.Contains(year.Reason, "MissingDay") {
		t.Fatalf("reason must preserve dependency provenance: %+v", year)
	}
}

func TestHVACOperationSchedulesIgnoreMisleadingComments(t *testing.T) {
	doc, err := Parse("Schedule:Constant, Correct, , 0;")
	if err != nil {
		t.Fatal(err)
	}
	doc.Objects[0].Fields[1] = Field{Value: "1", Comment: "Hourly Value"}
	doc.Objects[0].Fields[2].Comment = "Schedule Type Limits Name"
	schedules := buildHVACOperationSchedules(doc)
	if schedules[0].Status != "supported" || schedules[0].Value == nil || *schedules[0].Value != 0 {
		t.Fatalf("comments changed canonical field meaning: %+v", schedules[0])
	}
}
