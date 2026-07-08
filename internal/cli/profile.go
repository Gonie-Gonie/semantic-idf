package cli

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

func cliProfileGraph(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("profile-graph", stderr)
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
	profile := idf.AnalyzeProfile(input.Doc)
	switch normalizeCLIFormat(*format) {
	case "json":
		text, err := idf.ExportProfileGraphJSON(profile)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "text":
		return writeCLITextOutput(*output, []byte(formatProfileGraphText(profile)), stdout)
	default:
		return fmt.Errorf("unsupported profile-graph format %q", *format)
	}
}

func cliProfileQA(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("profile-qa", stderr)
	format := fs.String("format", "text", "Output format: text, json, or csv.")
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
	profile := idf.AnalyzeProfile(input.Doc)
	switch normalizeCLIFormat(*format) {
	case "text":
		return writeCLITextOutput(*output, []byte(formatProfileQAText(profile)), stdout)
	case "json":
		text, err := idf.ExportProfileQAJSON(profile)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "csv":
		text, err := idf.ExportProfileQACSV(profile)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	default:
		return fmt.Errorf("unsupported profile-qa format %q", *format)
	}
}

func cliProfileSchedules(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("profile-schedules", stderr)
	format := fs.String("format", "json", "Output format: json, text, or csv.")
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
	profile := idf.AnalyzeProfile(input.Doc)
	switch normalizeCLIFormat(*format) {
	case "json":
		text, err := idf.ExportProfileSchedulesJSON(profile)
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, []byte(text), stdout)
	case "text":
		return writeCLITextOutput(*output, []byte(formatProfileSchedulesText(profile)), stdout)
	case "csv":
		return writeCLITextOutput(*output, []byte(profileSchedulesCSV(profile)), stdout)
	default:
		return fmt.Errorf("unsupported profile-schedules format %q", *format)
	}
}

func formatProfileGraphText(profile idf.ProfileReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Profile graph series: %d\n", len(profile.GraphDataset.Series))
	fmt.Fprintf(&b, "Default deck: scope=%s metric=%s time=%s compare=%s scale=%s\n",
		profile.GraphDataset.DefaultDeck.ScopeType,
		profile.GraphDataset.DefaultDeck.MetricMode,
		profile.GraphDataset.DefaultDeck.TimeView,
		profile.GraphDataset.DefaultDeck.CompareMode,
		profile.GraphDataset.DefaultDeck.ScaleMode,
	)
	limit := len(profile.GraphDataset.Series)
	if limit > 20 {
		limit = 20
	}
	for _, series := range profile.GraphDataset.Series[:limit] {
		fmt.Fprintf(&b, "- %s [%s] %s %s peak=%s annual=%s\n",
			series.Label,
			series.ScopeType,
			series.Dimension,
			series.DisplayValue,
			formatSummaryFloat(series.Peak),
			formatSummaryFloat(series.AnnualContribution),
		)
	}
	return b.String()
}

func formatProfileQAText(profile idf.ProfileReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Profile QA outliers: %d\n", len(profile.Outliers))
	for _, hint := range profile.Outliers {
		location := strings.TrimSpace(strings.Join(nonEmptyCLIStrings(hint.ZoneName, hint.GroupID, hint.Dimension), " / "))
		if location != "" {
			location = " " + location
		}
		fmt.Fprintf(&b, "- [%s] %s%s: %s\n", hint.Severity, hint.RuleID, location, hint.Message)
	}
	fmt.Fprintf(&b, "\nParameter candidates: %d\n", len(profile.ParameterCandidates))
	for _, candidate := range profile.ParameterCandidates {
		fmt.Fprintf(&b, "- [%s] %s zones=%d range=%s..%s impact=%s\n",
			candidate.Severity,
			candidate.Label,
			len(candidate.ZoneNames),
			formatSummaryFloat(candidate.CurrentMin),
			formatSummaryFloat(candidate.CurrentMax),
			formatSummaryFloat(candidate.ImpactScore),
		)
	}
	return b.String()
}

func formatProfileSchedulesText(profile idf.ProfileReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Profile schedules: %d\n", len(profile.Schedules))
	for _, schedule := range profile.Schedules {
		fmt.Fprintf(&b, "- %s [%s] pattern=%s hash=%s operating_hours=%s\n",
			schedule.ScheduleName,
			schedule.ScheduleType,
			schedule.DetectedPattern,
			schedule.ContentHash,
			formatSummaryFloat(schedule.AnnualStats.OperatingHours),
		)
	}
	fmt.Fprintf(&b, "\nSchedule clusters: %d\n", len(profile.ScheduleClusters))
	for _, cluster := range profile.ScheduleClusters {
		fmt.Fprintf(&b, "- %s names=%d zones=%d same_content_names=%t\n",
			cluster.Pattern,
			len(cluster.ScheduleNames),
			len(cluster.ZoneNames),
			cluster.SameContentDifferentNames,
		)
	}
	return b.String()
}

func profileSchedulesCSV(profile idf.ProfileReport) string {
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	_ = writer.Write([]string{"name", "type", "pattern", "hash", "average", "max", "operating_hours", "equivalent_full_hours"})
	for _, schedule := range profile.Schedules {
		_ = writer.Write([]string{
			schedule.ScheduleName,
			schedule.ScheduleType,
			schedule.DetectedPattern,
			schedule.ContentHash,
			formatSummaryFloat(schedule.AnnualStats.Average),
			formatSummaryFloat(schedule.AnnualStats.Max),
			formatSummaryFloat(schedule.AnnualStats.OperatingHours),
			formatSummaryFloat(schedule.AnnualStats.EquivalentFullHours),
		})
	}
	writer.Flush()
	return b.String()
}

func nonEmptyCLIStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func formatSummaryFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", value), "0"), ".")
}
