package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

const hvacGraphExportSchema = "semantic-idf.hvac.graph.v1"

type hvacGraphExportPayload struct {
	Schema string              `json:"schema"`
	Graph  string              `json:"graph"`
	Counts hvacGraphCounts     `json:"counts"`
	Data   hvacGraphExportData `json:"data"`
}

type hvacGraphCounts struct {
	RuleNodes          int `json:"ruleNodes"`
	RuleEdges          int `json:"ruleEdges"`
	ZoneServices       int `json:"zoneServices"`
	ServicePaths       int `json:"servicePaths"`
	Systems            int `json:"systems"`
	Components         int `json:"components"`
	Couplings          int `json:"couplings"`
	Networks           int `json:"networks"`
	NavigationEntities int `json:"navigationEntities"`
	NavigationLinks    int `json:"navigationLinks"`
}

type hvacGraphExportData struct {
	RuleGraph    *idf.HVACRuleGraph       `json:"ruleGraph,omitempty"`
	ServiceModel *idf.HVACServiceModel    `json:"serviceModel,omitempty"`
	Couplings    []idf.SystemCoupling     `json:"couplings,omitempty"`
	Networks     []idf.EnergyNetwork      `json:"networks,omitempty"`
	Navigation   *idf.HVACNavigationIndex `json:"navigation,omitempty"`
}

func cliHVACGraph(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	fs := cliFlagSet("hvac-graph", stderr)
	graph := fs.String("graph", "service", "Graph to export: rule, service, or coupling.")
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
	report := idf.AnalyzeHVAC(input.Doc)
	payload, err := buildHVACGraphExportPayload(report, *graph)
	if err != nil {
		return err
	}
	switch normalizeCLIFormat(*format) {
	case "json":
		text, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		return writeCLITextOutput(*output, append(text, '\n'), stdout)
	case "text":
		return writeCLITextOutput(*output, []byte(formatHVACGraphText(payload)), stdout)
	default:
		return fmt.Errorf("unsupported hvac-graph format %q", *format)
	}
}

func buildHVACGraphExportPayload(report idf.HVACReport, graph string) (hvacGraphExportPayload, error) {
	rawGraph := graph
	graph = normalizeHVACGraphKind(graph)
	if graph == "" {
		return hvacGraphExportPayload{}, fmt.Errorf("unsupported HVAC graph %q", rawGraph)
	}
	serviceModel := report.ServiceModel
	payload := hvacGraphExportPayload{
		Schema: hvacGraphExportSchema,
		Graph:  graph,
		Counts: hvacGraphCounts{
			RuleNodes:          len(report.RuleGraph.Nodes),
			RuleEdges:          len(report.RuleGraph.Edges),
			ZoneServices:       len(serviceModel.ZoneServices),
			ServicePaths:       countHVACServicePaths(serviceModel.ZoneServices),
			Systems:            len(serviceModel.Systems),
			Components:         len(serviceModel.Components),
			Couplings:          len(serviceModel.Couplings),
			Networks:           len(serviceModel.Networks),
			NavigationEntities: len(serviceModel.Navigation.Entities),
			NavigationLinks:    len(serviceModel.Navigation.Links),
		},
	}
	switch graph {
	case "rule":
		payload.Data.RuleGraph = &report.RuleGraph
	case "service":
		payload.Data.ServiceModel = &serviceModel
	case "coupling":
		payload.Data.Couplings = serviceModel.Couplings
		payload.Data.Networks = serviceModel.Networks
		payload.Data.Navigation = &serviceModel.Navigation
	default:
		return hvacGraphExportPayload{}, fmt.Errorf("unsupported HVAC graph %q", graph)
	}
	return payload, nil
}

func normalizeHVACGraphKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "service", "services", "service-model":
		return "service"
	case "rule", "rules":
		return "rule"
	case "coupling", "couplings", "support":
		return "coupling"
	default:
		return ""
	}
}

func countHVACServicePaths(summaries []idf.ZoneServiceSummary) int {
	count := 0
	for _, summary := range summaries {
		count += len(summary.Paths)
	}
	return count
}

func formatHVACGraphText(payload hvacGraphExportPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "HVAC graph: %s\n", payload.Graph)
	fmt.Fprintf(&b, "Schema: %s\n", payload.Schema)
	fmt.Fprintf(&b, "Rule nodes: %d\n", payload.Counts.RuleNodes)
	fmt.Fprintf(&b, "Rule edges: %d\n", payload.Counts.RuleEdges)
	fmt.Fprintf(&b, "Zone services: %d\n", payload.Counts.ZoneServices)
	fmt.Fprintf(&b, "Service paths: %d\n", payload.Counts.ServicePaths)
	fmt.Fprintf(&b, "Systems: %d\n", payload.Counts.Systems)
	fmt.Fprintf(&b, "Components: %d\n", payload.Counts.Components)
	fmt.Fprintf(&b, "Couplings: %d\n", payload.Counts.Couplings)
	fmt.Fprintf(&b, "Networks: %d\n", payload.Counts.Networks)
	fmt.Fprintf(&b, "Navigation entities: %d\n", payload.Counts.NavigationEntities)
	fmt.Fprintf(&b, "Navigation links: %d\n", payload.Counts.NavigationLinks)
	return b.String()
}
