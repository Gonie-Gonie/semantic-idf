package idf

// Deprecated compatibility aliases keep older integrations source-compatible.
// New code should use the Metrics/Metric names from metrics.go.
type SummaryReport = MetricsReport
type SummaryCategory = MetricCategory
type SummaryMetric = Metric
type SummaryDefinition = MetricDefinition
type SummaryGuide = MetricGuide
type SummaryAnalysisOptions = MetricsAnalysisOptions

// Deprecated: use MetricDefinitions.
func SummaryDefinitions() []MetricDefinition { return MetricDefinitions() }

// Deprecated: use MetricGuides.
func SummaryGuides() []MetricGuide { return MetricGuides() }

// Deprecated: use AnalyzeMetrics.
func AnalyzeSummary(doc Document) MetricsReport { return AnalyzeMetrics(doc) }

// Deprecated: use AnalyzeMetricsQuick.
func AnalyzeSummaryQuick(doc Document) MetricsReport { return AnalyzeMetricsQuick(doc) }

// Deprecated: use AnalyzeMetricsWithOptions.
func AnalyzeSummaryWithOptions(doc Document, options MetricsAnalysisOptions) MetricsReport {
	return AnalyzeMetricsWithOptions(doc, options)
}

// Deprecated: use ExportMetricsJSON.
func ExportSummaryJSON(report MetricsReport) (string, error) { return ExportMetricsJSON(report) }

// Deprecated: use ExportMetricsCSV.
func ExportSummaryCSV(report MetricsReport) (string, error) { return ExportMetricsCSV(report) }

// Deprecated: use MetricsCSVNames.
func SummaryCSVMetricNames(report MetricsReport) map[string]string { return MetricsCSVNames(report) }
