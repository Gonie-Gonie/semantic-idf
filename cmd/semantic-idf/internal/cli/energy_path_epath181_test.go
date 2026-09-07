package cli

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestEPATH181CLIParsesInterspersedOptionsWithoutChangingPaths(t *testing.T) {
	want := energyPathCLIOptions{
		request: simulation.EnergyPathProjectionRequest{
			ResultPath: `C:\Run results\eplus.sql`, InputPath: `C:\Models\Office model.epJSON`,
			Scope: "zone", Zone: "Office West", Period: "M2", Service: "cooling",
		},
		format: "csv", output: "-", includeTrace: true,
	}
	options := []string{"--input", want.request.InputPath, "--scope", "zone", "--zone", "Office West", "--period", "M2", "--service", "cooling", "--format", "csv", "--include-trace", "--output", "-"}
	for _, offset := range []int{0, 2, 6, len(options)} {
		args := append([]string(nil), options[:offset]...)
		args = append(args, want.request.ResultPath)
		args = append(args, options[offset:]...)
		original := append([]string(nil), args...)
		got, err := parseEnergyPathCLI(args, io.Discard)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("path at %d: got %#v, error %v; want %#v", offset, got, err, want)
		}
		if !reflect.DeepEqual(args, original) {
			t.Fatal("argument normalization mutated the caller's argument slice")
		}
	}
}

func TestEPATH181CLIEqualFormsDefaultsAndTerminator(t *testing.T) {
	defaults, err := parseEnergyPathCLI([]string{"run directory"}, io.Discard)
	if err != nil || defaults.request != (simulation.EnergyPathProjectionRequest{ResultPath: "run directory", Scope: "building", Period: "annual", Service: "all"}) || defaults.format != "json" || defaults.includeTrace || defaults.output != "" {
		t.Fatalf("defaults = %#v, %v", defaults, err)
	}
	equals, err := parseEnergyPathCLI([]string{"run", "--input=model.idf", "--include-trace=true", "--include-trace=false", "--format=CSV", "-o=-"}, io.Discard)
	if err != nil || equals.request.InputPath != "model.idf" || equals.includeTrace || equals.format != "csv" || equals.output != "-" {
		t.Fatalf("equals/bool options = %#v, %v", equals, err)
	}
	terminated, err := parseEnergyPathCLI([]string{"--input=model.idf", "--", "--run-name"}, io.Discard)
	if err != nil || terminated.request.ResultPath != "--run-name" {
		t.Fatalf("explicit option terminator lost dash-prefixed path: %#v, %v", terminated, err)
	}
	flagLikeValue, err := parseEnergyPathCLI([]string{"run", "--zone=--help", "--input", "-model.idf"}, io.Discard)
	if err != nil || flagLikeValue.request.Zone != "--help" || flagLikeValue.request.InputPath != "-model.idf" {
		t.Fatalf("flag-like option values changed: %#v, %v", flagLikeValue, err)
	}
}

func TestEPATH181CLIRejectsInvalidArgumentsBeforeLoading(t *testing.T) {
	for _, item := range []struct {
		name string
		args []string
		want string
	}{
		{"missing result", nil, "exactly one"},
		{"extra result", []string{"one", "two"}, "exactly one"},
		{"after terminator", []string{"run", "--", "--format", "csv"}, "exactly one"},
		{"missing value", []string{"run", "--input"}, "needs an argument"},
		{"missing format value", []string{"run", "--format"}, "needs an argument"},
		{"unknown flag", []string{"run", "--not-a-flag"}, "flag provided but not defined"},
		{"invalid bool", []string{"run", "--include-trace=maybe"}, "invalid boolean value"},
		{"separate bool operand", []string{"run", "--include-trace", "false"}, "exactly one"},
		{"result stdin", []string{"-"}, "cannot be read from stdin"},
		{"model stdin", []string{"run", "--input", "-"}, "model stdin is not supported"},
		{"format before I/O", []string{"missing-run", "--format", "xlsx"}, "unsupported energy-path format"},
		{"no epjson format alias", []string{"missing-run", "--format", "epjson"}, "unsupported energy-path format"},
	} {
		t.Run(item.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"energy-path"}, item.args...)
			handled, code := MaybeRun(args, panicEnergyPathStdin{}, &stdout, &stderr, "test")
			if !handled || code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), item.want) {
				t.Fatalf("handled=%v exit=%d stdout=%q stderr=%q; want %q", handled, code, stdout.String(), stderr.String(), item.want)
			}
		})
	}
}

func TestEPATH181CLIHelpRoutesNeverReadOrRun(t *testing.T) {
	for _, prefix := range [][]string{{"energy-path"}, {"cli", "energy-path"}} {
		var stdout, stderr bytes.Buffer
		args := append(append([]string(nil), prefix...), "nonexistent-run", "--help")
		handled, code := MaybeRun(args, panicEnergyPathStdin{}, &stdout, &stderr, "test")
		if !handled || code != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "never runs EnergyPlus") || !strings.Contains(stderr.String(), "include-trace") {
			t.Fatalf("help route %#v: handled=%v exit=%d stdout=%q stderr=%q", prefix, handled, code, stdout.String(), stderr.String())
		}
	}
	_, err := parseEnergyPathCLI([]string{"--help"}, io.Discard)
	if err != flag.ErrHelp {
		t.Fatalf("help must keep flag.ErrHelp: %v", err)
	}
	var help bytes.Buffer
	writeCLIHelp(&help)
	if !strings.Contains(help.String(), "energy-path") || !strings.Contains(help.String(), "requires filesystem result and model paths") {
		t.Fatalf("global help omits Energy Path input exception: %s", help.String())
	}
}

func TestEPATH181CLIActualSQLMatchesSharedJSONAndCSV(t *testing.T) {
	runDir, sqlPath, inputPaths := energyPathCLIFixture181(t)
	beforeSQL, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, inputPath := range inputPaths {
		beforeInput, err := os.ReadFile(inputPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, resultPath := range []string{runDir, sqlPath} {
			for _, selection := range []struct{ scope, zone, period, service string }{
				{"building", "", "annual", "all"},
				{"zone", "Office West", "M2", "all"},
				{"building", "", "M1", "cooling"},
			} {
				request := simulation.EnergyPathProjectionRequest{ResultPath: resultPath, InputPath: inputPath, Scope: selection.scope, Zone: selection.zone, Period: selection.period, Service: selection.service}
				projection, err := simulation.LoadEnergyPathProjection(request)
				if err != nil {
					t.Fatalf("shared fixture projection %#v: %v", request, err)
				}
				wantJSON, err := json.MarshalIndent(projection, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(wantJSON, []byte(`"semantic-idf.energy-explanation/v2"`)) || !bytes.Contains(wantJSON, []byte("carrier.electricity.building")) {
					t.Fatalf("fixture did not exercise an actual V2 energy result: %s", wantJSON)
				}
				for _, format := range []string{"json", "csv"} {
					for _, trace := range []bool{false, true} {
						var stdout, stderr bytes.Buffer
						args := []string{"cli", "energy-path", resultPath, "--input", inputPath, "--scope", selection.scope, "--zone", selection.zone, "--period", selection.period, "--service", selection.service, "--format", format}
						if trace {
							args = append(args, "--include-trace")
						}
						handled, code := MaybeRun(args, panicEnergyPathStdin{}, &stdout, &stderr, "test")
						if !handled || code != 0 || stderr.Len() != 0 {
							t.Fatalf("actual CLI %#v: handled=%v code=%d stderr=%s", args, handled, code, stderr.String())
						}
						want := append(append([]byte(nil), wantJSON...), '\n')
						if format == "csv" {
							content, err := simulation.EnergyPathProjectionCSV(projection, trace)
							if err != nil {
								t.Fatal(err)
							}
							want = []byte(content)
						}
						if !bytes.Equal(stdout.Bytes(), want) {
							t.Fatalf("CLI altered shared %s projection (trace=%v, request=%#v)\ngot: %s\nwant: %s", format, trace, request, stdout.Bytes(), want)
						}
					}
				}
			}
		}
		afterInput, err := os.ReadFile(inputPath)
		if err != nil || !bytes.Equal(beforeInput, afterInput) {
			t.Fatalf("read-only export changed model input: %v", err)
		}
	}
	afterSQL, err := os.ReadFile(sqlPath)
	if err != nil || !bytes.Equal(beforeSQL, afterSQL) {
		t.Fatalf("read-only export changed original SQL: %v", err)
	}
	entries, err := os.ReadDir(runDir)
	if err != nil || len(entries) != 3 {
		t.Fatalf("read-only command created run artifacts: %#v, %v", entries, err)
	}
}

func TestEPATH181CLIWritesOnlyRequestedOutput(t *testing.T) {
	_, sqlPath, inputs := energyPathCLIFixture181(t)
	output := filepath.Join(t.TempDir(), "energy path.csv")
	if err := os.WriteFile(output, []byte("previous exported report"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	handled, code := MaybeRun([]string{"energy-path", sqlPath, "--input", inputs[0], "--format", "csv", "-o", output}, panicEnergyPathStdin{}, &stdout, &stderr, "test")
	if !handled || code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("file output route: handled=%v exit=%d stdout=%q stderr=%q", handled, code, stdout.String(), stderr.String())
	}
	content, err := os.ReadFile(output)
	if err != nil || len(content) == 0 || string(content) == "previous exported report" {
		t.Fatalf("requested output missing: %v", err)
	}
}

func TestEPATH181CLIProtectsSQLModelsAndRunMetadata(t *testing.T) {
	for _, variant := range []string{"sql", "model", "manifest", "run-plan", "missing run-plan", "SQL hardlink", "model hardlink", "SQL symlink", "model symlink"} {
		t.Run(variant, func(t *testing.T) {
			directory, sqlPath, inputs := energyPathCLIFixture181(t)
			modelBytes, err := os.ReadFile(inputs[0])
			if err != nil {
				t.Fatal(err)
			}
			sqlBytes, err := os.ReadFile(sqlPath)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(modelBytes)
			manifest := simulation.SimulationRunManifest{
				InputPath: inputs[0], InputHash: hex.EncodeToString(hash[:]),
				ResultFiles: []simulation.SimulationFileInfo{{Name: filepath.Base(sqlPath), Path: sqlPath, Kind: "sqlite"}},
			}
			manifestBytes, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(directory, "semantic-idf-run.json")
			if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			planPath := filepath.Join(directory, "semantic-idf-run-plan.json")
			if variant != "missing run-plan" {
				if err := os.WriteFile(planPath, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			output := sqlPath
			switch variant {
			case "model":
				output = inputs[0]
			case "manifest":
				output = manifestPath
			case "run-plan", "missing run-plan":
				output = planPath
			case "SQL hardlink", "model hardlink", "SQL symlink", "model symlink":
				source := sqlPath
				if strings.HasPrefix(variant, "model") {
					source = inputs[0]
				}
				output = filepath.Join(directory, "output-alias.csv")
				link := os.Link
				if strings.HasSuffix(variant, "symlink") {
					link = os.Symlink
				}
				if err := link(source, output); err != nil {
					t.Skipf("filesystem does not support %s: %v", variant, err)
				}
			}
			// Omit --input deliberately: protection must use the loader's verified
			// manifest-resolved model, not just the literal CLI input option.
			var stdout, stderr bytes.Buffer
			handled, code := MaybeRun([]string{"energy-path", directory, "--format", "csv", "--output", output}, panicEnergyPathStdin{}, &stdout, &stderr, "test")
			if !handled || code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "cannot overwrite") {
				t.Fatalf("%s collision accepted: handled=%v exit=%d stdout=%q stderr=%q", variant, handled, code, stdout.String(), stderr.String())
			}
			for path, expected := range map[string][]byte{inputs[0]: modelBytes, sqlPath: sqlBytes, manifestPath: manifestBytes} {
				actual, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(actual, expected) {
					t.Fatalf("protected input %s changed: %v", path, err)
				}
			}
			plan, err := os.ReadFile(planPath)
			if variant == "missing run-plan" {
				if !os.IsNotExist(err) {
					t.Fatalf("output created reserved run metadata: %s, %v", plan, err)
				}
			} else if err != nil || string(plan) != "{}" {
				t.Fatalf("run plan was changed: %s, %v", plan, err)
			}
		})
	}
}

func energyPathCLIFixture181(t *testing.T) (string, string, []string) {
	t.Helper()
	directory := t.TempDir()
	inputs := []string{filepath.Join(directory, "Office model.idf"), filepath.Join(directory, "Office model.epJSON")}
	for index, content := range []string{
		"Version,24.1; Building,CLI Building; Zone,Office West;",
		`{"Version":{"Version 1":{"version_identifier":"24.1"}},"Building":{"CLI Building":{}},"Zone":{"Office West":{}}}`,
	} {
		if err := os.WriteFile(inputs[index], []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter INTEGER, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO ReportDataDictionary VALUES (1,'','Electricity:Facility','J',1,'Monthly','Facility'), (2,'','Cooling:Electricity','J',1,'Monthly','Facility'), (3,'Office West','Zone Lights Electricity Energy','J',0,'Monthly','Zone'), (4,'Office West','Zone Air System Sensible Cooling Energy','J',0,'Monthly','Zone')`,
		`INSERT INTO "Time" VALUES (1,1,31,24,0),(2,2,28,24,0)`,
		`INSERT INTO ReportData VALUES (1,1,1,21600000),(2,2,1,32400000),(3,1,2,7200000),(4,2,2,10800000),(5,1,3,3600000),(6,2,3,7200000),(7,1,4,28800000),(8,2,4,43200000)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatalf("SQL fixture: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return directory, path, inputs
}

type panicEnergyPathStdin struct{}

func (panicEnergyPathStdin) Read([]byte) (int, error) {
	panic("Energy Path path-only command must not read stdin")
}
