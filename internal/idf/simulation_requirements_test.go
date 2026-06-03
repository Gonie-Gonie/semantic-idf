package idf

import "testing"

func TestInputRequiresWeatherFile(t *testing.T) {
	doc, err := Parse(`
Version, 24.2;

SimulationControl,
  Yes,
  Yes,
  Yes,
  No,
  Yes;

SizingPeriod:WeatherFileDays,
  DesignWinter,
  1,
  1,
  1,
  31;

RunPeriod,
  Year,
  1,
  1,
  12,
  31;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !InputRequiresWeatherFile(doc) {
		t.Fatalf("expected weather-file sizing/run period to require EPW")
	}
}

func TestInputRequiresWeatherFileIgnoresDisabledRunPeriod(t *testing.T) {
	doc, err := Parse(`
Version, 24.2;

SimulationControl,
  No,
  No,
  No,
  Yes,
  No;

RunPeriod,
  Year,
  1,
  1,
  12,
  31;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if InputRequiresWeatherFile(doc) {
		t.Fatalf("weather-file run periods disabled, should not require EPW")
	}
}
