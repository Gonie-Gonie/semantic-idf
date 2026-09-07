package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

type energyPathCLIOptions struct {
	request      simulation.EnergyPathProjectionRequest
	format       string
	output       string
	includeTrace bool
}

func cliEnergyPath(args []string, stdout, stderr io.Writer) error {
	options, err := parseEnergyPathCLI(args, stderr)
	if err != nil {
		return err
	}
	projection, err := simulation.LoadEnergyPathProjection(options.request)
	if err != nil {
		return err
	}
	var payload []byte
	if options.format == "csv" {
		content, err := simulation.EnergyPathProjectionCSV(projection, options.includeTrace)
		if err != nil {
			return err
		}
		payload = []byte(content)
	} else {
		// JSON is the shared GUI/API projection unchanged; trace is a CSV option.
		payload, err = json.MarshalIndent(projection, "", "  ")
		if err != nil {
			return err
		}
		payload = append(payload, '\n')
	}
	if err := guardEnergyPathOutput(options.output, projection); err != nil {
		return err
	}
	return writeCLITextOutput(options.output, payload, stdout)
}

// Exporting a report must not truncate the data that the read-only loader just
// verified. Use its resolved provenance instead of independently parsing the
// manifest or guessing which SQL/model a directory refers to.
func guardEnergyPathOutput(path string, projection simulation.EnergyPathProjection) error {
	path = strings.TrimSpace(path)
	if path == "" || path == "-" {
		return nil
	}
	if projection.Provenance == nil || projection.Provenance.SQLPath == "" || projection.Provenance.InputPath == "" {
		return fmt.Errorf("cannot safely write Energy Path output without resolved source paths")
	}
	outputPath, outputInfo, err := energyPathOutputFileIdentity(path)
	if err != nil {
		return err
	}
	directory := filepath.Dir(projection.Provenance.SQLPath)
	for _, protected := range []string{
		projection.Provenance.SQLPath,
		projection.Provenance.InputPath,
		filepath.Join(directory, "semantic-idf-run.json"),
		filepath.Join(directory, "semantic-idf-run-plan.json"),
	} {
		protectedPath, protectedInfo, err := energyPathOutputFileIdentity(protected)
		if err != nil {
			return err
		}
		samePath := outputPath == protectedPath || runtime.GOOS == "windows" && strings.EqualFold(outputPath, protectedPath)
		sameFile := outputInfo != nil && protectedInfo != nil && os.SameFile(outputInfo, protectedInfo)
		if samePath || sameFile {
			return fmt.Errorf("Energy Path output cannot overwrite its SQL, input model, or run metadata: %s", path)
		}
	}
	return nil
}

func energyPathOutputFileIdentity(path string) (string, os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}
	if err == nil {
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", nil, err
		}
		return filepath.Clean(resolved), info, nil
	}
	// Resolve the parent too: a missing metadata target may be addressed through
	// a directory alias even though there is no file identity to compare yet.
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil, nil
}

func parseEnergyPathCLI(args []string, stderr io.Writer) (energyPathCLIOptions, error) {
	options := energyPathCLIOptions{}
	fs := cliFlagSet("energy-path", stderr)
	fs.StringVar(&options.request.InputPath, "input", "", "IDF/epJSON path. If omitted, use the verified run input; stdin is not supported.")
	fs.StringVar(&options.request.Scope, "scope", "building", "Scope: building or zone.")
	fs.StringVar(&options.request.Zone, "zone", "", "Exact Zone name for zone scope.")
	fs.StringVar(&options.request.Period, "period", "annual", "Period: annual or M1 through M12.")
	fs.StringVar(&options.request.Service, "service", "all", "Service: all, cooling, or heating.")
	fs.StringVar(&options.format, "format", "json", "Output format: json or csv.")
	fs.BoolVar(&options.includeTrace, "include-trace", false, "Append source/link trace rows to CSV; JSON is unchanged.")
	fs.StringVar(&options.output, "o", "", "Output path, or - for stdout (default).")
	fs.StringVar(&options.output, "output", "", "Output path, or - for stdout (default).")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: semantic-idf energy-path <run-directory-or-sql> [options]")
		fmt.Fprintln(stderr, "Options may precede or follow the result path. This command never runs EnergyPlus.")
		fs.PrintDefaults()
	}
	orderedArgs, err := energyPathInterspersedArgs(fs, args)
	if err != nil {
		return options, err
	}
	if err := fs.Parse(orderedArgs); err != nil {
		return options, err
	}
	if fs.NArg() != 1 {
		return options, fmt.Errorf("energy-path requires exactly one run-directory or SQL path")
	}
	options.request.ResultPath = fs.Arg(0)
	if strings.TrimSpace(options.request.ResultPath) == "-" {
		return options, fmt.Errorf("energy-path requires a filesystem result path; SQL results cannot be read from stdin")
	}
	if strings.TrimSpace(options.request.InputPath) == "-" {
		return options, fmt.Errorf("energy-path --input requires an IDF/epJSON filesystem path; model stdin is not supported")
	}
	options.format = strings.ToLower(strings.TrimSpace(options.format))
	if options.format != "json" && options.format != "csv" {
		return options, fmt.Errorf("unsupported energy-path format %q; use json or csv", options.format)
	}
	// Scope, Zone, period, service, and file validation belong to the shared
	// simulation boundary, so CLI and HTTP reject the same invalid projections.
	return options, nil
}

// Go's flag parser stops at the first positional argument. Move only registered
// option/value tokens ahead of operands while preserving standard flag parsing,
// boolean semantics, and the explicit -- terminator. This is command-local so
// existing CLI commands retain their established argument contract.
func energyPathInterspersedArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	flags := make([]string, 0, len(args))
	operands := make([]string, 0, 1)
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			operands = append(operands, args[index+1:]...)
			break
		}
		if argument == "-" || !strings.HasPrefix(argument, "-") {
			operands = append(operands, argument)
			continue
		}
		flags = append(flags, argument)
		name := strings.TrimPrefix(strings.TrimPrefix(argument, "-"), "-")
		if strings.Contains(name, "=") {
			continue
		}
		option := fs.Lookup(name)
		if option == nil {
			continue // Let FlagSet produce its normal unknown-option/help error.
		}
		if boolean, ok := option.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
			continue
		}
		if index+1 >= len(args) {
			return nil, fmt.Errorf("flag needs an argument: -%s", name)
		}
		index++
		flags = append(flags, args[index])
	}
	flags = append(flags, "--")
	return append(flags, operands...), nil
}
