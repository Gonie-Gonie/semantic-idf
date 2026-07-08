package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/internal/tabular"
)

type cliInput struct {
	Path    string
	Content []byte
	Model   *epinput.Model
	Doc     idf.Document
}

func MaybeRun(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, version string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	if args[0] == "cli" {
		return true, runCLI(args[1:], stdin, stdout, stderr, version)
	}
	if isCLICommand(args[0]) || args[0] == "-h" || args[0] == "--help" || args[0] == "help" || args[0] == "version" || args[0] == "--version" {
		return true, runCLI(args, stdin, stdout, stderr, version)
	}
	return false, 0
}

func isCLICommand(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "summary", "multi-summary", "diagnostics", "diagnose", "analyze", "hvac-graph", "profile-graph", "profile-qa", "profile-schedules", "clean", "convert":
		return true
	default:
		return false
	}
}

func runCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, version string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		writeCLIHelp(stdout)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		if strings.TrimSpace(version) == "" {
			version = "0.0.0"
		}
		fmt.Fprintln(stdout, version)
		return 0
	}

	var err error
	switch strings.ToLower(args[0]) {
	case "summary":
		err = cliSummary(args[1:], stdin, stdout, stderr)
	case "multi-summary":
		err = cliMultiSummary(args[1:], stdin, stdout, stderr)
	case "diagnostics", "diagnose":
		err = cliDiagnostics(args[1:], stdin, stdout, stderr)
	case "analyze":
		err = cliAnalyze(args[1:], stdin, stdout, stderr)
	case "hvac-graph":
		err = cliHVACGraph(args[1:], stdin, stdout, stderr)
	case "profile-graph":
		err = cliProfileGraph(args[1:], stdin, stdout, stderr)
	case "profile-qa":
		err = cliProfileQA(args[1:], stdin, stdout, stderr)
	case "profile-schedules":
		err = cliProfileSchedules(args[1:], stdin, stdout, stderr)
	case "clean":
		err = cliClean(args[1:], stdin, stdout, stderr)
	case "convert":
		err = cliConvert(args[1:], stdin, stdout, stderr)
	default:
		err = fmt.Errorf("unknown CLI command %q; use `semantic-idf cli --help`", args[0])
	}
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func writeCLIHelp(w io.Writer) {
	fmt.Fprint(w, `SemanticIDF CLI

Usage:
  semantic-idf cli <command> [options] <input>
  semantic-idf <command> [options] <input>

Commands:
  summary        Export Summary metrics as text, JSON, CSV, or XLSX.
  multi-summary  Compare Summary metrics across multiple input files.
  diagnostics    Export Diagnose issues as text, JSON, or CSV.
  analyze        Export the full analysis report as JSON or compact text.
  hvac-graph     Export HVAC rule, service, or coupling navigation graph JSON.
  profile-graph  Export Profile Graph Deck series as JSON or text.
  profile-qa     Export Profile QA outliers and candidates as text, JSON, or CSV.
  profile-schedules Export resolved Profile schedules and similarity clusters.
  clean          Apply cleanup rules and optional semantic duplicate-name fixes.
  convert        Convert IDF/epJSON to IDF, JSON, semantic YAML view export, or XLSX tables.

Use "-" as input to read from stdin. Use "-o -" to write an output file stream to stdout.
Run "semantic-idf cli <command> --help" for command-specific options.
`)
}

func cliSummary(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("summary", stderr)
	format := fs.String("format", "text", "Output format: text, json, csv, xlsx.")
	output := fs.String("o", "", "Output path. Required for xlsx unless -o - is used.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inputPath, err := singleInputPath(fs)
	if err != nil {
		return err
	}
	input, err := readCLIInput(inputPath, stdin)
	if err != nil {
		return err
	}
	summary := idf.AnalyzeSummary(input.Doc)
	switch normalizeCLIFormat(*format) {
	case "text":
		return writeCLITextOutput(*output, []byte(formatSummaryText(summary)), stdout)
	case "json":
		text, err := idf.ExportSummaryJSON(summary)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "csv":
		text, err := idf.ExportSummaryCSV(summary)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "xlsx":
		sections := summarySections(summary)
		return writeCLIXLSXOutput(*output, "Summary", sections, stdout)
	default:
		return fmt.Errorf("unsupported summary format %q", *format)
	}
}

func cliMultiSummary(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("multi-summary", stderr)
	format := fs.String("format", "csv", "Output format: csv, json, xlsx, text.")
	output := fs.String("o", "", "Output path. Required for xlsx unless -o - is used.")
	orientation := fs.String("orientation", "metrics", "Table orientation: metrics or files.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("multi-summary requires at least one input file")
	}
	files := make([]multiSummaryCLIFile, 0, fs.NArg())
	for _, path := range fs.Args() {
		input, err := readCLIInput(path, stdin)
		if err != nil {
			return err
		}
		files = append(files, multiSummaryFileFromInput(input))
	}
	switch normalizeCLIFormat(*format) {
	case "text":
		return writeCLITextOutput(*output, []byte(formatMultiSummaryText(files)), stdout)
	case "json":
		payload, err := json.MarshalIndent(files, "", "  ")
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, append(payload, '\n'), stdout)
	case "csv":
		text, err := multiSummaryCSV(files, *orientation)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "xlsx":
		sections, err := multiSummarySections(files, *orientation)
		if err != nil {
			return err
		}
		return writeCLIXLSXOutput(*output, "Multi Summary", sections, stdout)
	default:
		return fmt.Errorf("unsupported multi-summary format %q", *format)
	}
}

func cliDiagnostics(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("diagnostics", stderr)
	format := fs.String("format", "text", "Output format: text, json, csv.")
	output := fs.String("o", "", "Output path.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inputPath, err := singleInputPath(fs)
	if err != nil {
		return err
	}
	input, err := readCLIInput(inputPath, stdin)
	if err != nil {
		return err
	}
	diagnostics := idf.AnalyzeDiagnostics(input.Doc)
	switch normalizeCLIFormat(*format) {
	case "text":
		return writeCLITextOutput(*output, []byte(formatDiagnosticsText(diagnostics)), stdout)
	case "json":
		payload, err := json.MarshalIndent(diagnostics, "", "  ")
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, append(payload, '\n'), stdout)
	case "csv":
		return writeCLITextOutput(*output, []byte(diagnosticsCSV(diagnostics)), stdout)
	default:
		return fmt.Errorf("unsupported diagnostics format %q", *format)
	}
}

func cliAnalyze(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("analyze", stderr)
	format := fs.String("format", "json", "Output format: json or text.")
	output := fs.String("o", "", "Output path.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inputPath, err := singleInputPath(fs)
	if err != nil {
		return err
	}
	input, err := readCLIInput(inputPath, stdin)
	if err != nil {
		return err
	}
	report := idf.Analyze(input.Doc)
	switch normalizeCLIFormat(*format) {
	case "json":
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, append(payload, '\n'), stdout)
	case "text":
		return writeCLITextOutput(*output, []byte(formatAnalysisText(report)), stdout)
	default:
		return fmt.Errorf("unsupported analyze format %q", *format)
	}
}

func cliClean(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("clean", stderr)
	output := fs.String("o", "", "Output path. Defaults to stdout.")
	rulesFlag := fs.String("rules", "default", "Cleanup rules: default, all, none, or comma-separated rule IDs.")
	excludeFlag := fs.String("exclude", "", "Comma-separated cleanup candidate keys to keep.")
	compact := fs.Bool("compact", false, "Also rewrite compact IDF formatting.")
	semanticDuplicates := fs.Bool("semantic-duplicates", false, "Rename duplicate same-type object names using semantic YAML policy.")
	dryRun := fs.Bool("dry-run", false, "Print planned removals/fixes without writing cleaned text.")
	format := fs.String("format", "text", "Dry-run output format: text or json.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inputPath, err := singleInputPath(fs)
	if err != nil {
		return err
	}
	input, err := readCLIInput(inputPath, stdin)
	if err != nil {
		return err
	}

	scan := idf.ScanCleanup(input.Doc)
	ruleIDs, err := cleanupRuleIDs(scan, *rulesFlag, *compact)
	if err != nil {
		return err
	}
	excluded := splitCommaFlag(*excludeFlag)
	preview := idf.PreviewCleanup(input.Doc, ruleIDs, excluded)
	_, semanticFixes := idf.ApplySemanticDuplicateNameFixes(input.Doc)

	if *dryRun {
		return writeCleanDryRun(*output, normalizeCLIFormat(*format), scan, ruleIDs, preview, semanticFixes, *semanticDuplicates, stdout)
	}

	updated, preview := idf.ApplyCleanup(input.Doc, ruleIDs, excluded)
	if *semanticDuplicates {
		var fixes []idf.SemanticDuplicateFix
		updated, fixes = idf.ApplySemanticDuplicateNameFixes(updated)
		semanticFixes = fixes
	} else {
		semanticFixes = nil
	}

	resultText := string(input.Content)
	if preview.RemovedCount > 0 || idf.CleanupCompacts(ruleIDs) || len(semanticFixes) > 0 {
		resultText = epinput.WriteDocumentLikeOriginal(updated, input.Model)
	}
	if err := writeCLITextOutput(*output, []byte(resultText), stdout); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Removed %d objects", preview.RemovedCount)
	if len(semanticFixes) > 0 {
		fmt.Fprintf(stderr, "; renamed %d duplicate objects", len(semanticFixes))
	}
	fmt.Fprintln(stderr, ".")
	return nil
}

func cliConvert(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("convert", stderr)
	target := fs.String("to", "", "Target format: idf, json, semantic-yaml projection (yaml alias), table, xlsx.")
	output := fs.String("o", "", "Output path. Required for table/xlsx unless -o - is used.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inputPath, err := singleInputPath(fs)
	if err != nil {
		return err
	}
	input, err := readCLIInput(inputPath, stdin)
	if err != nil {
		return err
	}
	switch normalizeCLIFormat(*target) {
	case "idf":
		text, err := epinput.Write(input.Model, epinput.FormatIDF)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "json", "epjson":
		text, err := epinput.Write(input.Model, epinput.FormatEPJSON)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "yaml", "semantic-yaml", "semantic":
		projection := idf.BuildSemanticYAMLProjection(input.Doc, idf.SemanticYAMLMetadata{
			EnergyPlusVersion: input.Model.Version.Raw,
			SourceFormat:      string(input.Model.Format),
		})
		return writeCLITextOutput(*output, []byte(projection.Text), stdout)
	case "table", "xlsx":
		return writeCLIXLSXOutput(*output, "IDF Tables", idf.ObjectTableSections(input.Doc), stdout)
	default:
		return fmt.Errorf("convert requires -to idf, json, yaml, table, or xlsx")
	}
}

func cliFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func singleInputPath(fs *flag.FlagSet) (string, error) {
	if fs.NArg() != 1 {
		return "", fmt.Errorf("%s requires exactly one input path", fs.Name())
	}
	return fs.Arg(0), nil
}

func readCLIInput(path string, stdin io.Reader) (cliInput, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return cliInput{}, fmt.Errorf("input path is required")
	}
	var content []byte
	var err error
	parseName := path
	if path == "-" {
		content, err = io.ReadAll(stdin)
		parseName = ""
	} else {
		content, err = os.ReadFile(path)
	}
	if err != nil {
		return cliInput{}, err
	}
	model, err := epinput.Parse(parseName, content)
	if err != nil {
		return cliInput{}, err
	}
	doc := epinput.ToIDFDocument(model)
	return cliInput{Path: path, Content: content, Model: model, Doc: doc}, nil
}

func writeCLITextOutput(path string, content []byte, stdout io.Writer) error {
	path = strings.TrimSpace(path)
	if path == "" || path == "-" {
		_, err := stdout.Write(content)
		return ignoreBrokenPipe(err)
	}
	return os.WriteFile(path, content, 0o644)
}

func writeCLIXLSXOutput(path string, sheetName string, sections []tabular.Section, stdout io.Writer) error {
	path = strings.TrimSpace(path)
	var b bytes.Buffer
	if err := tabular.WriteOneSheetXLSX(&b, sheetName, sections); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("xlsx output requires -o <file.xlsx> or -o -")
	}
	if path == "-" {
		_, err := stdout.Write(b.Bytes())
		return ignoreBrokenPipe(err)
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func ignoreBrokenPipe(err error) error {
	if err == nil {
		return nil
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "broken pipe") || strings.Contains(text, "pipe is being closed") {
		return nil
	}
	return err
}

func normalizeCLIFormat(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "":
		return ""
	case "epjson":
		return "json"
	case "xls":
		return "xlsx"
	default:
		return value
	}
}

func formatSummaryText(summary idf.SummaryReport) string {
	var b strings.Builder
	for categoryIndex, category := range summary.Categories {
		if categoryIndex > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[%s]\n", category.Name)
		for _, metric := range category.Metrics {
			unit := ""
			if strings.TrimSpace(metric.Unit) != "" {
				unit = " " + metric.Unit
			}
			fmt.Fprintf(&b, "%s: %s%s (%s)\n", metric.Name, metric.DisplayValue, unit, metric.Status)
		}
	}
	return b.String()
}

func summarySections(summary idf.SummaryReport) []tabular.Section {
	sections := make([]tabular.Section, 0, len(summary.Categories))
	for _, category := range summary.Categories {
		section := tabular.Section{Title: category.Name, Headers: []string{"id", "name", "value", "unit", "status"}}
		for _, metric := range category.Metrics {
			section.Rows = append(section.Rows, []string{
				metric.ID,
				metric.Name,
				metric.DisplayValue,
				metric.Unit,
				metric.Status,
			})
		}
		sections = append(sections, section)
	}
	return sections
}

func formatDiagnosticsText(diagnostics []idf.Diagnostic) string {
	if len(diagnostics) == 0 {
		return "No diagnostics found.\n"
	}
	var b strings.Builder
	for _, diagnostic := range diagnostics {
		location := ""
		if diagnostic.ObjectType != "" {
			location = fmt.Sprintf(" #%d %s", diagnostic.ObjectIndex+1, diagnostic.ObjectType)
			if diagnostic.ObjectName != "" {
				location += " " + strconvQuote(diagnostic.ObjectName)
			}
		}
		fmt.Fprintf(&b, "[%s] %s/%s%s: %s\n", diagnostic.Severity, diagnostic.Category, diagnostic.Code, location, diagnostic.Message)
	}
	return b.String()
}

func diagnosticsCSV(diagnostics []idf.Diagnostic) string {
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	_ = writer.Write([]string{"severity", "category", "code", "message", "object_index", "object_type", "object_name", "field_index", "field", "value"})
	for _, diagnostic := range diagnostics {
		objectIndex := ""
		if diagnostic.ObjectType != "" || diagnostic.ObjectName != "" || diagnostic.ObjectIndex > 0 {
			objectIndex = fmt.Sprintf("%d", diagnostic.ObjectIndex+1)
		}
		fieldIndex := ""
		if diagnostic.Field != "" || diagnostic.Value != "" || diagnostic.FieldIndex > 0 {
			fieldIndex = fmt.Sprintf("%d", diagnostic.FieldIndex+1)
		}
		_ = writer.Write([]string{
			diagnostic.Severity,
			diagnostic.Category,
			diagnostic.Code,
			diagnostic.Message,
			objectIndex,
			diagnostic.ObjectType,
			diagnostic.ObjectName,
			fieldIndex,
			diagnostic.Field,
			diagnostic.Value,
		})
	}
	writer.Flush()
	return b.String()
}

func formatAnalysisText(report idf.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objects: %d\n", report.ObjectCount)
	fmt.Fprintf(&b, "Types: %d\n", len(report.TypeCounts))
	fmt.Fprintf(&b, "Zones: %d\n", len(report.Zones))
	fmt.Fprintf(&b, "Schedules: %d\n", len(report.Schedules))
	fmt.Fprintf(&b, "HVAC connections: %d\n", len(report.HVACConnections))
	fmt.Fprintf(&b, "Diagnostics: %d\n\n", len(report.Diagnostics))
	b.WriteString("[Top Object Types]\n")
	limit := min(20, len(report.TypeCounts))
	for _, item := range report.TypeCounts[:limit] {
		fmt.Fprintf(&b, "%s: %d\n", item.Type, item.Count)
	}
	return b.String()
}

func cleanupRuleIDs(scan idf.CleanupScan, value string, compact bool) ([]string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = "default"
	}
	selected := map[string]bool{}
	switch value {
	case "default":
		for _, rule := range scan.Rules {
			if rule.Default && rule.Available {
				selected[rule.ID] = true
			}
		}
	case "all":
		for _, rule := range scan.Rules {
			if rule.Available && !rule.Future {
				selected[rule.ID] = true
			}
		}
	case "none":
	default:
		available := map[string]bool{}
		for _, rule := range scan.Rules {
			available[rule.ID] = rule.Available
		}
		for _, ruleID := range splitCommaFlag(value) {
			if ruleID == "" {
				continue
			}
			if !available[ruleID] && ruleID != idf.CleanupRuleCompactFormatting {
				return nil, fmt.Errorf("cleanup rule %q is not available", ruleID)
			}
			selected[ruleID] = true
		}
	}
	if compact {
		selected[idf.CleanupRuleCompactFormatting] = true
	}
	return sortedKeys(selected), nil
}

func writeCleanDryRun(output string, format string, scan idf.CleanupScan, ruleIDs []string, preview idf.CleanupPreview, semanticFixes []idf.SemanticDuplicateFix, includeSemantic bool, stdout io.Writer) error {
	if format == "json" {
		payload := map[string]any{
			"rules":              ruleIDs,
			"scan":               scan,
			"removedCandidates":  preview.RemovedCandidates,
			"removedCount":       preview.RemovedCount,
			"semanticNameFixes":  semanticFixes,
			"semanticDuplicates": includeSemantic,
		}
		content, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return writeCLITextOutput(output, append(content, '\n'), stdout)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Rules: %s\n", strings.Join(ruleIDs, ", "))
	fmt.Fprintf(&b, "Cleanup removals: %d\n", preview.RemovedCount)
	for _, candidate := range preview.RemovedCandidates {
		fmt.Fprintf(&b, "- %s #%d %s %q: %s\n", candidate.RuleID, candidate.ObjectIndex+1, candidate.ObjectType, candidate.ObjectName, candidate.Reason)
	}
	if includeSemantic {
		fmt.Fprintf(&b, "Semantic duplicate renames: %d\n", len(semanticFixes))
		for _, fix := range semanticFixes {
			fmt.Fprintf(&b, "- #%d %s: %q -> %q\n", fix.ObjectIndex+1, fix.ObjectType, fix.Before, fix.After)
		}
	}
	return writeCLITextOutput(output, []byte(b.String()), stdout)
}

func splitCommaFlag(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, ok := range values {
		if ok {
			keys = append(keys, key)
		}
	}
	sortStrings(keys)
	return keys
}

type multiSummaryCLIFile struct {
	Path         string            `json:"path"`
	Label        string            `json:"label"`
	Format       string            `json:"format"`
	Version      string            `json:"version,omitempty"`
	ObjectCount  int               `json:"objectCount"`
	MetricValues map[string]string `json:"metricValues"`
}

func multiSummaryFileFromInput(input cliInput) multiSummaryCLIFile {
	summary := idf.AnalyzeSummary(input.Doc)
	values := map[string]string{}
	label := filepath.Base(input.Path)
	if input.Path == "-" {
		label = "stdin"
	}
	for _, category := range summary.Categories {
		for _, metric := range category.Metrics {
			values[metric.ID] = metric.DisplayValue
			if metric.ID == "building_name" && metric.DisplayValue != "" && !strings.EqualFold(metric.DisplayValue, "N/A") {
				label = metric.DisplayValue
			}
		}
	}
	return multiSummaryCLIFile{
		Path:         input.Path,
		Label:        label,
		Format:       string(input.Model.Format),
		Version:      input.Model.Version.Raw,
		ObjectCount:  len(input.Doc.Objects),
		MetricValues: values,
	}
}

func formatMultiSummaryText(files []multiSummaryCLIFile) string {
	var b strings.Builder
	for _, file := range files {
		fmt.Fprintf(&b, "[%s]\npath: %s\nformat: %s\nversion: %s\nobjects: %d\n\n", file.Label, file.Path, file.Format, file.Version, file.ObjectCount)
	}
	return b.String()
}

func multiSummaryCSV(files []multiSummaryCLIFile, orientation string) (string, error) {
	section, err := multiSummarySection(files, orientation)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	if err := writer.Write(section.Headers); err != nil {
		return "", err
	}
	for _, row := range section.Rows {
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}
	writer.Flush()
	return b.String(), writer.Error()
}

func multiSummarySections(files []multiSummaryCLIFile, orientation string) ([]tabular.Section, error) {
	section, err := multiSummarySection(files, orientation)
	if err != nil {
		return nil, err
	}
	return []tabular.Section{section}, nil
}

func multiSummarySection(files []multiSummaryCLIFile, orientation string) (tabular.Section, error) {
	definitions := idf.SummaryDefinitions()
	orientation = strings.ToLower(strings.TrimSpace(orientation))
	if orientation == "" || orientation == "metrics" {
		headers := []string{"metric_id", "metric_name", "unit"}
		for _, file := range files {
			headers = append(headers, file.Label)
		}
		rows := make([][]string, 0, len(definitions))
		for _, definition := range definitions {
			row := []string{definition.ID, definition.Name, definition.Unit}
			for _, file := range files {
				row = append(row, file.MetricValues[definition.ID])
			}
			rows = append(rows, row)
		}
		return tabular.Section{Title: "multi_summary_metrics", Headers: headers, Rows: rows}, nil
	}
	if orientation == "files" {
		headers := []string{"label", "path", "format", "version", "object_count"}
		for _, definition := range definitions {
			headers = append(headers, definition.ID)
		}
		rows := make([][]string, 0, len(files))
		for _, file := range files {
			row := []string{file.Label, file.Path, file.Format, file.Version, fmt.Sprintf("%d", file.ObjectCount)}
			for _, definition := range definitions {
				row = append(row, file.MetricValues[definition.ID])
			}
			rows = append(rows, row)
		}
		return tabular.Section{Title: "multi_summary_files", Headers: headers, Rows: rows}, nil
	}
	return tabular.Section{}, fmt.Errorf("orientation must be metrics or files")
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		value := values[i]
		j := i - 1
		for j >= 0 && values[j] > value {
			values[j+1] = values[j]
			j--
		}
		values[j+1] = value
	}
}
